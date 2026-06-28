# Quickstart: ax-go mcp-server runnable wrapper

**Feature**: 011-mcp-server-runtime | **Spec**: [spec.md](spec.md)

How an adopting CLI exposes itself as a live MCP server with no per-tool work.

## 1. Mount the server subcommand

An ax-go-based CLI mounts the `mcp-server` subcommand onto its root, exactly the way it
already relies on the auto-mounted `__schema`. Mounting is an explicit opt-in (the server
executes commands, so it is not auto-mounted):

```go
package main

import (
    "context"
    "os"

    "github.com/spf13/cobra"

    "github.com/rshade/ax-go"
    "github.com/rshade/ax-go/mcp"
)

func main() {
    root := newRootCommand() // your existing Cobra tree

    // Opt in: expose `mycli mcp-server`.
    root.AddCommand(mcp.NewCommand(root))

    os.Exit(ax.Execute(context.Background(), root))
}
```

## 2. Run it as a local (stdio) server

```bash
mycli mcp-server            # speaks MCP over stdio; logs go to stderr
```

An MCP client configured to spawn `mycli mcp-server` as a subprocess will:

1. complete the `initialize` handshake (server name + injected version),
2. call `tools/list` and receive every non-hidden command as a tool,
3. call `tools/call` and receive the command's machine payload as the result.

## 3. Run it over streamable HTTP (loopback by default)

```bash
mycli mcp-server --transport=http                      # binds 127.0.0.1 (loopback)
mycli mcp-server --transport=http --addr=0.0.0.0:8080 --allow-non-loopback
```

A non-loopback bind is **fail-closed**: without `--allow-non-loopback` the server refuses
to start (exit 2). ax-go provides the brake (no accidental network exposure of a
command-executing server) but holds no credentials — put authn/authz in front of the
endpoint at deployment.

## 4. Embed without a subcommand

For full control, call `Serve` directly:

```go
err := mcp.Serve(ctx, root,
    mcp.WithTransport(mcp.TransportHTTP),
    mcp.WithHTTPAddr("127.0.0.1:8080"),
    mcp.WithVersion(version),
)
```

## What you get (and don't)

| Guarantee | Behavior |
|-----------|----------|
| Discovery | All non-hidden commands become tools automatically (reuses `BuildMCPSchema`); `__schema` and `mcp-server` are excluded. |
| Execution | `tools/call` runs the command in machine/JSON mode and returns its verbatim `stdout` bytes. |
| Errors | A non-zero exit returns a structured tool error (the `ax.Error` envelope); the server keeps serving. |
| Safety | `--dry-run` and `--idempotency-key` flow through; HTTP defaults to loopback. |
| Streams | Protocol I/O stays on the transport channel; logs stay on `stderr`. |
| Tracing | Each call is a span; the caller's W3C trace is continued when present. |
| Not included (MVP) | Incremental MCP streaming/progress notifications; an auth flow; a standalone `cmd/` launcher. |

## Verify

```bash
go test -race ./mcp/... ./internal/mcpserver/...   # unit + integration + concurrency
make doc-coverage                                  # ExampleXxx on mcp.Serve / mcp.NewCommand
go run ./internal/cmd/apidiff-verdict check-packages  # mcp is in the public allowlist
```

The `examples/integration` CLI mounts `mcp.NewCommand` and is the runnable reference
instance for this feature.
