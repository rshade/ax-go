# Contract: Public `logging` Package

**Feature**: `017-import-isolated-logging` | **Date**: 2026-07-22

Import path: `github.com/rshade/ax-go/logging`

This is a gated public surface. Every symbol below enters
`internal/cmd/surfacecheck/baseline.json` and is subject to `go-apidiff`.

## Exported surface

```go
package logging

// Type aliases — identity-preserving, NOT new definitions.
type Logger = logcore.Logger
type Labels = logcore.Labels
type LoggerOption = logcore.Option

func NewLogger(ctx context.Context, opts ...LoggerOption) Logger
func WithLoggerWriter(w io.Writer) LoggerOption
func WithLoggerLevel(level zerolog.Level) LoggerOption
func WithLoggerLabels(labels Labels) LoggerOption
func Flush(ctx context.Context, l Logger) error
```

Nothing else is exported. In particular `Sink`, `LabelSanctioner`, and `Config`
are **not** re-exported — that is what makes external backend registration
impossible (FR-011).

## Import isolation contract

`go list -deps github.com/rshade/ax-go/logging` MUST NOT contain any of:

| Forbidden | Why |
|---|---|
| `github.com/rshade/ax-go` (root) | the whole point; root drags the exporter tree |
| `github.com/rshade/ax-go/internal/telemetry` | pulls the OTLP exporter |
| `github.com/rshade/ax-go/internal/mcpserver` | pulls the MCP SDK |
| `go.opentelemetry.io/otel/sdk/...` | 4,518,153 bytes standalone |
| `go.opentelemetry.io/otel/exporters/...` | pulls 66 gRPC packages |
| `go.opentelemetry.io/contrib/instrumentation/...` | otelhttp / otelgrpc |
| `google.golang.org/grpc` | 10,400,009 bytes standalone |
| `google.golang.org/protobuf` | arrives with the exporter |
| `github.com/spf13/cobra` | CLI framework |
| `net/http` | **largest single lever**; pulls `crypto/tls` |
| `crypto/tls` | follows `net/http` |

**Explicitly permitted** (and required):

- `github.com/rs/zerolog` — appears in the exported `Logger` method set
- `go.opentelemetry.io/otel/trace` — the trace **API** only, 0 gRPC packages
- `github.com/rshade/ax-go/contract` — zero-value ID constants
- `github.com/rshade/ax-go/internal/logcore` — the implementation

This differs from the four existing contract packages, which forbid `zerolog`
outright. `internal/testutil/imports.go` therefore needs a per-surface rule set
rather than one shared list.

**Configuration independence**: this contract MUST hold identically under all
four build configurations — default, `ax_no_grpc`, `ax_no_otlp`, and both —
because `logging` never links the trees those constraints decline (FR-014).

## Behavioral contract

| ID | Guarantee |
|---|---|
| L-01 | `NewLogger` never returns nil. |
| L-02 | Default writer is `stderr`; default level is info. |
| L-03 | All output goes to the diagnostic stream; the payload stream is never written. |
| L-04 | With an active span, every line carries correct `trace_id` and `span_id`. |
| L-05 | With no active span, both carry zero-value valid hex constants, and the path allocates nothing. |
| L-06 | Options are order-independent, including interaction with root `ax`'s `WithLokiFromEnv`. |
| L-07 | Empty `Labels` fields are omitted, not emitted empty. |
| L-08 | `WithLabels` returns a derived logger carrying sinks forward. |
| L-09 | Safe for concurrent use, including concurrent `Flush`. |
| L-10 | Output is byte-identical to root `ax` under identical configuration. |
| L-11 | `Flush(ctx, nil)` returns `nil`. |
| L-12 | `Flush` performs no work for consumers of `logging` alone, and its doc comment says so. |

## Cross-surface identity contract

Because the types are aliases, all of the following must compile:

```go
var a ax.Logger = logging.NewLogger(ctx)
var b logging.Logger = ax.NewLogger(ctx)
_ = ax.Flush(ctx, logging.NewLogger(ctx))
_ = logging.Flush(ctx, ax.NewLogger(ctx))
_ = logging.NewLogger(ctx, ax.WithLokiFromEnv())   // option identity
```

The last line is the sharpest test: an option manufactured by root `ax` must be
accepted by the isolated constructor, proving the alias chain is unbroken.

## Documentation contract

- Every exported symbol carries a doc comment (`godoclint` `require-doc`).
- `ExampleNewLogger` exists, compiles, runs, and is registered with
  `make doc-coverage` — `NewLogger` is a primary-API constructor, so this is
  gated, not optional.

  **Gated means package-qualified.** `doccover`'s required-symbol set and its
  `baseline.txt` are keyed on **bare** names today, and root already has an
  `ExampleNewLogger`. Simply pointing the scanner at a second directory would let
  root's example satisfy this requirement, leaving the contract stated but
  unenforced. The requirement is therefore `logging.NewLogger`, qualified, and
  the gate must fail if `logging`'s own example is absent while root's is
  present. See `research.md` R12.
- `WithX` options are demonstrated inside the parent example rather than gated
  individually.
- `Flush`'s doc comment states the no-op reality of L-12 plainly, and that
  sentence is **asserted by a test**, not merely written. `godoclint` gates
  doc-comment presence, never content, so an FR-012 promise carried only by a
  comment is unverified. Root's `documentation_test.go` is the in-repo precedent
  for asserting documentation content.

## Test package placement

`parity_test.go` and `identity_test.go` both import root `ax`, and both live in
the **external** test package `logging_test`. Root `ax` does not import `logging`
(`research.md` R7), so no cycle exists in either direction; the external package
is chosen so the comparison code can never contribute to `logging`'s own
dependency graph, keeping the import-isolation assertion unambiguous.
`parity_test.go` additionally carries **no build tag**, so it runs under all four
configurations — a parity claim verified only by the default build proves
nothing.

## Stability

Additive under Principle XI. No existing symbol changes, so `go-apidiff` must
report no breaking change. Adding this package requires updating **both**
`PublicPackages` in `internal/cmd/surfacecheck/inventory.go` and
`allowedPackages()` in `internal/cmd/apidiff-verdict/main.go` in the same
change; a guard test fails CI if they disagree.
