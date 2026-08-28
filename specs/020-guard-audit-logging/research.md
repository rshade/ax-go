# Research: Default-On Audited Guard/Perform

**Feature**: `020-guard-audit-logging` | **Date**: 2026-08-26

**Decision Records Absorbed**: **N/A.** This feature is governed directly by
Constitution Principle IV (Agent-Safety Primitives) and Principle VIII
(Observability & ID Discipline) — no ADR governs it. No ADR to absorb or retire.

All Technical-Context unknowns are resolved below; no `NEEDS CLARIFICATION`
remain.

---

## D1 — Exact entry-point signatures

**Decision**: `Guard`/`Perform` keep their existing signatures verbatim. Two new
functions and one new context helper are added, in package `ax`:

```go
func Guard(ctx context.Context, effect func(context.Context) error) (bool, error)                          // UNCHANGED signature
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error                       // UNCHANGED signature

func GuardWithAudit(ctx context.Context, description string, effect func(context.Context) error) (bool, error)
func PerformWithAudit(ctx context.Context, description string, rehearse, commit func(context.Context) error) error

func WithAudit(ctx context.Context, enabled bool) context.Context
```

**Rationale**: `GuardWithAudit`/`PerformWithAudit` match the shapes proposed in the
source GitHub issue (#179) verbatim — nothing in the `/speckit-clarify` session
changed them, only whether `Guard`/`Perform` themselves default into audited
behavior. `WithAudit(ctx, enabled bool) context.Context` mirrors
`WithDryRun(ctx, dryRun bool) context.Context` exactly (same shape, same package,
same "boolean toggle carried in context" idiom already established for dry-run,
approval, and idempotency-key state) — inventing a different shape for the same
kind of problem would be needless novelty (Principle X). A positive-framed name
(`WithAudit(ctx, false)` = "without audit") was chosen over a negatively-framed
`WithAuditDisabled(ctx, true)` to avoid a double-negative at call sites.

**Alternatives considered**:

- Add a variadic `opts ...GuardOption` parameter to `Guard`/`Perform` instead of a
  context helper. Rejected: still requires a new exported option type and a
  functional-options call convention just to carry one boolean, and — unlike a
  context signal — a variadic parameter cannot be threaded down through nested
  helper calls the way `context.Context` already is throughout this codebase.
  `WithDryRun`/`DryRunFromContext` is the established precedent for exactly this
  "toggle carried across a call boundary" problem; reusing it is simpler and more
  consistent than adding a second mechanism (Principle X: no invented convention
  where an established one already fits).
- Change `Guard`'s signature to accept a `description string` directly. Rejected
  during `/speckit-clarify`: breaks every existing call site at compile time, not
  just at the behavior level — a strictly bigger break than intended, and it would
  make the "no code change required for the default" half of FR-001 impossible.
- A single combined `Guard(ctx, effect, opts ...)` replacing all four entry points.
  Rejected: the named `GuardWithAudit`/`PerformWithAudit` read more clearly at call
  sites than an options-call for the common "I want a description" case, and the
  clarify session explicitly asked for named functions to stay.

## D2 — Default-on / opt-out semantics (absence defaults to enabled)

**Decision**: The context getter treats an **absent** value as `true` (audited),
not the Go zero value:

```go
func auditEnabledFromContext(ctx context.Context) bool {
	enabled, ok := ctx.Value(auditContextKey).(bool)
	if !ok {
		return true
	}
	return enabled
}
```

**Rationale**: This is the one place this feature deliberately departs from the
`DryRunFromContext`-style pattern (which returns the bool zero value, `false`, on
absence — the correct default there, since "no dry-run flag set" must mean "run for
real"). Here, "no `WithAudit` call was ever made" is the overwhelmingly common case
— every pre-existing call site — and FR-001 requires that exact case to be audited
by default. An `ok`-checked default is a small, explicit, well-precedented Go
idiom (same shape as `IdempotencyKeyFromContext`'s "empty key → not set" check), so
no new pattern is introduced, just a different default value than dry-run's.

**Alternatives considered**: Store the audited-by-default state as an explicit
`true` at every call site via a wrapping `ax.Execute`/`ax.NewLogger` change.
Rejected: would require touching `execute.go`'s context construction and every
caller's entry path for no benefit over a getter that simply defaults `true` on
absence.

## D3 — Where the opt-out context key lives

**Decision**: The `auditContextKey` type, `WithAudit`, and
`auditEnabledFromContext` all live in root `ax` (`guard.go`), **not** in
`contract`, even though `WithDryRun`/`DryRunFromContext`/`WithApproval`/
`WithIdempotencyKey` all live in `contract` with thin root wrappers.

**Rationale**: Those four existing context values are read by
`contract.MetadataFromContext` to stamp the machine envelope (`dry_run`,
`idempotency_key`) — they are cross-cutting metadata multiple packages need. The
audit opt-out is read by exactly one thing: the unexported audit-logging helpers in
`guard.go` itself. It never touches the envelope, `__schema`, or any other
package's behavior. Feature 012 already established the precedent that
logger-dependent code stays root-only because `contract` is forbidden to import the
logger (Principle VI/import isolation); this feature's opt-out isn't
logger-dependent by itself, but keeping it colocated with the only code that reads
it is simpler than splitting a two-function pair across packages for no consumer on
the `contract` side.

**Alternatives considered**: Put `WithAudit`/`AuditEnabledFromContext` in
`contract` for consistency with the other four context helpers. Rejected: would
require exporting a getter (`AuditEnabledFromContext`) that has no external
caller — `contract`'s own `MetadataFromContext` has no reason to read it — adding
public surface to an import-isolated package for zero consumers. If a future
feature needs `contract` or another package to read audit-enabled state, that is a
new spec, not a speculative addition here (Principle X: no design for hypothetical
future requirements).

## D4 — Audit log-line shape and levels

**Decision**: Three constant messages, structured fields only, shared by all four
entry points via unexported helpers:

```go
// before invoking a non-nil effect/commit on a real run
NewLogger(ctx).Info(ctx).
    Str("ax_helper", helper).        // "Guard" or "Perform"
    Str("description", description). // "" for the plain (non-audited) entry points
    Msg("ax: about to run effect")

// after a nil-error return
NewLogger(ctx).Info(ctx).
    Str("ax_helper", helper).
    Str("description", description).
    Msg("ax: effect succeeded")

// after a non-nil-error return
NewLogger(ctx).Error(ctx).
    Str("ax_helper", helper).
    Str("description", description).
    Err(err).
    Msg("ax: effect failed")
```

**Rationale**: Matches `logDryRunSkip`'s existing discipline exactly — a constant
message plus ZeroLog field methods only, so no caller-supplied or effect-derived
string is ever formatted into the message itself (FR-007, no log forging). The
`description` field is always present (empty string, not omitted) for consistent
grepability across every audit line regardless of which entry point produced it —
a consumer greps for `ax_helper` and `description` the same way whether the call
used `Guard` or `GuardWithAudit`. `Info` for the about-to-run/succeeded lines
matches the default logger level (`InfoLevel`, confirmed in
`internal/logcore/logcore.go`); `Error` for the failed line is standard severity
practice and remains visible at the same default level (`Error` > `Info` in
zerolog's ordering). `trace_id`/`span_id` are added automatically by the existing
`tracingHook`, so every audit line correlates with the active span (Principle
VIII) — the same mechanism `logDryRunSkip` already relies on, untouched by this
feature.

**Alternatives considered**:

- A single `Str("phase", "about_to_run"|"succeeded"|"failed")` field with one
  shared message. Rejected: three distinct constant messages are at least as
  greppable as one message plus a phase field, and match the existing convention
  of `logDryRunSkip`'s single fixed message per call site rather than introducing
  a new "phase" vocabulary this codebase doesn't otherwise use.
- Omit the `description` field entirely when empty (conditional `.Str(...)` call).
  Rejected: a field that's sometimes present and sometimes absent is harder for a
  downstream log-processing pipeline to query reliably than one that's always
  present, possibly empty — consistent presence is cheap and worth it here.
- Log the error with `.Str("error", err.Error())` instead of `.Err(err)`.
  Rejected: `.Err()` is zerolog's dedicated, idiomatic error field (`"error"` key,
  correctly serialized), and using it doesn't concatenate anything into the
  message string — no log-forging concern either way, but `.Err()` is the more
  idiomatic choice and matches the field name (`"error"`) a consumer already
  expects from zerolog-based tooling.

## D5 — Dry-run path stays completely untouched

**Decision**: `logDryRunSkip` (the existing suppression-line helper) is not
modified, and the audit-enabled check is never consulted on the dry-run branch —
dry-run behavior is unconditional and independent of `WithAudit`.

**Rationale**: User Story 3 / FR-006 require byte-for-byte dry-run parity
regardless of audit opt-out state. Making the dry-run suppression line itself
respect `WithAudit(ctx, false)` would be a second, silent behavior change nobody
asked for (a caller opting out of *real-run* audit noise has no reason to also
lose the dry-run suppression signal, which long predates this feature and has its
own contract from feature 012). Keeping the two paths fully independent is the
simplest way to guarantee SC-005.

**Alternatives considered**: Let `WithAudit(ctx, false)` also suppress
`logDryRunSkip`. Rejected for the reason above — conflates two independently
useful signals ("don't audit my real runs" vs. "don't tell me what dry-run
skipped") that have no reason to be coupled.

## D6 — Shared internal implementation (avoiding duplication across 4 entry points)

**Decision**: `Guard` and `GuardWithAudit` both delegate to one unexported
`guard(ctx, description string, effect func(context.Context) error) (bool, error)`;
`Perform` and `PerformWithAudit` both delegate to one unexported
`perform(ctx, description string, rehearse, commit func(context.Context) error) error`.
`Guard`/`Perform` call their unexported counterpart with `description = ""`;
`GuardWithAudit`/`PerformWithAudit` pass the caller's description through
unchanged.

**Rationale**: Guarantees the two entry points for each primitive can never drift
in dry-run handling, nil handling, wrap-chain preservation, or audit-line shape —
exactly the kind of duplication bug a two-function-pair-per-primitive design
invites if each is implemented independently. Mirrors feature 012's own D5
decision (`logDryRunSkip` shared by both `Guard` and `Perform`), extended one
level further now that there are two audited entry points per primitive instead of
one.

**Alternatives considered**: Implement `GuardWithAudit` by calling `Guard`
internally and threading the description through a context value instead of a
shared unexported function. Rejected: needlessly indirect — a context value for a
single call's description is a worse fit than a plain function parameter, and it
would make the audit-enabled context check ambiguous about scope (is a
context-carried description still active for a *nested* `Guard` call inside the
effect closure?).

## D7 — Truth-table extension (opt-out axis)

**Decision**: Extend feature 012's truth tables (see `data-model.md`) with a third
axis — `auditEnabledFromContext(ctx)` — applied only on the real-run branch. The
existing dry-run rows are unchanged from feature 012 (D5 above). Full tables live
in `data-model.md`.

**Rationale**: Makes the interaction between dry-run and opt-out explicit and
testable rather than left to be inferred: dry-run always wins (its own suppression
line fires regardless of the opt-out setting), and among real runs, the opt-out
setting is the sole determinant of whether the two audit lines fire.

## D8 — Breaking-change classification and the apidiff gap

**Decision**: This feature ships as `feat!:`/`BREAKING CHANGE:` with the
`breaking-change-approved` label applied to the PR, even though `go-apidiff` will
report the Go-surface diff as purely additive (two new functions, one new
exported function — nothing removed, renamed, or re-typed). The
`breaking-change-approved` label and the commit trailer are added **deliberately
by the author**, not because CI's automated apidiff gate demands them — it won't,
for this specific class of change.

**Rationale**: Constitution Principle XI's breaking-change definition explicitly
includes "semantic change" of the Go API surface as breaking, not only
add/remove/rename/re-type. `Guard`/`Perform` undergo exactly that: identical
signatures, materially different real-run behavior (two new unconditional stderr
writes per call). `go-apidiff` is a structural diffing tool — it cannot detect a
behavior-only change, so nothing in the automated CI pipeline will force this
label onto the PR. Documenting the gap here, in `contracts/public-api.md`, and as
an explicit `tasks.md` step is the only guardrail against this breaking change
accidentally shipping unlabeled as a plain `feat:` — which would violate
Principle XI's process even though CI would stay green.

**Evidence this is a real break, not a theoretical one**: `guard_test.go` today
contains `TestGuardSuppressionLogged` and `TestPerformSuppressionLogged`, each of
which explicitly asserts real-run stderr is **empty**
(`if strings.TrimSpace(out) != "" { t.Errorf("real run emitted a suppression
line: %q", out) }`). Both assertions become **false** the moment `Guard`/`Perform`
default into audited behavior — concrete proof this touches existing, already
tested behavior, not just a hypothetical downstream consumer. Both tests are
corrected (not deleted — their nil-effect/nil-commit and dry-run assertions in the
same tests remain valid and unchanged) as part of this feature's own task list.

**Migration note (for the `BREAKING CHANGE:` commit trailer and README)**: "Guard
and Perform now emit two structured stderr log lines (`ax: about to run effect`,
then `ax: effect succeeded`/`ax: effect failed`) around every real (non-dry-run)
invocation by default. Dry-run behavior is unchanged. To restore the previous
silent behavior for a specific call site, wrap its context with
`ax.WithAudit(ctx, false)`."

**Alternatives considered**: Rely on `go-apidiff` alone and skip the manual label.
Rejected: would silently violate Principle XI's process for a change the
constitution's own text classifies as breaking — the whole point of writing this
decision down is to make sure a future reviewer (human or agent) doesn't defer to
a green CI check that was never designed to catch this class of change.

## D9 — MCP dispatch path

**Decision**: No change to `internal/mcpserver/dispatch.go`, same as feature 012's
D9. It already seeds dry-run into the per-call context; served commands calling
`Guard`/`Perform`/`GuardWithAudit`/`PerformWithAudit` compose with the new default
behavior automatically. The audit lines land on the server's `stderr`, never on
the tool result (the command's `stdout`), so stream separation holds across the
MCP boundary exactly as it already does for the dry-run suppression line.

**Rationale**: No new coupling between `internal/mcpserver` and the new symbols is
required; the dispatcher's context propagation already carries whatever
`WithAudit` state a served command's own `RunE` sets before calling
`Guard`/`Perform`.

## D10 — Stability, apidiff, doc-coverage, and coverage gates

**Decision**: Root `ax` is already on the apidiff allowlist
(`internal/cmd/apidiff-verdict`) and already public — no allowlist edit needed.
`GuardWithAudit`/`PerformWithAudit` are top-level entry points on par with
`Guard`/`Perform`, so each gets its own gated `ExampleXxx`
(`ExampleGuardWithAudit`, `ExamplePerformWithAudit`) satisfying `make
doc-coverage`'s primary-API requirement with no `baseline.txt` exemption needed
(it is already empty). `WithAudit` is a `WithX`-style functional-option-shaped
helper, so per Principle VII it is demonstrated **inside** one of those two parent
examples rather than gated with its own `ExampleWithAudit`. The root `ax` package
coverage floor is **85%** (current `covercheck` table); the extended truth-table
and stderr-capture tests keep `guard.go` at parity with its pre-existing near-100%
coverage.

**Rationale**: Keeps every CI gate green through the correct mechanism (examples
added, not exempted; no new package, so no allowlist churn) while the one gate
that CANNOT catch this feature's real risk — the behavior-only breaking change —
is called out explicitly in D8 rather than assumed covered.

## Open questions

None. All Technical-Context items are resolved; Constitution Check passes pre- and
post-design. The single material scope decision (default-on vs. opt-in-only) was
resolved in `/speckit-clarify` and is recorded in `spec.md`'s Clarifications
section; this document elaborates the resulting technical design, it does not
revisit the decision itself.
