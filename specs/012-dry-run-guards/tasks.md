---

description: "Task list for feature 012-dry-run-guards"
---

# Tasks: Agent-safety helpers for --dry-run side-effect suppression

**Input**: Design documents from `/specs/012-dry-run-guards/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/public-api.md, quickstart.md

**Tests**: REQUIRED. Constitution Principle VII (Test-First Discipline) is NON-NEGOTIABLE
for this repo — every new behavior/exported function lands its failing test first. Test
tasks therefore lead each story.

**Organization**: Tasks are grouped by user story. The two helpers share `guard.go` and the
private `logDryRunSkip`, so stories are independently *testable* but `Guard` (US1) and
`Perform` (US2) edit the same file and are sequenced, not file-parallel against each other.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 (Setup/Foundational/Polish carry no story label)
- Exact file paths are included in each task.

## Path Conventions

Single Go module at the repository root (package `ax`). New code lives in `guard.go` /
`guard_test.go`; examples in `example_test.go`; the canonical demo in
`examples/integration/main.go`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Make test files compile so behavior tests fail on assertions, not compilation.

- [X] T001 Create `guard.go` (package `ax`) with compile-only stubs for exported `Guard(ctx context.Context, effect func(context.Context) error) (bool, error)` and `Perform(ctx context.Context, rehearse, commit func(context.Context) error) error`, plus an unexported `logDryRunSkip(ctx context.Context, helper string)` stub, each with a placeholder doc comment. Stubs return zero values / no-op so the package builds. File: `guard.go`

**Checkpoint**: `go build ./...` is green; the new symbols exist but do nothing yet.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Test scaffolding shared by US1 and US2. No production behavior is implemented here.

**⚠️ CRITICAL**: Must complete before US1/US2 test tasks.

- [X] T002 Add `guard_test.go` (package `ax_test`) test scaffolding shared by US1/US2: a `recordingFn(err error) (fn func(context.Context) error, ran *bool)` factory that records invocation and returns the supplied error; `dryRunCtx()` / `realCtx()` context builders over `ax.WithDryRun`; a package `errSentinel`; and a non-parallel `captureStderr(t, func())` helper that swaps `os.Stderr` for an `os.Pipe`, runs the closure, restores it, and returns the captured bytes. File: `guard_test.go`

**Checkpoint**: `go test ./... -run TestGuardScaffold` (or `go vet`) compiles; stories can begin.

---

## Phase 3: User Story 1 - Guard a side effect against dry-run in one call (Priority: P1) 🎯 MVP

**Goal**: `ax.Guard` runs an effect normally but suppresses it under dry-run, reports whether
it ran, and logs one stderr suppression line on skip.

**Independent Test**: Call `Guard` with an effect that records whether it ran, once on
`realCtx()` and once on `dryRunCtx()`; assert it ran / reported `executed==true` for real and
did not run / reported `false` / returned nil for dry-run, with one stderr line on the skip.

### Tests for User Story 1 (write first, verify they FAIL)

- [X] T003 [US1] Write table-driven `TestGuard` in `guard_test.go` covering all 4 truth-table rows (real+effect → ran, `(true, errSentinel)`; real+nil → `(false, nil)`; dry-run+effect → not-run, `(false, nil)`; dry-run+nil → `(false, nil)`); assert effect runs ONLY on the real path and error passthrough via `errors.Is(err, errSentinel)`. Verify it FAILS against the T001 stub. File: `guard_test.go`
- [X] T004 [US1] Write `TestGuardSuppressionLogged` in `guard_test.go` using `captureStderr`: under dry-run with a non-nil effect assert exactly one JSON line containing `"dry_run":true`, `"ax_helper":"Guard"`, and the message `dry-run: side effect suppressed`; assert NO line on the real path and NO line when effect is nil under dry-run; assert nothing is written to stdout. Verify it FAILS. File: `guard_test.go`

### Implementation for User Story 1

- [X] T005 [US1] Implement `Guard` in `guard.go` per `data-model.md` (run effect on the real path returning `(true, effect's err)`; on dry-run skip the effect and return `(false, nil)`, calling `logDryRunSkip(ctx, "Guard")` only when `effect != nil`; nil effect is a no-op). Implement `logDryRunSkip` to emit the FR-013 stderr line via `ax.NewLogger(ctx).Info(ctx).Bool("dry_run", true).Str("ax_helper", helper).Msg("dry-run: side effect suppressed")`. Replace the placeholder doc comment with a contract-style doc comment (dry-run gating, the stderr line, nil handling, error passthrough, no exit-code mapping). File: `guard.go`

**Checkpoint**: `TestGuard` and `TestGuardSuppressionLogged` pass; US1 is independently shippable (MVP).

---

## Phase 4: User Story 2 - Faithful dry-run preview via rehearse/commit (Priority: P2)

**Goal**: `ax.Perform` runs the real `commit`, or a read-only `rehearse` preview under dry-run
that surfaces the same validation errors without mutating, logging one stderr line when a real
commit was skipped.

**Independent Test**: Provide a mutating `commit` and a non-mutating `rehearse`; on `realCtx()`
assert `commit` ran and `rehearse` did not; on `dryRunCtx()` assert `rehearse` ran, `commit`
did not, and an invalid input yields the same error in both modes.

### Tests for User Story 2 (write first, verify they FAIL)

- [X] T006 [US2] Write table-driven `TestPerform` in `guard_test.go` covering all 6 truth-table rows from `data-model.md` (real+commit; real+nil-commit; dry-run+rehearse+commit; dry-run+rehearse+nil-commit; dry-run+nil-rehearse+commit; dry-run+nil-nil); assert `commit` runs ONLY on the real path, `rehearse` runs ONLY on the dry-run path and is ignored on real, nil handling is a no-op returning nil, and error passthrough via `errors.Is` (commit error real, rehearse error dry-run). Verify it FAILS. File: `guard_test.go`
- [X] T007 [US2] Write `TestPerformRehearsalParity` (one invalid input → `rehearse` under dry-run and `commit` under real both return `errSentinel`, and `commit` is never called under dry-run — SC-003) and `TestPerformSuppressionLogged` (using `captureStderr`: one stderr line with `"ax_helper":"Perform"` only when dry-run AND `commit != nil`; none otherwise; nothing on stdout) in `guard_test.go`. Verify they FAIL. File: `guard_test.go`

### Implementation for User Story 2

- [X] T008 [US2] Implement `Perform` in `guard.go` per the truth table: on the real path run `commit` (if non-nil) and return its error, ignoring `rehearse`; on dry-run run `rehearse` (if non-nil) and return its error, never running `commit`, and call `logDryRunSkip(ctx, "Perform")` only when `commit != nil`. Reuse the `logDryRunSkip` from T005. Add the contract-style doc comment. File: `guard.go`

**Checkpoint**: all `TestGuard*` and `TestPerform*` pass; both helpers work independently.

---

## Phase 5: User Story 3 - Consistent envelope and a discoverable canonical example (Priority: P3)

**Goal**: The envelope keeps surfacing `dry_run: true` regardless of helper used; verified
examples teach the API; the integration command demonstrates `Perform` canonically.

**Independent Test**: Run a guarded command under dry-run and assert the success envelope
carries `dry_run: true` and is byte-identical to a real run apart from documented
non-deterministic fields; confirm each helper's example compiles and runs.

### Tests for User Story 3 (write first, verify they FAIL/run)

- [X] T009 [P] [US3] Add verified `ExampleGuard` and `ExamplePerform` to `example_test.go` (package `ax_test`) with `// Output:` blocks showing real vs dry-run behavior (use a fixed/zeroed context so output is deterministic). Ensure they execute under `go test`. File: `example_test.go`
- [X] T010 [P] [US3] Write `TestEnvelopeDeterministicUnderDryRun` in `guard_test.go`: for both a real context and a dry-run context (zeroed trace/idempotency state), FIRST run a no-op side effect through `ax.Guard` (and `ax.Perform`) on that context, THEN build `ax.NewEnvelope` from the same context, marshal both, and assert byte-identical output except the `dry_run` field (SC-004). Routing through the helper before building the envelope proves exercising a helper never perturbs the envelope (FR-009) and matches US3's "run a guarded command under dry-run" independent test. File: `guard_test.go`

### Implementation for User Story 3

- [X] T011 [US3] Refactor `examples/integration/main.go` `newPatchConfigCommand` to call `ax.Perform(ctx, rehearse, commit)` where `rehearse` wraps the existing in-memory rehearsal (formerly `dryRunPatchConfig`) and `commit` calls `ax.PatchConfigFile`; delete the hand-rolled `if ax.DryRunFromContext(...) { ... } else { ... }` branch. This before/after refactor is the demonstration of SC-001 (one `Perform` call replaces the multi-line conditional). File: `examples/integration/main.go`
- [X] T012 [US3] Update the `examples/integration` tests so the dry-run path tolerates the new FR-013 stderr suppression line and still asserts the stdout envelope carries `dry_run:true` and is otherwise unchanged (SC-007 end-to-end: one stderr line, zero added stdout bytes). File: `examples/integration/main_test.go`

**Checkpoint**: examples run; the integration command demonstrates `Perform`; envelope stays consistent.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T013 [P] Document `Guard`/`Perform` in `README.md` under the agent-safety section with a short before/after snippet adapted from `quickstart.md`; run `markdownlint` on `README.md`. File: `README.md`
- [X] T014 [P] Review the contract-style doc comments on `Guard` and `Perform` against `godoclint require-doc` (presence is gated; ensure they state dry-run gating, the stderr line, nil handling, error passthrough, and that they map no exit code). File: `guard.go`
- [X] T015 Run the full gate suite and fix any failures: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./...`, `make doc-coverage` (confirms `ExampleGuard`/`ExamplePerform` present — no `internal/cmd/doccover/baseline.txt` edit), and `make cover-check` (root `github.com/rshade/ax-go` floor 80% satisfied by `guard.go` truth-table coverage). Note: the `go test -race ./...` run includes the existing `__schema` golden tests, which enforce FR-010 — adding any new flag/env/field would change `__schema` output and fail the golden, so FR-010 needs no dedicated task.
- [X] T016 Confirm no public-API governance action is needed: `Guard`/`Perform` are additive to the already-allowlisted root `ax` package, so `go-apidiff` reports no incompatible change — no `internal/cmd/apidiff-verdict` allowlist edit, no `breaking-change-approved` label; the change rides a `feat:` Conventional Commit (pre-v1.0 `0.MINOR.0`).

> **No ADR-retirement task.** This feature is governed by Constitution Principle IV
> (Agent-Safety Primitives), not an ADR. `research.md` records Decision Records Absorbed = N/A.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: no dependencies — start immediately.
- **Foundational (T002)**: depends on T001 — BLOCKS all user-story test tasks.
- **US1 (T003–T005)**: depends on T002. The MVP.
- **US2 (T006–T008)**: depends on T002; reuses `logDryRunSkip` implemented in T005 (if US1 is not done first, implement `logDryRunSkip` as part of T008). Edits the same `guard.go`/`guard_test.go` as US1 → sequence after US1.
- **US3 (T009–T012)**: depends on `Guard` (T005) and `Perform` (T008) existing.
- **Polish (T013–T016)**: depends on all desired user stories being complete.

### Within Each User Story

- Test tasks are written and verified FAILING before the implementation task.
- US1: T003, T004 (tests) → T005 (impl).
- US2: T006, T007 (tests) → T008 (impl).
- US3: T009, T010 (tests/examples) → T011 (refactor) → T012 (integration test update).

### Parallel Opportunities

- US3: **T009 (`example_test.go`)** and **T010 (`guard_test.go`)** touch different files → run in parallel.
- Polish: **T013 (`README.md`)** and **T014 (`guard.go` doc review)** touch different files → run in parallel.
- US1 and US2 are NOT file-parallel against each other (both edit `guard.go` + `guard_test.go`); they are independently testable but sequenced.

---

## Parallel Example: User Story 3

```bash
# Different files — safe to run together:
Task: "Add ExampleGuard/ExamplePerform in example_test.go"          # T009
Task: "Add TestEnvelopeDeterministicUnderDryRun in guard_test.go"   # T010
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. T001 (Setup) → T002 (Foundational) → T003–T005 (US1).
2. **STOP and VALIDATE**: `go test -race -run TestGuard ./...` — `Guard` suppresses side
   effects under dry-run and logs to stderr. Shippable on its own.

### Incremental Delivery

1. Setup + Foundational → scaffold ready.
2. US1 (`Guard`) → test → **MVP**.
3. US2 (`Perform`) → test → faithful preview.
4. US3 → examples + canonical integration demo + envelope-consistency proof.
5. Polish → README, doc-comment review, full gate suite, governance confirmation.

---

## Notes

- [P] = different files, no incomplete-task dependency.
- Verify each test FAILS for the right reason before implementing (Principle VII).
- The suppression log is `stderr`-only via `ax.NewLogger`; nothing the helpers do reaches
  `stdout` (Principle I) — the envelope's `dry_run:true` continues to flow automatically.
- Do not add `Guard`/`Perform` to `contract` (FR-008 — import isolation).
- Commit message rides Conventional Commits `feat:` (no changelog hand-edit — release-please owns it).
