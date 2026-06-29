# Implementation Plan: Agent-safety helpers for --dry-run side-effect suppression

**Branch**: `012-dry-run-guards` | **Date**: 2026-06-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/012-dry-run-guards/spec.md`

## Summary

Add two small, additive ergonomic helpers to the root `ax` package so command
authors stop hand-rolling `if ax.DryRunFromContext(ctx) { ... } else { ... }` around
every side-effecting operation:

- **`Guard(ctx, effect) (executed bool, err error)`** — runs `effect` unless dry-run is
  active; under dry-run it skips the effect, logs a single suppression line to `stderr`,
  and reports `executed == false`.
- **`Perform(ctx, rehearse, commit) error`** — runs `commit` for real, or runs the
  read-only `rehearse` preview under dry-run (so the same validation errors surface
  without the mutation), logging a suppression line when a real `commit` was skipped. A
  nil `rehearse` means a pure skip.

The dry-run flag is already resolved into `context.Context` and the machine envelope
already stamps `dry_run: true` automatically (`contract.MetadataFromContext`), so this
feature touches neither flag resolution nor envelope shape. Because the helpers emit the
suppression log line via the canonical logger (`ax.NewLogger`), and the import-isolated
`contract` package is forbidden to import the logger, the helpers live in the **root `ax`
package only** — not in `contract`, and not as a thin facade. The repository's
integration command adopts `Perform` as the canonical demonstration in place of its
current hand-rolled dry-run conditional.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: Existing set only — `github.com/rs/zerolog` (via the canonical
`ax.NewLogger`) for the suppression line; `context` from the stdlib. **No new
dependencies.** Reuses `ax.DryRunFromContext` (over `contract.DryRunFromContext`) and the
existing `tracingHook` that stamps `trace_id`/`span_id` on every log line.

**Storage**: N/A — pure control-flow helpers, no persistent state.

**Testing**: `go test -race ./...`, `go vet ./...`, `golangci-lint run`,
`make doc-coverage`, `make cover-check`. Table-driven behavior tests (truth tables for
`Guard`/`Perform`), an `os.Stderr`-capture test for the suppression line (FR-013/SC-007),
a determinism test (byte-identical envelope dry-run vs real, SC-004), and verified
`ExampleGuard`/`ExamplePerform` (FR-012, doc-coverage).

**Target Platform**: Go library consumers on the platforms ax-go/CI already support.

**Project Type**: Go library — two new exported functions in the existing root `ax`
package (new file `guard.go`); no new package.

**Performance Goals**: No numeric targets asserted. The helpers are thin wrappers invoked
at most once per side effect per command (not a hot loop); constructing `NewLogger(ctx)`
on the skip path is a bounded, per-skip cost. No `testing.B` claim is made, so none is
required (Principle VII / X). Helpers add no goroutines, so `-race` is trivially clean.

**Constraints**:

- Stream separation: the suppression line goes to `stderr` only (the `NewLogger` default);
  nothing the helpers do reaches `stdout` (FR-007/FR-013, Principle I).
- No new flags, env vars, or envelope fields; dry-run state read solely from context
  (FR-010).
- Defensive nil handling: a nil `effect`/`commit` is a no-op returning a nil error, never
  a panic (FR-011, Principle IX — no panic in library code).
- Import isolation preserved: helpers live in root `ax`; `contract` is NOT modified and
  gains no logger dependency (FR-008, Principle XI).
- Errors propagate with their wrap chain intact (`%w` / passthrough) so `errors.Is`/`As`
  keep working (FR-003/FR-005, Principle X).

**Scale/Scope**: One new root-package file (`guard.go`) with two exported functions and a
small unexported `logDryRunSkip` helper; one new test file (`guard_test.go`); two new
examples appended to `example_test.go`; an `examples/integration/main.go` refactor of
`newPatchConfigCommand` onto `Perform`; a README agent-safety note. No apidiff-allowlist
edit (root `ax` is already allowlisted; the change is additive) and no doccover
`baseline.txt` edit (examples are added, not exempted).

**Governing ADR(s)**: **N/A.** Agent-safety primitives are governed by Constitution
Principle IV (Agent-Safety Primitives), not an ADR. `research.md` records
"Decision Records Absorbed = N/A"; no ADR-retirement task is required.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | PASS | Suppression line → `stderr` via `NewLogger` default; helpers write nothing to `stdout`; SC-007 captures both streams to prove it. |
| II. Deterministic Output & Exit Codes | PASS | Helpers never touch `stdout`/envelope; `dry_run: true` stamping unchanged; SC-004 asserts byte-identical envelope dry-run vs real; helpers map no exit code themselves and return the caller's error unmodified for the caller to map via `ErrorExitCode`. |
| III. Machine Discoverability via `__schema` | PASS | No new flags/commands → `__schema` output unchanged; golden schema tests stay green. |
| IV. Agent-Safety Primitives | PASS | This feature directly strengthens the `--dry-run` "no side effects" mandate; envelope `dry_run: true` guarantee preserved (FR-009). |
| V. Asymmetric JSON I/O | PASS | No JSON I/O changes; writes remain strict; no Hujson involved. |
| VI. ADR-Governed Scope — Library, Not Application | PASS | Ergonomic library primitives ("ax is the brake, not the engine") — dry-run suppression is squarely brake territory; no orchestration/domain logic added. |
| VII. Test-First Discipline | PASS | Tasks lead with failing table-driven behavior tests, the `os.Stderr`-capture suppression test, the determinism test, and the two `ExampleXxx`; `-race`, `make doc-coverage`, and `make cover-check` are gates. |
| VIII. Observability & ID Discipline | PASS | The suppression line carries `trace_id`/`span_id` via the existing `tracingHook`; no trace↔resource ID mixing; structured fields only (no PII). |
| IX. Security & Resource Safety | PASS | No panic (defensive nil), no unbounded reads, no TLS changes; static log message + ZeroLog field methods → no log forging / no PII; errors wrapped/propagated with `%w`. |
| X. Idiomatic Go & Dependency Minimalism | PASS | No new dependency; `context.Context` first; plain function signatures (≤3 args, functional options unwarranted); no package-level state; no `init()`. |
| XI. Stability & SemVer | PASS | Additive to the already-allowlisted root `ax` package → pre-v1.0 minor (`0.MINOR.0`), `feat:` commit; no `breaking-change-approved` label; no new public package, so the apidiff allowlist needs no edit. |
| XII. Deprecation Lifecycle | PASS | No deprecations or removals (the integration example's private `dryRunPatchConfig` is refactored, not a public deprecation). |

**ADR absorption gate (Constitution §Governance)**: PASS — Governing ADR(s) = N/A;
`research.md` records why (Principle IV governs agent-safety directly). No ADR-retirement
task required in `tasks.md`.

**Post-design re-check**: PASS. Phase 1 artifacts (data-model truth tables, public-API
contract, quickstart) keep the feature additive, root-package-local, import-isolation-safe,
and stream-separated. No new violations introduced; Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/012-dry-run-guards/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions (Decision Records Absorbed = N/A)
├── data-model.md        # Phase 1 — helper signatures, truth tables, log-line shape
├── quickstart.md        # Phase 1 — adopter usage (Guard + Perform)
├── contracts/
│   └── public-api.md    # `ax` package public surface delta + apidiff/stability note
├── checklists/
│   └── requirements.md  # spec quality checklist (from /speckit-specify, re-validated)
└── tasks.md             # Phase 2 — /speckit-tasks (NOT created here)
```

### Source Code (repository root)

```text
guard.go                 # NEW (package ax): Guard, Perform, unexported logDryRunSkip
guard_test.go            # NEW (package ax / ax_test): truth-table behavior tests,
                         #   os.Stderr-capture suppression test, determinism test
example_test.go          # MODIFIED (package ax_test): + ExampleGuard, + ExamplePerform
examples/integration/
└── main.go              # MODIFIED: newPatchConfigCommand uses ax.Perform; the existing
                         #   dryRunPatchConfig becomes the rehearse closure (canonical demo)
README.md                # MODIFIED: agent-safety section documents Guard/Perform
```

Unchanged by design: `contract/` (no `Guard`/`Perform` there — FR-008), the apidiff
allowlist (`internal/cmd/apidiff-verdict`), `internal/cmd/doccover/baseline.txt`, and the
MCP dispatch path (`internal/mcpserver/dispatch.go` already seeds dry-run into context;
served commands compose with the helpers unchanged — research D9).

**Structure Decision**: Single Go library at the module root. The feature is two exported
functions added to the existing root `ax` package via a new `guard.go`, plus tests,
examples, an integration-example refactor, and a README note. No new package, so no
package-layout decision is needed; root-package placement is forced by the logging
requirement (FR-008) and recorded in the spec's Clarifications.

## Complexity Tracking

> No Constitution Check violations. Section intentionally empty.
