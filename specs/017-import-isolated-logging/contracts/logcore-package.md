# Contract: `internal/logcore` Package

**Feature**: `017-import-isolated-logging` | **Date**: 2026-07-22

Import path: `github.com/rshade/ax-go/internal/logcore`

**Not a public surface.** The Go toolchain forbids any module outside
`github.com/rshade/ax-go` from importing it. That restriction is what carries
Principle VI's no-pluggable-backend guardrail once `Sink` must be exported.

`logcore` is not gated directly by `surfacecheck` or `go-apidiff`. It does take
an explicit `covercheck` floor calibrated to its measured coverage, per the
established convention, rather than resting on the 25% default.

**`Config` is not baseline-gated.** It is named in `Option`'s signature, so
godoc for `logging.LoggerOption` mentions this internal type, but
`surfacecheck` inventories aliases by target name rather than expanded
signature and therefore records zero `Config` entries. Field-set discipline is
review and convention; adding a field does not require a baseline regeneration.
See `data-model.md` § Option (measured outcome).

## Exported surface

```go
package logcore

type Labels struct {
    Environment, Application, Host, Version string
}

type Logger interface {
    Debug(ctx context.Context) *zerolog.Event
    Info(ctx context.Context) *zerolog.Event
    Warn(ctx context.Context) *zerolog.Event
    Error(ctx context.Context) *zerolog.Event
    WithLabels(labels Labels) Logger
    Zerolog() *zerolog.Logger
}

type Config struct {
    Ctx             context.Context
    Writer          io.Writer
    Level           zerolog.Level
    Labels          Labels
    AdditionalSinks []Sink
}

type Option func(*Config)

type Sink interface {
    io.Writer
    Drain(ctx context.Context) error
}

type LabelSanctioner interface {
    SanctionLabels(Labels)
}

func New(ctx context.Context, opts ...Option) Logger
func WithWriter(w io.Writer) Option
func WithLevel(level zerolog.Level) Option
func WithLabels(labels Labels) Option
func Flush(ctx context.Context, l Logger) error
```

Unexported and never leaving the package: `zerologLogger`, `flusher`,
`tracingHook`, `applyLabels`, and the `labelField*` name constants.

## Why each export exists

Every export must be justified — the default is unexported.

| Symbol | Justification |
|---|---|
| `Labels`, `Logger`, `Option`, `New`, `With*`, `Flush` | re-exported by public `logging` and root `ax` |
| `Config` | named in `Option`'s signature |
| `Config.AdditionalSinks` | `loki.go` (package `ax`) appends to it |
| `Config.Ctx`, `Config.Writer` | `loki.go` reads them; `Writer`'s **address** is taken |
| `Sink` | `lokiWriter` (package `ax`) must satisfy it |
| `Sink.Drain` | an unexported method is unsatisfiable across packages |
| `LabelSanctioner` / `SanctionLabels` | same, for label promotion |

`flusher` is deliberately **not** exported: `zerologLogger` lives in the same
package, so same-package satisfaction applies. This is the one place the
unexported-interface idiom still works, and it is used.

## Allowed dependencies

A closed set. Anything else is a defect.

| Dependency | Purpose |
|---|---|
| stdlib (`context`, `errors`, `io`, `os`) | core |
| `github.com/rs/zerolog` | the backend, per Principle VI |
| `go.opentelemetry.io/otel/trace` | **API only** — `SpanContextFromContext` |
| `github.com/rshade/ax-go/contract` | `ZeroTraceID` / `ZeroSpanID` |

Forbidden, and asserted: the OTel SDK, any OTel exporter, gRPC, protobuf,
Cobra, `net/http`, `crypto/tls`, root `ax`, and any other `internal/` package
of ax-go. `logcore` MUST NOT import root `ax` — that would reintroduce the
import cycle the feature exists to break.

## Behavioral contract

| ID | Guarantee |
|---|---|
| C-01 | `New` applies every `Option` before sanctioning labels on any sink, making option order irrelevant. |
| C-02 | `New` sanctions labels on each `AdditionalSinks` entry that satisfies `LabelSanctioner`; entries that do not are left alone, never rejected. |
| C-03 | With sinks present, output fans out through `io.MultiWriter(Writer, sinks...)`. |
| C-04 | `tracingHook` stamps `trace_id` and `span_id` on every enabled event. |
| C-05 | With no active span, the zero-value hex constants are used and no allocation occurs. |
| C-06 | Level-filtered events never construct, so the hook never runs for them. |
| C-07 | `applyLabels` omits empty fields entirely. |
| C-08 | `WithLabels` re-sanctions on carried sinks and returns a derived logger. |
| C-09 | `Flush` drains each sink in order, joining errors with `errors.Join`. |
| C-10 | `Flush` returns `nil` for a nil logger or one not satisfying `flusher`. |
| C-11 | No `panic` on any path (Principle IX). |
| C-12 | No mutable package-level state, no `init()` (Principle X). |
| C-13 | `logcore` contains no Loki-specific identifier (FR-010). |

## Contract with `loki.go` (package `ax`)

`lokiWriter` must satisfy both interfaces, which requires renaming two methods:

| Today | After | Reason |
|---|---|---|
| `drain(ctx) error` | `Drain(ctx) error` | cross-package interface satisfaction |
| `sanctionLabels(Labels)` | `SanctionLabels(Labels)` | same |

Both are unexported types' methods on an unexported type, so **no public
surface changes** — `lokiWriter` itself is never exported.

`loki.go` continues to own: the push URL, auth token, channel and goroutine
lifecycle, retry and fail-open diagnostics, the `labelPair` sanctioned set, and
the five-label cardinality allowlist. None of it moves.

## Test obligations

- Table-driven tests for option application and order independence.
- A fake `Sink` and a fake `LabelSanctioner` proving C-02 — specifically that a
  sink which does **not** implement `LabelSanctioner` is handled without error.
  This is the regression guard for the generic-seam requirement (FR-010).
- Benchmark preserving the allocation contract of C-05.
- A test asserting C-13 by scanning for Loki identifiers.
- `race`-enabled concurrency test for C-09 against concurrent emission.
