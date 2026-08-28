# Implementation Plan: Default-On Audited Guard/Perform

**Branch**: `020-guard-audit-logging` | **Date**: 2026-08-26 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/020-guard-audit-logging/spec.md`

## Summary

Give `Guard`/`Perform` a built-in, structured "about to run / succeeded / failed"
audit trail around every real (non-dry-run) invocation, **on by default** — no code
change required at existing call sites — while preserving today's `Guard`/`Perform`
Go signatures exactly:

- `Guard`/`Perform` gain no new parameters. On a real run, each now unconditionally
  emits an "about to run" line before the effect/commit and a "succeeded"/"failed"
  line after, using a generic (description-less) label, unless the call site opts
  out.
- `WithAudit(ctx, enabled bool) context.Context` is a new context helper —
  mirroring `WithDryRun`'s exact shape — that lets a call site suppress (or
  re-enable) this default for effects where it's inappropriate (very-high-frequency
  internal effects, non-consequential effects).
- `GuardWithAudit(ctx, description, effect)` / `PerformWithAudit(ctx, description,
  rehearse, commit)` are new named entry points that carry a caller-supplied
  description into every audit line instead of the generic default — the upgrade
  path for a genuinely meaningful audit trail (e.g. "destroy stack prod-east").
- Dry-run behavior is untouched: the existing single suppression line
  (`logDryRunSkip`) remains the only output on the dry-run path, byte-for-byte,
  regardless of which entry point or opt-out state is in play.

Because the real-run stderr output of `Guard`/`Perform` changes for every existing
caller — even though neither function's Go signature moves — this is a deliberate
breaking **behavior** change (Constitution Principle XI's "semantic change" clause),
not a purely additive one. It ships as `feat!:`/`BREAKING CHANGE:` with the
`breaking-change-approved` label, in the same release as the new named variants and
the opt-out. `go-apidiff` itself will NOT flag this (it diffs Go signatures, not
behavior), so the label and commit trailer must be applied deliberately — this is
called out explicitly in `research.md` and `contracts/public-api.md` so it isn't
missed in review.

This is the sole reason `Guard`/`Perform` and their new siblings stay in the root
`ax` package (never `contract`): all four entry points call `ax.NewLogger`, and the
import-isolated `contract` package is forbidden to import the logger — exactly the
precedent already set by feature 012.

## Technical Context

**Language/Version**: Go 1.26.7

**Primary Dependencies**: Existing set only — `github.com/rs/zerolog` (via the
canonical `ax.NewLogger`) for the audit lines; `context` from the stdlib. **No new
dependencies.** Reuses `ax.DryRunFromContext`, the existing `tracingHook` that
stamps `trace_id`/`span_id` on every log line, and the existing `logDryRunSkip`
(unchanged) for the dry-run path.

**Storage**: N/A — pure control-flow helpers plus a context value, no persistent
state.

**Testing**: `go test -race ./...`, `go vet ./...`, `golangci-lint run`,
`make doc-coverage`, `make cover-check`. Table-driven behavior tests (truth tables
for `Guard`/`Perform`/`GuardWithAudit`/`PerformWithAudit` × dry-run × opt-out),
`os.Stderr`-capture tests for the new default audit lines (FR-001/FR-002),
adversarial-description log-forging tests (FR-007), a determinism test extended
from feature 012's `TestEnvelopeDeterministicUnderDryRun` (SC-005), and verified
`ExampleGuardWithAudit`/`ExamplePerformWithAudit` (`WithAudit` demonstrated inside
those examples per Principle VII's "`WithX` options inside a parent example" rule,
not gated individually). Two EXISTING tests
(`TestGuardSuppressionLogged`/`TestPerformSuppressionLogged` in `guard_test.go`)
currently assert real-run stderr is **empty** — those assertions are now wrong by
design and MUST be updated to assert the new default audit lines instead, as part
of this feature (not left to bit-rot).

**Target Platform**: Go library consumers on the platforms ax-go/CI already
support.

**Project Type**: Go library — extends the existing root `ax` package (`guard.go`);
no new package.

**Performance Goals**: No numeric targets asserted. Every real-run `Guard`/`Perform`
call now constructs `NewLogger(ctx)` and writes to `stderr` twice (about-to-run +
outcome) by default — up from zero times before this feature. This is a real,
deliberate cost for callers who don't opt out; `guard.go`/`Guard`/`Perform` are not
in the `internal/cmd/benchcheck` tracked-benchmark list, so this ships without a
`testing.B` claim (Principle VII/X), but the cost and the opt-out escape hatch for
high-frequency call sites are documented in `quickstart.md` and the migration note.

**Constraints**:

- Stream separation: all audit lines go to `stderr` only (the `NewLogger` default);
  nothing reaches `stdout` (FR-008, Principle I).
- No new flags, env vars, or envelope fields; the opt-out state is read solely from
  context, mirroring `WithDryRun`/`DryRunFromContext` (FR-003).
- `Guard`/`Perform`'s existing signatures do not change — the opt-out is a context
  signal, not a parameter, so no existing call site fails to compile (Assumptions).
- Defensive nil handling carries forward unchanged: a nil `effect`/`commit`
  produces no audit lines (FR-009, Principle IX — no panic in library code).
- No-log-forging discipline extends to the new lines: every value (description,
  error) goes through a ZeroLog field method, never string-formatted into the
  message (FR-007, Principle IX).
- Errors propagate with their wrap chain intact (`%w` / passthrough) through all
  four entry points (FR-005, Principle X).
- This is a governed breaking change (Principle XI): `feat!:`/`BREAKING CHANGE:`
  commit, `breaking-change-approved` label, and a migration note covering the
  `WithAudit(ctx, false)` opt-out (FR-010).

**Scale/Scope**: `guard.go` gains two new exported functions
(`GuardWithAudit`/`PerformWithAudit`), one new exported context helper
(`WithAudit`), an unexported context getter, and unexported audit-logging helpers
shared by all four public entry points; `guard_test.go` gains new truth-table and
stderr-capture tests and two existing tests are corrected; `example_test.go` gains
two new `ExampleXxx`; `README.md` documents the default behavior, the named
variants, and the opt-out. No new package; no apidiff-allowlist edit (root `ax` is
already allowlisted); no `doccover baseline.txt` edit (examples are added, not
exempted).

**Governing ADR(s)**: **N/A.** Same as feature 012 — agent-safety/observability
primitives are governed directly by Constitution Principles IV and VIII, not an
ADR. `research.md` records "Decision Records Absorbed = N/A"; no ADR-retirement
task is required.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | PASS | All audit lines → `stderr` via `NewLogger`; nothing added to `stdout`; stderr-capture tests cover both defaulted and opted-out real runs. |
| II. Deterministic Output & Exit Codes | PASS | No envelope/`stdout` changes; `TestEnvelopeDeterministicUnderDryRun`-style test extended to prove exercising the new default audit path never perturbs the envelope (SC-005); helpers still map no exit code themselves. |
| III. Machine Discoverability via `__schema` | PASS | No new flags/commands → `__schema` output unchanged. |
| IV. Agent-Safety Primitives | PASS | Strengthens operational visibility around the existing `--dry-run` safety primitive without touching its `dry_run: true` guarantee. |
| V. Asymmetric JSON I/O | PASS | No JSON I/O changes. |
| VI. ADR-Governed Scope — Library, Not Application | PASS | Still squarely "brake" territory — an audit trail around an existing safety primitive, not new orchestration/domain logic. |
| VII. Test-First Discipline | PASS | Tasks lead with failing table-driven tests (4 entry points × dry-run × opt-out), stderr-capture tests for the new default lines, a log-forging adversarial test, and the two new `ExampleXxx`; the two pre-existing tests whose assertions are now wrong are corrected as part of this feature, not silently left broken. |
| VIII. Observability & ID Discipline | PASS | This IS the observability feature — audit lines carry `trace_id`/`span_id` via the existing `tracingHook`; description/error are structured fields, never labels (no cardinality violation — these are stderr log fields, not Loki labels). |
| IX. Security & Resource Safety | PASS | No panic (defensive nil carried forward); no unbounded reads; static messages + ZeroLog field methods for description/error → no log forging, no PII; errors wrapped/propagated with `%w`. |
| X. Idiomatic Go & Dependency Minimalism | PASS | No new dependency; `context.Context` first; `WithAudit` mirrors the existing `WithDryRun` shape exactly (no invented convention); no package-level state; no `init()`. |
| XI. Stability & SemVer | PASS (with an explicit breaking-change path) | This is a governed pre-v1.0 `0.MINOR.0` **breaking behavior change** (semantic change to `Guard`/`Perform`, not a signature change) — ships `feat!:`/`BREAKING CHANGE:` with the `breaking-change-approved` label, per the Clarifications' explicit decision. `go-apidiff` will report this as additive-only (new functions), so the label/trailer are a deliberate manual step, documented in `contracts/public-api.md` so review doesn't rely on the automated gate alone. |
| XII. Deprecation Lifecycle | PASS | No deprecations or removals. |

**ADR absorption gate (Constitution §Governance)**: PASS — Governing ADR(s) = N/A;
`research.md` records why. No ADR-retirement task required in `tasks.md`.

**Post-design re-check**: PASS. Phase 1 artifacts (data-model truth tables, public-API
contract, quickstart) keep the feature root-package-local, import-isolation-safe,
and stream-separated, and keep the breaking-change classification explicit rather
than papered over by the additive-looking Go diff. No new violations introduced;
Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/020-guard-audit-logging/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions (Decision Records Absorbed = N/A)
├── data-model.md        # Phase 1 — entry-point signatures, truth tables, log-line shapes
├── quickstart.md        # Phase 1 — adopter usage (default behavior, opt-out, named variants)
├── contracts/
│   └── public-api.md    # `ax` package public surface delta + breaking-change/apidiff note
├── checklists/
│   └── requirements.md  # spec quality checklist (from /speckit-specify, re-validated)
└── tasks.md              # Phase 2 — /speckit-tasks (NOT created here)
```

### Source Code (repository root)

```text
guard.go                 # MODIFIED (package ax): Guard/Perform gain default audit
                         #   behavior via shared unexported helpers; + GuardWithAudit,
                         #   PerformWithAudit, WithAudit (exported), unexported
                         #   audit-context getter and audit-logging helpers
guard_test.go            # MODIFIED (package ax_test): new truth-table + stderr-capture
                         #   tests for the 4 entry points × dry-run × opt-out; corrects
                         #   TestGuardSuppressionLogged / TestPerformSuppressionLogged's
                         #   now-wrong "real run emits nothing" assertions
example_test.go          # MODIFIED (package ax_test): + ExampleGuardWithAudit,
                         #   + ExamplePerformWithAudit (WithAudit demonstrated inline)
README.md                # MODIFIED: documents the default audit behavior, the named
                         #   variants, and the WithAudit opt-out; migration note for
                         #   existing Guard/Perform adopters
```

Unchanged by design: `contract/` (the opt-out context key lives in root `ax` only,
same reasoning feature 012 used for the logger — FR stays root-scoped), the apidiff
allowlist (`internal/cmd/apidiff-verdict`), `internal/cmd/doccover/baseline.txt`,
and the MCP dispatch path (`internal/mcpserver/dispatch.go` already seeds dry-run
into context; served commands compose with the new default behavior for free,
exactly as they already do with `Guard`/`Perform` today).

**Structure Decision**: Single Go library at the module root. The feature extends
the existing `guard.go` in the root `ax` package with two new functions, one new
context helper, and default-on internal wiring, plus tests, examples, and a README
update. No new package. Root-package placement is forced by the logger dependency
(same constraint as feature 012, recorded there and reaffirmed in this feature's
`research.md`).

## Complexity Tracking

*No violations — table intentionally empty.*
