# Contract: Public API surface (`mcp` package)

**Feature**: 011-mcp-server-runtime | **Spec**: [../spec.md](../spec.md)

The runnable server is delivered as a new, deliberately thin public package
`github.com/rshade/ax-go/mcp`. Root `ax` gains **no new exported symbols** (FR-023). All
protocol/transport/dispatch mechanics live in `internal/mcpserver/` and are not part of
the public surface.

## Public symbols (package `mcp`)

> Signatures are the intended contract; exact SDK-facing internals are confirmed during
> implementation. Every exported symbol carries a doc comment (godoclint `require-doc`),
> and the primary entry points carry verified `ExampleXxx` (`make doc-coverage`).

```go
// Serve runs an ax-go CLI's command tree as a live MCP server until ctx is
// canceled, returning a non-nil error only on startup/transport failure (never
// on an individual tool-call failure). Stream separation is preserved: MCP
// protocol I/O uses the transport channel; logs/diagnostics go to stderr.
func Serve(ctx context.Context, root *cobra.Command, opts ...Option) error

// NewCommand returns the reserved "mcp-server" Cobra subcommand an adopting CLI
// mounts to expose itself as an MCP server (e.g. `mycli mcp-server`). It is NOT
// auto-mounted by ax.Execute; mounting is an explicit opt-in.
func NewCommand(root *cobra.Command, opts ...Option) *cobra.Command

// Option configures Serve and NewCommand.
type Option func(*options)

// WithTransport selects the transport (TransportStdio default, or TransportHTTP).
func WithTransport(t Transport) Option

// WithHTTPAddr sets the bind address for the HTTP transport. The default is
// loopback; a non-loopback host also requires WithAllowNonLoopback(true).
func WithHTTPAddr(addr string) Option

// WithAllowNonLoopback permits the HTTP transport to bind a non-loopback/public
// interface. Required (fail-closed) before a public bind is accepted.
func WithAllowNonLoopback(allow bool) Option

// WithVersion sets the server version reported in the MCP initialize handshake.
func WithVersion(version string) Option

// Transport identifies an MCP transport.
type Transport int

const (
    TransportStdio Transport = iota // default
    TransportHTTP
)
```

## Behavioral contract for the public surface

- **C-API-1**: `Serve` blocks until `ctx` is canceled or the transport fails; it returns
  `nil` on clean shutdown and a wrapped (`%w`) error on startup/transport failure. It
  never returns because a single tool call failed.
- **C-API-2**: `NewCommand` returns a command named `mcp-server`; running it calls
  `Serve` with the resolved options. The command is excluded from the tool list it serves.
- **C-API-3**: Options are functional and composable; invalid combinations (e.g.
  non-loopback `WithHTTPAddr` without `WithAllowNonLoopback`) fail closed at startup with
  a validation error (exit 2), not a panic.
- **C-API-4**: The package MUST NOT introduce mutable package-level state; all config
  enters through `Serve`/`NewCommand` arguments.
- **C-API-5**: `context.Context` is the first parameter of `Serve` and of every internal
  I/O path (Principle X).

## Stability / apidiff gate (REQUIRED)

Adding `github.com/rshade/ax-go/mcp` as a public package requires updating the public
allowlist or CI fails (AGENTS.md, Constitution Principle XI):

- Add `"github.com/rshade/ax-go/mcp"` to `allowedPackages` in
  `internal/cmd/apidiff-verdict/main.go` (keep the slice sorted/grouped as the file
  already is).
- The `check-packages` guard then expects `mcp` to be public; the `go-apidiff` gate now
  scopes incompatible-change detection to it too.
- This is an **additive** public-surface change → a pre-v1.0 **minor** (`0.MINOR.0`)
  release; no `breaking-change-approved` label is required. Commit as `feat:`.

## Import isolation

- **C-API-6**: `mcp` MAY import `schema` (for `BuildMCPSchema`), `contract` (exit codes,
  error envelope), `internal/mcpserver`, the SDK, and `cobra`. The import-isolated
  contract packages (`contract`, `config`, `schema`, `id`) MUST NOT import `mcp`
  (asserted by their existing `import_isolation_test.go`, extended as needed).
- **C-API-7**: A new `mcp/import_isolation_test.go` asserts the package does not drag the
  contract surfaces into a forbidden runtime graph beyond what is intended.
