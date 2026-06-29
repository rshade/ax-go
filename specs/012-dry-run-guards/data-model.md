# Data Model: Agent-safety helpers for --dry-run side-effect suppression

**Feature**: `012-dry-run-guards` | **Date**: 2026-06-28

This feature adds no data types, envelope fields, or persistent state. The "model" is the
behavioral contract of two functions plus the shape of one log line. Truth tables are the
authoritative spec for the implementation and the table-driven tests.

## Exported surface (package `ax`)

```go
// Guard runs effect unless dry-run is active in ctx. When dry-run is inactive it
// executes effect and returns (true, <effect's error>). When dry-run is active it
// skips effect entirely, logs a single suppression line to stderr (only if effect is
// non-nil), and returns (false, nil). A nil effect is a no-op returning (false, nil).
// returns (executed bool, err error); names omitted per the repo's nonamedreturns lint
func Guard(ctx context.Context, effect func(context.Context) error) (bool, error)

// Perform runs commit when dry-run is inactive, or the read-only rehearse preview when
// dry-run is active, so a dry-run surfaces the same validation errors as a real run
// without performing the mutation. When dry-run is active and commit is non-nil it logs
// a single suppression line to stderr. rehearse may be nil to mean a pure skip; a nil
// commit on the real path is a no-op returning nil. Returns the error of whichever
// branch ran, with its wrap chain preserved.
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error
```

### Callback type

Both helpers take work as `func(context.Context) error`. The supplied function receives the
same `ctx` passed to the helper (carrying cancellation, deadlines, mode, trace state). The
helpers invoke it synchronously and add no goroutines, timeouts, or `recover`.

## Guard truth table

| `DryRunFromContext(ctx)` | `effect` | Action | Suppression log? | Returns |
|--------------------------|----------|--------|------------------|---------|
| `false` | non-nil | run `effect(ctx)` | no | `(true, effect's error)` |
| `false` | `nil` | none | no | `(false, nil)` |
| `true` | non-nil | **skip** | **yes** | `(false, nil)` |
| `true` | `nil` | none | no | `(false, nil)` |

Invariants:

- Under dry-run, `effect` is **never** invoked (FR-007).
- `err` is the unmodified error from `effect`; `errors.Is`/`errors.As` against it keep
  working (FR-003). The skip path returns a `nil` error (never a synthesized one).
- `executed` is `true` **iff** `effect` was actually called.

## Perform truth table

`rehearse`'s outcome is shown as `ok` (returns nil) or `err` (returns non-nil):

| `DryRunFromContext(ctx)` | `rehearse` | `commit` | Action | Suppression log? | Returns |
|--------------------------|------------|----------|--------|------------------|---------|
| `false` | any | non-nil | run `commit(ctx)` | no | `commit`'s error |
| `false` | any | `nil` | none | no | `nil` |
| `true` | `nil` | non-nil | none (pure skip) | **yes** | `nil` |
| `true` | `nil` | `nil` | none | no | `nil` |
| `true` | `ok` | non-nil | run `rehearse`, **skip** `commit` | **yes** | `nil` |
| `true` | `ok` | `nil` | run `rehearse` | no | `nil` |
| `true` | `err` | non-nil | run `rehearse` | no | `rehearse`'s error |
| `true` | `err` | `nil` | run `rehearse` | no | `rehearse`'s error |

Invariants:

- `commit` is invoked **only** when dry-run is inactive (FR-004/FR-007).
- Under dry-run, `rehearse` (when non-nil) runs and its error is returned unchanged, so a
  preview fails identically to a real run when the input is invalid (FR-005, SC-003).
- The real path **ignores** `rehearse` (it is a dry-run-only preview), and the dry-run path
  **never** runs `commit`.
- A nil `rehearse` under dry-run is an intentional pure skip equivalent to `Guard` skipping
  (FR-006).
- The suppression line is emitted only when a real `commit` would have run (`commit` is
  non-nil) **and** the preview did not itself fail: a failed `rehearsal` returns its error
  and emits **no** suppression line, because the command already surfaces that error and
  nothing would have proceeded.

## Suppression log line (FR-013)

Emitted on the dry-run skip path only, to `stderr`, via `ax.NewLogger(ctx)` at Info level:

| Aspect | Value |
|--------|-------|
| Stream | `stderr` only (never `stdout`) — Principle I |
| Level | Info (the default-logger level; Debug would be filtered out) |
| Message | constant: `"dry-run: side effect suppressed"` |
| Field `dry_run` | `true` (bool) |
| Field `ax_helper` | `"Guard"` or `"Perform"` (string) |
| Fields `trace_id`, `span_id` | added automatically by the existing `tracingHook` |
| Emitted when | `Guard`: dry-run active AND `effect` non-nil. `Perform`: dry-run active AND `commit` non-nil AND the rehearsal did not fail (rehearse nil or returned nil) |

Constraints:

- Built only from constants and ZeroLog field methods — no user-controlled string is
  formatted into the message (no log forging, no PII; Principle IX / guardrails).
- Exactly one line per skipped guard/commit. A command that guards N side effects under
  dry-run emits N lines.
- The line does not alter the machine envelope or its byte-for-byte determinism (SC-004/
  SC-007): logs are `stderr`, the envelope is `stdout`.

## Relationship to existing state (unchanged)

- `contract.Metadata.DryRun` and `contract.MetadataFromContext` already stamp
  `dry_run: true` from the context — the helpers do **not** touch the envelope (FR-009).
- `ax.WithDryRun` / `ax.DryRunFromContext` (over `contract`) remain the sole dry-run state
  plumbing; the helpers only *read* it (FR-010).

## Exit-code mapping

The helpers map **no** exit code themselves. They return the caller's `effect`/`commit`/
`rehearse` error verbatim; the caller maps it through the existing `ax.Error` / `ErrorExitCode`
machinery exactly as it does today (Principle II). A skipped side effect returns a `nil`
error and therefore the success exit code `0`.
