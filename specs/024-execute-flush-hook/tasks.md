# Tasks: Execute Shutdown Flush Hook

**Input**: Design documents from `/specs/024-execute-flush-hook/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/public-api.md`, `quickstart.md`

**Tests**: Required by the feature specification and Constitution Principle VII.
Every behavior task begins with a failing test and records the expected failure
before implementation.

**Organization**: Tasks are grouped by independently testable user story. Task
IDs are execution ordered; `[P]` marks work in different files that can proceed
without an incomplete same-file dependency.

## Phase 1: Setup (Shared Verification)

**Purpose**: Confirm the feature pointer, quality checklist, and pre-change
behavior before writing a test.

- [X] T001 Verify the 16/16 requirements checklist in `specs/024-execute-flush-hook/checklists/requirements.md`, confirm `.specify/feature.json` and the managed `AGENTS.md` block point to feature 024, and run the existing focused tests for `execute_test.go` and `examples/integration/`

---

## Phase 2: Foundational (Design Contract)

**Purpose**: Lock the exact public and lifecycle contract that every story uses.

- [X] T002 Cross-check `specs/024-execute-flush-hook/contracts/public-api.md` against `execute.go`, `internal/telemetry/telemetry.go`, `logger.go`, and `internal/logcore/sink.go`, resolving any signature, timeout, defer-order, sanitizer, or nil-safety mismatch in the feature design artifacts before tests begin

**Checkpoint**: The callback signature, last-option-wins behavior, defer order,
separate timeout windows, diagnostic line, and fail-open exit semantics are
unambiguous and implementation-ready.

---

## Phase 3: User Story 1 — Register One Lifecycle-Owned Flush (Priority: P1) 🎯 MVP

**Goal**: A registered callback runs exactly once on every normal `Execute`
return path with a fresh bounded context; omitted/nil and option precedence are
deterministic.

**Independent Test**: Success, classified `RunE` failure, argument failure, and
persistent-pre-run failure each invoke the selected callback exactly once while
retaining their command exit code; absent/final-nil cases invoke nothing; a
canceled command parent does not pre-cancel the fresh callback context.

### Tests for User Story 1

- [X] T003 [US1] Add table-driven lifecycle, nil, last-option-wins, final-nil, and fresh-deadline tests to `execute_test.go`, then run the focused test pattern and verify it fails because `WithFlushFunc` does not exist

### Implementation for User Story 1

- [X] T004 [US1] Add `executeConfig.flushFunc`, the fully documented `WithFlushFunc` including the non-redaction/sensitive-error precondition, the expanded `WithTelemetryShutdownTimeout`/`Execute` lifecycle comments, and the dedicated bounded flush defer in `execute.go`, registered to produce `span.End` → flush → telemetry order while leaving callback errors fail-open
- [X] T005 [US1] Run the User Story 1 focused tests in `execute_test.go` under default and `ax_no_grpc,ax_no_otlp`, confirming exact-once invocation, fresh bounded context, last-option-wins semantics, and unchanged exit codes

**Checkpoint**: A nil-returning callback is lifecycle-owned and works on success
and error paths; callers that omit or clear it retain current behavior.

---

## Phase 4: User Story 2 — Flush Failures Remain Fail-Open and Safe (Priority: P2)

**Goal**: Callback errors are visible as one sanitized stderr diagnostic without
changing stdout, the authoritative error envelope, telemetry cleanup, or exit
code.

**Independent Test**: On both success and classified command failure, an error
containing all ASCII controls and DEL produces exactly one physical
`ax: flush failed:` line with no control characters, byte-identical stdout, and
the original exit code; a callback waiting on its deadline terminates and is
diagnostic-only.

### Tests for User Story 2

- [X] T006 [US2] Add table-driven callback-error stream/exit tests, control-character sanitization assertions, error-envelope-before-diagnostic ordering, a short deadline cancellation case, direct observation that telemetry receives a newly bounded context after the flush exhausts its window, and a flush-panic/telemetry-defer ordering regression to `execute_test.go`, then verify the new diagnostic assertions fail before implementation

### Implementation for User Story 2

- [X] T007 [US2] Implement the exact sanitized `ax: flush failed: <error>\n` branch on the mutex-wrapped configured stderr in `execute.go`, preserving the already-computed return code and keeping telemetry shutdown in its separate earlier-registered defer; keep telemetry shutdown behind a private per-execution seam so the independent second deadline is directly testable without mutable package state
- [X] T008 [US2] Run the User Story 1 and 2 focused tests in `execute_test.go` under all four supported build-tag configurations and confirm no stdout, error-envelope, exit-code, or telemetry-order regression

**Checkpoint**: Flush success, failure, timeout, and caller panic behavior match
the public contract while command results remain authoritative.

---

## Phase 5: User Story 3 — Copy the Recommended Integration Pattern (Priority: P3)

**Goal**: Canonical code and prose teach Execute-owned draining instead of a
command-local timeout/defer.

**Independent Test**: The integration example and verified `ExampleExecute`
compile/run with a late-bound logger closure, the first-CLI and Loki quickstarts
contain no recommended manual flush defer, and all documentation describes the
same timeout/error behavior.

### Tests and examples for User Story 3

- [X] T009 [P] [US3] Update the verified `ExampleExecute` to exercise `WithFlushFunc` and revise `ExampleFlush` to distinguish direct non-Execute lifecycles in `example_test.go`, then run the example tests and `make doc-coverage`
- [X] T010 [P] [US3] Change `newRootCommand` to return its late-bound flush closure, remove `lokiFlushBudget` and the root `RunE` defer, register `WithFlushFunc` in `runWithEntityID`, and adapt private direct-constructor call sites in `examples/integration/main.go`, `examples/integration/main_test.go`, and `examples/integration/axtest_example_test.go`

### Documentation for User Story 3

- [X] T011 [P] [US3] Update the recommended lifecycle and fail-open guidance in `logger.go`, `loki.go`, and `README.md`; add non-redaction/sensitive-error guidance to the public option contract and README; include the copyable version/logger/Execute snippet
- [X] T012 [P] [US3] Replace manual shutdown wiring and explain the independent bounded context in `docs/src/content/docs/tutorials/build-your-first-cli.md` and `specs/007-loki-direct-push/quickstart.md`; reconcile root-only guidance in `docs/src/content/docs/guides/choose-a-logging-surface.md`
- [X] T013 [P] [US3] Document the lifecycle-owned `WithFlushFunc` evidence in `examples/integration/README.md` and `examples/integration/AUDIT.md`
- [X] T014 [US3] Run all `examples/integration/` tests plus `make build-example` and `make build-example-minimal`, confirming payload/error goldens and stream separation are unchanged

**Checkpoint**: All canonical adoption surfaces teach one closure registered on
`Execute`; direct `Flush` remains documented for other lifecycle owners.

---

## Phase 6: Public Surface and Cross-Cutting Verification

**Purpose**: Record the intentional export, review all generated artifacts, and
run the repository's required gates without lowering a policy budget.

- [X] T015 Add the sorted supported/live/keep-public `func:WithFlushFunc` decision and advance `audited_at` in `specs/023-internalize-helpers/public-surface-audit.json`, then run `make surface-update` and review `internal/cmd/surfacecheck/baseline.json` to confirm it adds only `func:WithFlushFunc` with signature `func(func(context.Context) error) ExecuteOption` and universal presence
- [X] T016 Run `gofmt -s` on changed Go files, execute the feature scenarios in `specs/024-execute-flush-hook/quickstart.md`, and mark every completed task `[X]` in `specs/024-execute-flush-hook/tasks.md`
- [X] T017 Run `make test` and `make validate`, fixing every race, build-tag, format, tidy, and vet failure without weakening tests or changing the feature contract
- [X] T018 Run `make lint` and `make doc-coverage`, fixing every Go/Markdown/action lint or verified-example failure without adding a doccover exemption
- [X] T019 Run `make cover-check`, `make surface-check`, and `make size-check`, fixing failures without lowering coverage floors, changing size budgets, or accepting unintended public-surface drift
- [X] T020 Run `make bench-check`, confirm no tracked benchmark disappeared or exceeded the 5%/+1 budgets, and make no benchmark-policy change because this feature asserts no numeric performance target
- [X] T021 Review `git diff --check`, `git status --short`, and the full diff for scope: no `CHANGELOG.md` edit, no ADR edit/deletion, no payload/schema golden drift, no new dependency, and no unrelated user change; confirm all 21 tasks are complete in `specs/024-execute-flush-hook/tasks.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on T001 and blocks behavior work.
- **User Story 1 (Phase 3)**: Depends on T002; it is the MVP and establishes the
  option/context/defer mechanics used by all later stories.
- **User Story 2 (Phase 4)**: Depends on T004–T005; adds failure diagnostics and
  panic-safe defer ordering without changing callback registration.
- **User Story 3 (Phase 5)**: Depends on T007–T008 so examples document the final
  behavior. T009–T013 can proceed in parallel by file; T014 joins them.
- **Public Surface and Verification (Phase 6)**: T015 depends on the final export;
  T016–T021 are ordered validation checkpoints.

### User Story Dependencies

- **US1 (P1)**: Independent MVP after design lock; delivers lifecycle-owned
  invocation with nil-returning callbacks.
- **US2 (P2)**: Extends US1's callback path with safe visibility on failure; the
  command outcome remains independently testable before and after it.
- **US3 (P3)**: Documents and exercises the complete US1+US2 contract; it does
  not alter runtime semantics.

### Within Each User Story

- T003 must fail for the missing export before T004 changes `execute.go`.
- T006 must fail for the missing diagnostic assertions before T007 implements
  the branch.
- Same-file work remains serial: `execute_test.go` T003/T006 and `execute.go`
  T004/T007.
- Documentation tasks T009–T013 touch distinct files and may proceed together
  after runtime behavior is final.
- Surface artifacts are updated only after source and docs settle.

## Parallel Opportunities

- T009, T010, T011, T012, and T013 are independent file groups after T008.
- Within verification, read-only focused tests for root `ax` and
  `examples/integration` can run concurrently when they do not contend on build
  outputs; make targets remain serial to keep diagnostics attributable.
- No Phase 3/4 same-file task is marked `[P]`; the test-first dependency is
  intentionally serial.

## Parallel Example: User Story 3

```text
Task: "Update ExampleExecute/ExampleFlush in example_test.go"
Task: "Migrate examples/integration source and private tests"
Task: "Update root Go doc comments and README.md"
Task: "Update Starlight and Loki quickstarts"
Task: "Update integration README/AUDIT evidence"
```

## Implementation Strategy

### MVP First (User Story 1)

1. Complete T001–T002.
2. Write T003 and observe the expected compile failure.
3. Implement T004 and validate T005.
4. Stop: a caller can now register a nil-returning bounded lifecycle callback on
   success/error paths without changing existing callers.

### Incremental Delivery

1. US1 adds deterministic callback invocation.
2. US2 adds sanitized fail-open visibility and pins defer safety.
3. US3 migrates canonical examples and documentation.
4. Phase 6 records the public surface and runs the complete policy suite.

## Notes

- No governing ADR means no final ADR-retirement task is generated.
- Do not edit or create `CHANGELOG.md`; release notes come from the eventual
  Conventional Commit.
- Do not add a standalone `ExampleWithFlushFunc`; demonstrate the option inside
  `ExampleExecute`.
- Do not normalize non-positive `WithTelemetryShutdownTimeout` values in this
  feature; preserve the existing Execute contract.
- Do not truncate/redact diagnostics under the name of sanitization; callers
  must avoid sensitive callback errors, and this feature only reuses the current
  control-character policy.
