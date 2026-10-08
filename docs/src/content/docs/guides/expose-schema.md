---
title: Expose your command tree with `__schema`
description: Emit a structured JSON description of your CLI so agents — and ax-go mcp-server — can discover it.
sidebar:
  order: 1
---

You have an `ax-go` CLI and you want an agent (or an MCP client) to discover it
without a human hand-writing an integration. This guide shows how to read the
`__schema` output, report a real version in it, emit an MCP-compatible
description, and serve the whole CLI as an MCP server.

This is a recipe, not a lesson. It assumes you already have a command built on
`ax.Execute` — if not, work through
[Build your first agent-ready CLI](/ax-go/tutorials/build-your-first-cli/) first.

## Read the schema you already have

`ax.Execute` injects a `__schema` command for you. Run it:

```bash
yourcli __schema
```

The JSON on `stdout` describes the tool and its command tree:

```json
{"schema_version":"1.0.0","tool":"yourcli","version":"0.1.0","mode_detection":"--format flag > AGENT_MODE env > TTY detection","command":{"use":"yourcli","short":"...","flags":[{"name":"dry-run","type":"bool","default":"false","usage":"..."},{"name":"format","type":"string","usage":"..."},{"name":"idempotency-key","type":"string","usage":"..."}]}}
```

The `--format`, `--dry-run`, and `--idempotency-key` flags appear even though you
never declared them — ax-go adds them to every command.

## Report a real version

If `version` reads `0.0.0-unknown`, no build version reached the binary. Wire one
in with a package variable and a link-time flag:

```go
var version string // set by -ldflags "-X main.version=..."

func main() {
	resolved := ax.ResolveVersion(version)

	root := newRootCommand() // your existing command tree

	os.Exit(ax.Execute(context.Background(), root, ax.WithVersion(resolved)))
}
```

Build with the version injected:

```bash
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o yourcli .
yourcli __schema   # "version" now reflects the build
```

`ax.ResolveVersion` guarantees a usable value — it falls back to Go build
metadata and never returns the bare `dev` or `unknown`. Pass the same `resolved`
value to `ax.WithVersion`, `ax.WithLoggerLabels`, and `mcp.WithVersion` so
`__schema.version`, `ax.Error.version`, and the logger `version` label all agree.

## Emit MCP tool descriptions

To get the same command tree shaped as MCP tool definitions, add `--as=mcp`:

```bash
yourcli __schema --as=mcp
```

```json
{"tools":[{"name":"yourcli","description":"...","inputSchema":{"properties":{"format":{"type":"string","description":"..."}},"type":"object"}}]}
```

This is a one-shot description — useful for registering your CLI with an MCP
client or inspecting what it would expose.

## Serve the CLI as an MCP server

To serve live over MCP, mount the reserved `mcp-server` command. Unlike
`__schema`, this one is opt-in — you add it explicitly:

```go
import "github.com/rshade/ax-go/mcp"

// inside your command setup, after building root:
root.AddCommand(mcp.NewCommand(root, mcp.WithVersion(resolved)))
```

Run it over stdio (the default transport):

```bash
yourcli mcp-server
```

Every visible, runnable command becomes a callable MCP tool. The rules are
covered in [Excluding commands](#excluding-commands) below. To serve over HTTP
instead:

```bash
yourcli mcp-server --transport=http --addr=127.0.0.1:8080
```

:::caution[Public binds are fail-closed]
The HTTP transport binds loopback (`127.0.0.1:8080`) by default. Binding a
non-loopback or public interface additionally requires `--allow-non-loopback`;
without it, startup fails with a validation error (exit `2`). A placeholder
version (`dev`, `unknown`, or empty) is rejected the same way — inject a real
one.
:::

## Excluding commands

`__schema --as=mcp` and `mcp-server` share one walk of your command tree, so
they always list the same tools. That walk applies these rules:

| Command | Effect on the tool list |
| --- | --- |
| `Hidden: true` | Dropped with its whole subtree |
| Reserved: `__schema`, `mcp-server`, `completion`, `help` | Dropped with its whole subtree |
| Pure group with no `Run` or `RunE` | Dropped on its own; children stay tools |
| Marked with `mcp.Exclude` | Dropped on its own; children stay tools |

A group command only prints usage, which is prose rather than a machine
payload, so it never becomes a tool. A command named `help` is reserved even
when you define it yourself; rename it if you want it callable.

Use `mcp.Exclude` for a command an agent should not call, such as an
interactive TUI root or a long-running server that would block every later
tool call:

```go
import "github.com/rshade/ax-go/mcp"

mcp.Exclude(root)  // the TUI root; its subcommands stay tools
mcp.Exclude(serve) // a blocking server command
```

An excluded command still appears in `--help` and in `__schema --as=ax`.
Calling its tool name returns the same unknown-tool error as any name the
server never registered, and the command does not run. Exclusion is a Cobra
annotation (`github.com/rshade/ax-go/mcp/exclude` set to `true`); any other
value is ignored. To hide a command from humans too, set `Hidden: true`
instead.

## Declare prompts and resources

Tools tell an agent what it *can* call. A prompt tells it *how* to use your CLI
for a task, and a resource gives it stable reference material it can look up
again. Declare both on the command they describe:

```go
if err := ax.DeclarePrompt(root, ax.Prompt{
    Name:        "triage-spike",
    Title:       "Triage a cost spike",
    Description: "Find and explain the top cost driver in a window.",
    Arguments: []ax.PromptArgument{
        {Name: "window", Description: "lookback window, e.g. 7d", Required: true},
    },
    Template: "Run `yourcli report --since={{window}}`, then `yourcli explain` on the top line item.",
}); err != nil {
    return err
}

if err := ax.DeclareResource(root, ax.Resource{
    URI:      "yourcli://docs/pricing-model",
    Name:     "pricing-model",
    MIMEType: "text/markdown",
}); err != nil {
    return err
}
```

`yourcli __schema` shows each declaration on its command, under `prompts` and
`resources`. `yourcli __schema --as=mcp` collects every declaration into
top-level arrays, in command-tree order:

```json
{"tools":[...],"prompts":[{"name":"triage-spike","title":"Triage a cost spike","description":"...","arguments":[{"name":"window","description":"lookback window, e.g. 7d","required":true}],"template":"Run `yourcli report --since={{window}}`, then `yourcli explain` on the top line item."}],"resources":[{"uri":"yourcli://docs/pricing-model","name":"pricing-model","mimeType":"text/markdown"}]}
```

The rules are checked when you declare, and a bad declaration returns an
`*ax.Error` with `error_code` `invalid_schema_declaration` (exit `2`). Its
`context` names the `field` and a stable `reason`, and `actionable_fix` says
what to change:

- A prompt needs a name and a template. Names (prompt and argument) use only
  letters, digits, `_`, `.`, and `-`, and argument names are unique within a
  prompt.
- Every `{{name}}` in a template must name a declared argument. Other brace
  text, such as `{{ name }}` with spaces, is left as literal text.
- A resource needs a name and a URI. The URI must carry a scheme
  (`yourcli://docs/x` or `urn:yourcli:x`), parse, stay within 2048 bytes, and
  contain no spaces or control characters. A MIME type may contain spaces, as
  in `text/plain; charset=utf-8`, but no control characters.
- A prompt name or resource URI already declared on the same command is
  rejected, and all text must be valid UTF-8.

A resource can carry static `Content` (at most 1 MiB, valid UTF-8) that
`mcp-server` returns from `resources/read`. `__schema` never projects it, so a
long document does not bloat discovery. Content is fixed text captured when you
declare it, never a callback, so a resource cannot expose live state or run
records. A resource declared without content is still listed and reads as an
empty body. ax-go never inlines a truncated body, because an agent cannot tell a
fragment from the whole. Keep prompt templates short (at most 64 KiB), because
`__schema` includes them; put long reference text in resource content.

Declarations under a hidden command are dropped, like the command itself. The
root command always keeps its own, even when it is hidden, exactly as
`__schema` keeps a hidden root. A group command or a command marked with
`mcp.Exclude` keeps its declarations, because those rules only decide which
commands become tools.

`__schema` refuses to emit a silently reduced contract. It fails with a
`validation_error` (exit `2`) when two commands declare the same prompt name or
resource URI, naming both commands, and when a declaration annotation was
written by hand and does not decode cleanly. Declare only through
`ax.DeclarePrompt` and `ax.DeclareResource`.

### Serve them, and tell the agent where to look

`mcp-server` registers every declared prompt and resource it would list in
`__schema --as=mcp`, in the same order, and advertises a capability only when
something is declared, so a CLI that declares nothing sends the handshake it
always did. Pass `mcp.WithInstructions` to put a short string in the
`initialize` result:

```go
root.AddCommand(mcp.NewCommand(root,
    mcp.WithVersion(version),
    mcp.WithInstructions("Read yourcli://docs/pricing-model before calling any tool."),
))
```

Instructions must be valid UTF-8 and at most 8 KiB, or the server fails at
startup with a `validation_error` (exit `2`). In Claude Code the instructions
reach the agent's context when it connects, the model can read a resource itself,
and a prompt becomes a `/mcp__<server>__<prompt>` slash command the user runs.
`prompts/get` replaces each declared `{{name}}` with the argument's value,
leaves other brace text alone, and rejects a missing required argument or an
undeclared one with an invalid-params error. An absent optional argument
renders as empty text. The server only renders text; it never runs it.

When the server starts, it applies the same duplicate and corrupt-annotation
checks as `__schema`, so it never serves a set `__schema` would refuse.

## Keep the schema stable in CI

`__schema` is part of your public contract: an agent that learned your CLI from
it will break if the shape changes silently. Pin the output with a golden-file
test so any change to the command tree, flags, or types has to be reviewed
deliberately rather than slipping through.

## Related

- **Tutorial:** [Build your first agent-ready CLI](/ax-go/tutorials/build-your-first-cli/)
- **Guide:** [Test a command built on ax-go](/ax-go/guides/test-a-command/) —
  run the same flags this page documents through a real test.
- **Explanation:** [Why Agentic Experience?](/ax-go/explanation/why-agentic-experience/)
  — why self-description matters to an agent.
