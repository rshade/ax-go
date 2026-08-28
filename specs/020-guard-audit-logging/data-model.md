# Data Model: Default-On Audited Guard/Perform

**Feature**: `020-guard-audit-logging` | **Date**: 2026-08-26

This feature adds no data types, envelope fields, or persistent state. The "model"
is the behavioral contract of four functions, one context helper, and the shape of
the audit log lines. Truth tables are the authoritative spec for the
implementation and the table-driven tests.

## Exported surface (package `ax`)

```go
// Guard runs effect unless dry-run is active in ctx. Signature UNCHANGED from
// feature 012. On a real run it now ALSO emits the default audit lines described
// below, unless the call site opted out via WithAudit(ctx, false).
func Guard(ctx context.Context, effect func(context.Context) error) (bool, error)

// Perform runs commit when dry-run is inactive, or the read-only rehearse preview
// when dry-run is active. Signature UNCHANGED from feature 012. On a real run it
// now ALSO emits the default audit lines described below, unless the call site
// opted out via WithAudit(ctx, false).
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error

// GuardWithAudit is Guard with a caller-supplied description carried into every
// audit log line instead of the generic default label.
func GuardWithAudit(ctx context.Context, description string, effect func(context.Context) error) (bool, error)

// PerformWithAudit is Perform with a caller-supplied description carried into
// every audit log line instead of the generic default label.
func PerformWithAudit(ctx context.Context, description string, rehearse, commit func(context.Context) error) error

// WithAudit returns a context that enables or disables the default audit lines
// for Guard/Perform/GuardWithAudit/PerformWithAudit real-run invocations made
// through it. Mirrors WithDryRun's shape exactly. Absence of a prior WithAudit
// call means enabled (see AuditEnabled truth table below).
func WithAudit(ctx context.Context, enabled bool) context.Context
```

### Callback type

Unchanged from feature 012: all four entry points take work as
`func(context.Context) error`, invoked synchronously with the same `ctx` passed
to the entry point (carrying cancellation, deadlines, mode, dry-run, and audit
state). No goroutines, no timeouts, no `recover`.

## `AuditEnabled` resolution (context helper)

| Prior `WithAudit` call on this context chain | `auditEnabledFromContext(ctx)` |
|---|---|
| none | `true` (default-on) |
| `WithAudit(ctx, true)` | `true` |
| `WithAudit(ctx, false)` | `false` |
| `WithAudit(ctx, false)` then a nested `WithAudit(childCtx, true)` | `true` for the nested context (standard `context.Value` override-nearest-wins semantics; unaffected by this feature) |

This is read on the **real-run branch only** — it is never consulted on the
dry-run branch (see D5 in `research.md`; the dry-run suppression line is
unconditional).

## `Guard`/`GuardWithAudit` truth table

Both delegate to one unexported `guard(ctx, description, effect)`.
`Guard(ctx, effect)` calls it with `description = ""`; `GuardWithAudit(ctx,
description, effect)` passes `description` through.

| `DryRunFromContext(ctx)` | `effect` | `auditEnabledFromContext(ctx)` | Action | Dry-run suppression log? | Audit lines? | Returns |
|---|---|---|---|---|---|---|
| `false` | non-nil | `true` | run `effect(ctx)` | no | **yes** (about-to-run, then succeeded/failed) | `(true, effect's error)` |
| `false` | non-nil | `false` | run `effect(ctx)` | no | no | `(true, effect's error)` |
| `false` | `nil` | any | none | no | no | `(false, nil)` |
| `true` | non-nil | any | **skip** | **yes** | no | `(false, nil)` |
| `true` | `nil` | any | none | no | no | `(false, nil)` |

Invariants (carried forward from feature 012, unchanged):

- Under dry-run, `effect` is **never** invoked (FR-006).
- `err` is the unmodified error from `effect`; `errors.Is`/`errors.As` against it
  keep working (FR-005).
- `executed` is `true` **iff** `effect` was actually called.

New invariants (this feature):

- Audit lines fire **only** on the real, non-nil-effect row, and **only** when
  `auditEnabledFromContext(ctx)` is `true` (FR-001/FR-002/FR-003/FR-009).
- The dry-run suppression log column is identical to feature 012's table in every
  row — the audit-enabled axis has zero effect on it (FR-006, D5).

## `Perform`/`PerformWithAudit` truth table

Both delegate to one unexported `perform(ctx, description, rehearse, commit)`.
`Perform(ctx, rehearse, commit)` calls it with `description = ""`;
`PerformWithAudit(ctx, description, rehearse, commit)` passes `description`
through. `rehearse`'s outcome is shown as `ok` (returns nil) or `err` (returns
non-nil):

| `DryRunFromContext(ctx)` | `rehearse` | `commit` | `auditEnabledFromContext(ctx)` | Action | Dry-run suppression log? | Audit lines? | Returns |
|---|---|---|---|---|---|---|---|
| `false` | any | non-nil | `true` | run `commit(ctx)` | no | **yes** | `commit`'s error |
| `false` | any | non-nil | `false` | run `commit(ctx)` | no | no | `commit`'s error |
| `false` | any | `nil` | any | none | no | no | `nil` |
| `true` | `nil` | non-nil | any | none (pure skip) | **yes** | no | `nil` |
| `true` | `nil` | `nil` | any | none | no | no | `nil` |
| `true` | `ok` | non-nil | any | run `rehearse`, **skip** `commit` | **yes** | no | `nil` |
| `true` | `ok` | `nil` | any | run `rehearse` | no | no | `nil` |
| `true` | `err` | non-nil | any | run `rehearse` | no | no | `rehearse`'s error |
| `true` | `err` | `nil` | any | run `rehearse` | no | no | `rehearse`'s error |

Invariants (carried forward from feature 012, unchanged):

- `commit` is invoked **only** when dry-run is inactive (FR-005/FR-006).
- Under dry-run, `rehearse` (when non-nil) runs and its error is returned
  unchanged (SC unchanged from feature 012).
- The real path **ignores** `rehearse`; the dry-run path **never** runs `commit`.
- A nil `rehearse` under dry-run is an intentional pure skip.
- The dry-run suppression line fires only when a real `commit` would have run
  (non-nil) **and** the rehearsal did not itself fail.

New invariants (this feature):

- Audit lines fire **only** on the real, non-nil-`commit` row, and **only** when
  `auditEnabledFromContext(ctx)` is `true` (FR-001/FR-002/FR-003/FR-009).
- Every dry-run row's suppression-log column is identical to feature 012's table
  — the audit-enabled axis never changes dry-run behavior (FR-006, D5).

## Audit log lines (FR-001, FR-002, FR-004, FR-007)

Emitted on the real-run, non-nil-effect/commit, audit-enabled path only, to
`stderr`, via `ax.NewLogger(ctx)`:

| Aspect | About-to-run line | Succeeded line | Failed line | Abnormal-termination line |
|---|---|---|---|---|
| Emitted | before invoking `effect`/`commit` | after a nil-error return | after a non-nil-error return | when `effect`/`commit` panics (via deferred check) |
| Level | Info | Info | Error | Error |
| Message (constant) | `"ax: about to run effect"` | `"ax: effect succeeded"` | `"ax: effect failed"` | `"ax: effect did not return normally"` |
| Field `ax_helper` | `"Guard"` or `"Perform"` (string) | same | same | same |
| Field `description` | caller's description, or `""` for the plain entry points (string, always present) | same | same | same |
| Field `error` | — | — | the returned error, via `.Err(err)` | — (no error value available) |
| Fields `trace_id`, `span_id` | added automatically by the existing `tracingHook` | same | same | same |

Constraints:

- Built only from constants and ZeroLog field methods — no user-controlled or
  effect-derived string is ever formatted into the message (no log forging, no
  PII; FR-007).
- Exactly two lines per audited real invocation (about-to-run + exactly one of
  succeeded/failed/abnormal-termination) — never zero, never more than two, on
  the audited path. A panicking effect/commit is treated as an
  abnormal-termination outcome recorded by a deferred check (not via `recover`),
  emitting the third distinct message `"ax: effect did not return normally"` at
  Error level, so the panic propagates to the caller unchanged while the audit
  log still observes and records it.
- Does not alter the machine envelope or its byte-for-byte determinism (SC-002 /
  the extended `TestEnvelopeDeterministicUnderDryRun`): logs are `stderr`, the
  envelope is `stdout`.

## Relationship to existing state (unchanged from feature 012)

- `contract.Metadata.DryRun` and `contract.MetadataFromContext` continue to stamp
  `dry_run: true` from the context — this feature does **not** touch the envelope
  (FR-008).
- `ax.WithDryRun`/`ax.DryRunFromContext` (over `contract`) remain the sole dry-run
  state plumbing; this feature only reads it, exactly as feature 012 did.
- `logDryRunSkip` (dry-run suppression line) is entirely unmodified — same
  function, same behavior, same tests.

## Exit-code mapping

Unchanged from feature 012. All four entry points map **no** exit code
themselves; they return the caller's `effect`/`commit`/`rehearse` error verbatim
(FR-005), and the caller maps it through the existing `ax.Error`/`ErrorExitCode`
machinery. A skipped side effect returns a `nil` error and therefore the success
exit code `0`.
