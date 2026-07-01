---
title: Build your first agent-ready CLI
description: A guided lesson from an empty Go module to a working ax-go CLI that agents can call predictably.
sidebar:
  order: 1
---

In this lesson you will build a small command-line tool, `greeter`, from an
empty Go module. By the end it will do everything an LLM agent needs from a
well-behaved CLI: emit a deterministic JSON payload on `stdout`, keep logs on
`stderr`, return a structured error with the right exit code, and describe
itself when asked. You will run every command yourself and see the output at
each step.

This is a lesson, not a reference. Follow the steps in order and you will reach
a working tool. Once you understand the shape, the
[How-to Guides](/ax-go/guides/expose-schema/) show how to go further, and
[Why Agentic Experience?](/ax-go/explanation/why-agentic-experience/) explains
the reasoning behind the contracts you are about to use.

## What you will build

A single command that greets someone and prints a machine-readable envelope:

```console
$ greeter --format=json --name Ada
{"data":{"message":"hello","name":"Ada","times":1,"mode":"json"},"meta":{"trace_id":"...","span_id":"...","idempotency_key":"..."}}
```

The same binary will also reject bad input with a structured error and a
deterministic exit code, and answer `greeter __schema` with a full description
of itself — without you writing any of that plumbing.

## Prerequisites

- **Go 1.26 or newer** installed (`go version` to check).
- A terminal.
- Basic familiarity with Cobra commands is helpful but not required — you will
  copy the small amount you need.

## Step 1 — Create the module

Make a new directory and initialize a Go module:

```bash
mkdir greeter && cd greeter
go mod init example.com/greeter
```

You now have an empty module with a `go.mod` file. You will add the ax-go
dependency in Step 3, after there is code that imports it.

## Step 2 — Wire `ax.Execute`

Create a file named `main.go` with the following contents. Read the comments —
they point out the two or three lines that matter.

```go
package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	// The import PATH ends in "ax-go", but the package is named "ax".
	// Refer to everything as ax.Something.
	"github.com/rshade/ax-go"
)

// greeting is the payload that will appear under "data" in the envelope.
// Use a struct, never a map: structs give you a stable, deterministic field
// order that agents can rely on across runs.
type greeting struct {
	Message string `json:"message"`
	Name    string `json:"name"`
	Times   int    `json:"times"`
	Mode    string `json:"mode"`
}

func main() {
	var name string
	var times int

	root := &cobra.Command{
		Use:   "greeter",
		Short: "Greet someone the agent-friendly way",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, _ := ax.ModeFromContext(cmd.Context())
			payload := greeting{
				Message: "hello",
				Name:    name,
				Times:   times,
				Mode:    mode.String(),
			}
			// ax.NewEnvelope attaches the standard metadata (trace_id,
			// idempotency_key, and so on); ax.WriteJSON writes strict,
			// minified JSON to stdout.
			return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), payload))
		},
	}

	root.Flags().StringVar(&name, "name", "world", "who to greet")
	root.Flags().IntVar(&times, "times", 1, "how many greetings the caller intends to send")

	// ax.Execute wraps Cobra: it resolves the output mode, adds the
	// --format/--dry-run/--idempotency-key flags, injects the __schema
	// command, and returns a deterministic exit code.
	os.Exit(ax.Execute(context.Background(), root, ax.WithVersion("0.1.0")))
}
```

:::note[Why the import looks odd]
`import "github.com/rshade/ax-go"` gives you a package called `ax`, not
`ax-go`. That mismatch is intentional and is the single most common thing
newcomers trip on — the code uses `ax.Execute`, `ax.NewEnvelope`, and so on.
:::

## Step 3 — Fetch the dependencies and run

Let Go resolve ax-go (and Cobra) from the imports you just wrote:

```bash
go mod tidy
```

Now run your command, asking for the machine format:

```bash
go run . --format=json --name Ada
```

You will see a single line of JSON on `stdout`:

```json
{"data":{"message":"hello","name":"Ada","times":1,"mode":"json"},"meta":{"trace_id":"...","span_id":"...","idempotency_key":"..."}}
```

That is the envelope. `data` is your payload; `meta` is the standard metadata
ax-go added for you. The `idempotency_key` was auto-generated because you did
not pass one — you will use that in Step 7.

:::tip[Human vs. machine output]
Drop `--format=json` and run `go run . --name Ada`. In an interactive terminal
the mode resolves differently (you will see `"mode":"human"`). The `--format`
flag, the `AGENT_MODE` environment variable, and TTY detection decide the mode,
in that order. Agents set `--format=json` (or `AGENT_MODE`) and get the same
bytes every time.
:::

## Step 4 — Prove stream separation

The core promise of ax-go is that `stdout` carries **only** the machine
payload. Everything else — logs, diagnostics, errors — goes to `stderr`. Prove
it by sending each stream to its own file:

```bash
go run . --format=json --name Ada > out.json 2> err.log
cat out.json
```

`out.json` contains exactly one clean line of JSON — nothing else. Any
telemetry or diagnostic noise landed in `err.log`. This is why an agent can
pipe your `stdout` straight into a JSON parser without a sanitizing step.

To make the split visible, add a log line to your command. Insert this just
before the `payload :=` line in `RunE`:

```go
logger := ax.NewLogger(
	cmd.Context(),
	ax.WithLoggerWriter(cmd.ErrOrStderr()),
	ax.WithLoggerLabels(ax.Labels{Application: "greeter", Version: "0.1.0"}),
)
defer func() { _ = ax.Flush(context.Background(), logger) }()

logger.Info(cmd.Context()).Str("event", "greeted").Str("name", name).Msg("handled greet")
```

Run the split again:

```bash
go run . --format=json --name Ada > out.json 2> err.log
```

`out.json` is still just the envelope. The structured log line — correlated
with the same `trace_id` — is now in `err.log`. You never have to choose
between observability and clean machine output; the streams keep them apart.

:::caution[Never write logs to stdout]
Writing to `cmd.OutOrStdout()` is for the payload alone. Use
`cmd.ErrOrStderr()` (as the logger above does) for everything else. Mixing them
is the fastest way to break an agent that is parsing your output.
:::

## Step 5 — Return a real error

Agents need failures to be as structured as successes. Add a validation check
at the very top of `RunE`, before the logger:

```go
if times < 1 {
	return ax.NewError(
		cmd.Context(),
		"validation_error",
		"times must be at least 1",
		ax.WithActionableFix("pass --times with a positive integer"),
		ax.WithErrorExitCode(ax.ExitValidation),
	)
}
```

Now build a real binary and trigger the error. Build it rather than using
`go run` here: `go run` reports *its own* exit code (`1`) for any failure and
prints the program's real code only as text (`exit status 2`), which would hide
the very contract you are trying to see.

```bash
go build -o greeter .
./greeter --format=json --times=0
echo "exit code: $?"
```

You will see a structured error envelope on `stderr` and a deterministic exit
code:

```json
{"error_code":"validation_error","message":"times must be at least 1","trace_id":"...","tool":"greeter","version":"0.1.0","schema_version":"1.0.0","actionable_fix":"pass --times with a positive integer"}
```

```console
exit code: 2
```

Exit code `2` always means "validation / bad input" in ax-go. The
`actionable_fix` field tells the caller — human or agent — exactly how to
recover. You return an `*ax.Error`; the framework serializes it, routes it to
`stderr`, and maps it to the right exit code.

:::note[The exit-code contract]
`0` success · `1` internal error · `2` validation · `3` network/timeout ·
`4` auth/permission. You chose `2` here with `ax.WithErrorExitCode(ax.ExitValidation)`.
:::

## Step 6 — Discover it like an agent would

You never wrote a `__schema` command, but you have one. `ax.Execute` injected
it. Ask the binary you just built to describe itself:

```bash
./greeter __schema
```

The JSON on `stdout` describes the command tree, every flag (including the
`--format`, `--dry-run`, and `--idempotency-key` flags ax-go added), types, and
your `0.1.0` version. This is how an agent learns to drive your CLI without a
human writing an integration.

There is an MCP-flavored adapter too:

```bash
./greeter __schema --as=mcp
```

That emits an MCP-tool-compatible description — the groundwork for wrapping your
CLI as an MCP server with no per-command effort.

## Step 7 — Flip the agent-safety flags

Two more flags came for free. First, idempotency. Pass your own key and watch
it surface in the envelope's metadata:

```bash
./greeter --format=json --name Ada --idempotency-key=demo-key-123
```

```json
{"data":{"message":"hello","name":"Ada","times":1,"mode":"json"},"meta":{"trace_id":"...","span_id":"...","idempotency_key":"demo-key-123"}}
```

The key you passed is echoed back under `meta.idempotency_key`, so an agent
retrying a call can prove it is the same logical request. (When you omit it,
ax-go generates one — that is the auto-generated key you saw in Step 3.)

Second, dry-run. Add `--dry-run` and the envelope reports it:

```bash
./greeter --format=json --name Ada --dry-run
```

The metadata now includes `"dry_run":true`. Your greeter has no side effects,
so nothing changes — but for a command that *does* write, `--dry-run` produces
the same envelope shape while suppressing the side effect, so an agent can
preview an action safely.

## What you learned

You built a CLI that, on top of ordinary Cobra, now:

- writes a **deterministic JSON envelope** to `stdout` and nothing else;
- keeps **logs and errors on `stderr`**, correlated by `trace_id`;
- returns a **structured `ax.Error`** with an actionable fix and a deterministic
  **exit code**;
- **describes itself** via `__schema` and `__schema --as=mcp`;
- honors **`--format`, `--dry-run`, and `--idempotency-key`** without extra code.

## Where to go next

- **How-to:** [Expose your command tree with `__schema`](/ax-go/guides/expose-schema/)
  — go deeper on self-description and MCP.
- **Explanation:** [Why Agentic Experience?](/ax-go/explanation/why-agentic-experience/)
  — the reasoning behind determinism, stream separation, and self-description.
- **Reference:** [Sources](/ax-go/sources/) — provenance of the AX discipline
  ax-go implements.
