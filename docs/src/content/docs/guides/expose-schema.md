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

Every non-hidden command becomes a callable MCP tool — except `__schema` and
`mcp-server` themselves, which are reserved. To serve over HTTP instead:

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

## Keep the schema stable in CI

`__schema` is part of your public contract: an agent that learned your CLI from
it will break if the shape changes silently. Pin the output with a golden-file
test so any change to the command tree, flags, or types has to be reviewed
deliberately rather than slipping through.

## Related

- **Tutorial:** [Build your first agent-ready CLI](/ax-go/tutorials/build-your-first-cli/)
- **Explanation:** [Why Agentic Experience?](/ax-go/explanation/why-agentic-experience/)
  — why self-description matters to an agent.
