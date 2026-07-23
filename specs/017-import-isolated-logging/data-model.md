# Phase 1 Data Model: Import-Isolated Logging Package

**Feature**: `017-import-isolated-logging` | **Date**: 2026-07-22

This feature moves types rather than introducing domain data. The "model" is
the type graph, its package placement, and the visibility rules that make the
boundary compile.

## Placement overview

| Entity | Package | Visibility | Moves? |
|---|---|---|---|
| `Labels` | `internal/logcore` | exported | moved from `ax` |
| `Logger` | `internal/logcore` | exported | moved from `ax` |
| `Config` | `internal/logcore` | exported | renamed from `loggerConfig` |
| `Option` | `internal/logcore` | exported | renamed from `LoggerOption` |
| `Sink` | `internal/logcore` | exported | renamed from `logSink` |
| `LabelSanctioner` | `internal/logcore` | exported | **new** |
| `flusher` | `internal/logcore` | unexported | moved from `ax` |
| `zerologLogger` | `internal/logcore` | unexported | moved from `ax` |
| `tracingHook` | `internal/logcore` | unexported | moved from `ax` |
| `applyLabels` | `internal/logcore` | unexported | moved from `ax` |
| `lokiWriter` | `ax` (root) | unexported | **stays** |

## Entities

### Labels

The bounded set of low-cardinality descriptors attached to every log line and
eligible for promotion to Loki stream labels.

| Field | Type | Notes |
|---|---|---|
| `Environment` | `string` | emitted as `environment` |
| `Application` | `string` | emitted as `application` |
| `Host` | `string` | emitted as `host` |
| `Version` | `string` | emitted as `version` |

**Validation rules**:

- Empty fields are omitted entirely, not emitted as empty strings
  (`applyLabels` behavior; preserved exactly).
- The set is closed. Adding a field is a public-surface change requiring its
  own feature (Principle VIII's cardinality split).
- `level` is the fifth sanctioned label but is supplied by zerolog, not by this
  struct.
- `trace_id`, `span_id`, `user_id`, durations, and resource IDs are payload and
  MUST NOT be promoted.

**Field-name constants** (`labelFieldEnvironment` and siblings) move with the
struct so a name change stays a single-site edit.

### Logger

The structured-logging handle. An interface, and per Principle VI a migration
seam only — never a pluggable-backend selector.

| Method | Signature | Notes |
|---|---|---|
| `Debug` / `Info` / `Warn` / `Error` | `(ctx context.Context) *zerolog.Event` | `ctx` carried onto the event so `tracingHook` can read it |
| `WithLabels` | `(labels Labels) Logger` | returns a derived logger |
| `Zerolog` | `() *zerolog.Logger` | escape hatch to the underlying handle |

**Invariants**:

- Emission is `stderr`-bound; the payload stream is never written (FR-008).
- With an active span, every line carries correct `trace_id` and `span_id`.
- With no active span, both carry the zero-value valid hex constants and the
  path allocates nothing (ADR-0004 consequence; see `research.md` R8).
- `WithLabels` carries sinks forward so a later `Flush` still drains them.
- Safe for concurrent use across goroutines.

**Identity requirement**: `ax.Logger`, `logging.Logger`, and
`logcore.Logger` must denote **one** type, via alias declarations (`=`), not
distinct definitions. A value obtained from either surface must satisfy the
other without conversion (FR-005).

### Config

The accumulated construction state. Exported so `loki.go` — which stays in root
`ax` — can register its sink.

| Field | Type | Notes |
|---|---|---|
| `Ctx` | `context.Context` | bounds sink goroutine lifetime to the logger |
| `Writer` | `io.Writer` | primary destination; defaults to `stderr` |
| `Level` | `zerolog.Level` | defaults to info |
| `Labels` | `Labels` | applied to every line |
| `AdditionalSinks` | `[]Sink` | write-through destinations |

**Why exported fields rather than accessors**: `loki.go` takes
`&cfg.Writer` — the field's *address* — so a `WithLoggerWriter` applied later
is still observed by the Loki error path. That aliasing is what makes option
order irrelevant. See `research.md` R5.

**Contained-context note**: `Ctx` on a struct is a deliberate, existing
exception (currently carrying a `//nolint:containedctx`), retained because sink
goroutines must be bound to the logger's lifetime. The lint suppression moves
with the field.

### Option

`func(*Config)` — the functional-option shape.

**Alias chain**: `ax.LoggerOption` = `logging.LoggerOption` = `logcore.Option`.
Identity must be preserved so an option produced by root `ax` (notably
`WithLokiFromEnv`) remains usable wherever an option is accepted.

**Known cosmetic consequence**: the public `logging.LoggerOption` resolves to
`func(*logcore.Config)`, so godoc names an `internal/` type. External callers
still cannot author their own options — exactly as true today, since
`loggerConfig` is unexported — so this is behavior-preserving.

**Predicted non-cosmetic consequence — MEASURED AND DID NOT MATERIALIZE.**

This section originally predicted that `logcore.Config` would become **reachable**
from a gated public package (`logging.LoggerOption` aliases `logcore.Option`,
whose underlying signature names `*logcore.Config`), that `surfacecheck` would
therefore inventory `Config` and its fields under `logging`, and that changing
`Config`'s field set would from then on be reviewed public-surface drift.

Measured during implementation, it does not. `surfacecheck` records an alias by
its **target name** rather than by its expanded underlying type, so
`logging.LoggerOption` renders as
`= github.com/rshade/ax-go/internal/logcore.Option` and the inventory never walks
through to `Config`. The regenerated baseline carries 18 `logging` features and
**zero** `Config` entries. Verified:

```bash
python3 -c "import json;d=json.load(open('internal/cmd/surfacecheck/baseline.json'));\
print([f['id'] for p in d['packages'] if p['path'].endswith('/logging') for f in p['features'] if 'Config' in f['id']])"
# []
```

The consequence is therefore the **opposite** of what was predicted, and in the
harmless direction: adding a field to `logcore.Config` remains a free internal
edit and does not require a baseline regeneration. Nothing about the design
depends on the prediction having been right — it was a warning about a constraint
that turns out not to exist. The **cosmetic** consequence above is unaffected and
still holds: godoc for `logging.LoggerOption` still names an `internal/` type, and
external callers still cannot author their own options.

What the baseline does carry, and what T034 should be read as expecting, is one
group: the `logging` package's own 18 features, including the four promoted
`Labels` fields. Every entry's `configurations` and `profiles` presence sets are
the `"all"` sentinel, since nothing here varies by build tag or platform
(FR-014).

### Sink

A write-through destination that can drain buffered entries at shutdown.

```go
type Sink interface {
    io.Writer
    Drain(ctx context.Context) error
}
```

**Rules**:

- Exported **only** so `lokiWriter`, which lives in package `ax`, can satisfy
  it across the boundary. An unexported method is unsatisfiable from outside
  its defining package (`research.md` R3).
- Never re-exported by the public `logging` package. External registration is
  impossible because `internal/logcore` is unimportable externally (FR-011).
- `Drain` takes a context so shutdown cannot hang.
- All sinks are fanned out with the primary writer via `io.MultiWriter`.

### LabelSanctioner

The optional, separately-asserted capability by which a sink is told which
label pairs may be promoted to stream labels.

```go
type LabelSanctioner interface {
    SanctionLabels(Labels)
}
```

**Why separate from `Sink`**: `specs/007-loki-direct-push/research.md`
(absorbing ADR-0006 D2) requires the sink seam stay fully generic. A future
file-rotator or ring-buffer sink has no label concept and must not be forced to
implement one. Asserted with `if ls, ok := s.(LabelSanctioner); ok` (FR-010).

**This replaces** the two `*lokiWriter` concrete type assertions at
`logger.go:114` and `logger.go:161` — the whole of the Loki decoupling.

### flusher (unexported)

```go
type flusher interface {
    flush(ctx context.Context) error
}
```

Stays unexported because `zerologLogger` moves into `logcore` alongside it;
same-package satisfaction needs no export. `Flush` type-asserts against it and
returns `nil` when the assertion fails.

## Construction sequence

Order is contractual — step 2 must follow step 1 so label sanctioning is
independent of option order.

1. Apply every `Option` to `*Config`.
2. For each `Config.AdditionalSinks`, assert `LabelSanctioner` and call
   `SanctionLabels(cfg.Labels)`.
3. If any sinks exist, wrap `cfg.Writer` and them in `io.MultiWriter`.
4. Build the zerolog logger at `cfg.Level`, apply labels, attach
   `tracingHook`.
5. Return a `zerologLogger` carrying the sink slice.

`WithLabels` repeats steps 2 and 4 against the derived logger's carried sinks.

## Alias graph

The two public surfaces are **siblings over `internal/logcore`**, not a chain.
Both alias the same underlying declaration; neither aliases the other.

```text
                    ┌── logging.Logger        (type alias)
logcore.Logger  ◄───┤
                    └── ax.Logger             (type alias)

                    ┌── logging.Labels
logcore.Labels  ◄───┤
                    └── ax.Labels

                    ┌── logging.LoggerOption
logcore.Option  ◄───┤
                    └── ax.LoggerOption

                    ┌── logging.NewLogger     (thin func, not var)
logcore.New     ◄───┤
                    └── ax.NewLogger

                    ┌── logging.Flush
logcore.Flush   ◄───┤
                    └── ax.Flush
```

**Root `ax` MUST NOT import the public `logging` package.** Identity holds either
way — aliases are transitive — but the direction is load-bearing for two reasons:
the root runtime should depend on an internal package rather than a public one,
and root importing `logging` would make `logging`'s parity test (which imports
`ax` to compare the surfaces) an import cycle. See `research.md` R7.

Functions must remain **functions** at every level. Converting `ax.Flush` or
`ax.NewLogger` to a `var` is classified as breaking by `go-apidiff` and would
fail SC-003.

## What does not move

`lokiWriter`, `WithLokiFromEnv`, `AX_LOKI_URL`, `AX_LOKI_AUTH_TOKEN`,
`labelPair`, `sanctionedLabels`, the push/retry machinery, and the cardinality
allowlist all stay in `loki.go` in root `ax`, unchanged. The only edit is that
`lokiWriter` gains exported `Drain` and `SanctionLabels` methods (renamed from
`drain` and `sanctionLabels`) to satisfy the two `logcore` interfaces.
