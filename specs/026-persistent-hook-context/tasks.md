# Tasks: Agent-Safety Context Reaches Every Command in the Tree

**Input**: Design documents from `/specs/026-persistent-hook-context/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/behavior-contract.md`, `quickstart.md`

**Tests**: Required by the feature specification and Constitution Principle
VII. Every behavior task begins with a failing test and records the expected
failure before implementation.

**Organization**: Tasks are grouped by independently testable user story.
Task IDs are execution ordered; `[P]` marks work in different files that can
proceed without an incomplete same-file dependency. This feature's three
user stories share one underlying implementation (there is no way to ship
User Story 1's fix without also satisfying User Story 2's and User Story
3's guarantees as emergent properties of the same tree walk), so US2 and
US3 contribute dedicated tests against the US1 implementation rather than
additional implementation of their own.

## Phase 1: Setup (Shared Verification)

**Purpose**: Confirm the feature pointer and pre-change (buggy) baseline
before writing a test.

- [X] T001 Verify the 16/16 requirements checklist in `specs/026-persistent-hook-context/checklists/requirements.md`, confirm `.specify/feature.json` and the managed `AGENTS.md` block point to feature 026, and run the existing focused tests for `execute_test.go` to record the pre-change baseline

---

## Phase 2: Foundational (Design Contract)

**Purpose**: Lock the exact defect and fix shape before tests begin.

- [X] T002 Cross-check `specs/026-persistent-hook-context/contracts/behavior-contract.md` and `research.md` D1 against the current `prepareCommand`/`wrapPersistentPreRun` in `execute.go` and the vendored `github.com/spf13/cobra` `command.go`'s `execute()` method, confirming the exact line ranges, the nearest-ancestor dispatch behavior, and that Cobra always passes the invoked command (not the hook-owning ancestor) as the hook's first argument

**Checkpoint**: The exact defect (root-only wrapping), Cobra's dispatch
contract, and the fix shape (tree walk + idempotent per-command wrap) are
unambiguous and implementation-ready.

---

## Phase 3: User Story 1 — A Subcommand Group's Own Setup Hook No Longer Breaks Agent Safety (Priority: P1) 🎯 MVP

**Goal**: For any command in the tree, `ModeFromContext`, `DryRunFromContext`,
`ApprovalFromContext`, and `IdempotencyKeyFromContext` return the same values
whether or not that command or an ancestor declares its own persistent hook,
and `ax.Guard`/`ax.Confirm` behave accordingly.

**Independent Test**: Build a two-level command tree where a subcommand
group declares its own persistent hook, run a leaf command under it with
`--dry-run`, and confirm no `ax.Guard`-wrapped side effect occurs — exactly
as it would with no such hook anywhere in the tree.

### Tests for User Story 1

- [X] T003 [US1] Add a table-driven test to `execute_test.go` covering the five required hook shapes (child `PersistentPreRun`, child `PersistentPreRunE`, both declared on the same command, grandchild-only, parent-and-child both), each asserting `ModeFromContext`/`DryRunFromContext`/`ApprovalFromContext`/`IdempotencyKeyFromContext` match a no-hook baseline row in the same table, plus a `Guard`-wrapped side-effect counter staying at zero under `--dry-run` and `Confirm` returning approved under `--yes` for every shape; run it and verify every non-baseline row fails against current `main`
- [X] T004 [US1] Add a repeated-`Execute`-call test to `execute_test.go` running the same command tree through `ax.Execute` twice and asserting a subcommand's own hook fires exactly once per call (twice total, not more), then verify it fails to compile/assert correctly before the fix (documents the idempotency requirement even though today's un-fixed code happens to tolerate repeat calls via nested closures)

### Implementation for User Story 1

- [X] T005 [US1] In `execute.go`, add the `persistentHookWrappedAnnotation` constant, extract `wrapCommandPersistentPreRun(cmd *cobra.Command, cfg executeConfig)` from the existing per-command body of `wrapPersistentPreRun` (parameterized by `cmd` instead of hardcoded to `root`, using the closure's own invoked-command argument for every flag lookup and context mutation, guarded by the annotation for idempotency), and rewrite `wrapPersistentPreRun` to wrap `root` unconditionally plus any other command in the tree that already declares its own `PersistentPreRun`/`PersistentPreRunE` at wrap time, with a doc comment explaining why (Cobra dispatches only the nearest ancestor's hook, so any self-hooked command must get ax's setup directly)
- [X] T006 [US1] Run the User Story 1 focused tests in `execute_test.go` under default and `ax_no_grpc,ax_no_otlp`, confirming every hook shape now matches the no-hook baseline and the repeated-`Execute` test passes

**Checkpoint**: A command reached through any subcommand persistent-hook
shape gets correct agent-safety context, matching the no-hook baseline.

---

## Phase 4: User Story 2 — An Adopter's Own Hook Keeps Working Exactly as They Wrote It (Priority: P2)

**Goal**: A subcommand group's own persistent hook keeps running exactly
once per invocation with its existing error-handling behavior intact, so
adopters need no code changes.

**Independent Test**: Declare a persistent hook that increments a counter
and, separately, one that returns an error; confirm each still runs exactly
once per invocation and error handling is unchanged.

### Tests for User Story 2

- [X] T007 [US2] Add a table-driven test to `execute_test.go` (across the four `PersistentPreRunE`-capable hook shapes from T003) asserting a subcommand group's own error-returning persistent hook still fails the command with that exact error exactly once — **and captures `DryRunFromContext` inside the erroring hook itself**, asserting it observed the correct dry-run state before deciding to error. Verified empirically via isolated probes (not committed) that a version testing only the error/call-count fails to distinguish pre-fix from post-fix behavior — Cobra's own error propagation is independent of ax's context wrapping — while the context-capturing version fails against unfixed `execute.go` (`sawDryRun=false`, want `true`) and passes against the fix
- [X] T008 [US2] Run the User Story 2 focused tests, confirming the adopter's own hook's error-propagation, exactly-once behavior, and correct pre-error context are all satisfied by the Phase 3 implementation

**Checkpoint**: The fix is a drop-in change from the adopter's point of
view — their own hook's behavior is byte-for-byte unchanged.

---

## Phase 5: User Story 3 — The Most Careful Invocation Is Never the Most Dangerous One (Priority: P3)

**Goal**: `--dry-run --yes` together, against a command reached through a
subcommand hook, produces no side effect and correct envelope metadata,
matching the no-hook baseline.

**Independent Test**: Run `--dry-run --yes` together against a command
under a subcommand group with its own hook and confirm no side effect
occurs and the command's own idempotency key is still generated.

### Tests for User Story 3

- [X] T009 [US3] Add a test to `execute_test.go` running `--dry-run --yes` together against a command reached through a subcommand group's own persistent hook, asserting no `Guard`-wrapped side effect occurs and the success envelope's `meta.dry_run`/`meta.idempotency_key` are correct; verify it fails against current `main`
- [X] T010 [US3] Run the User Story 3 focused test, confirming the combined-flag invocation is safe

**Checkpoint**: The combined `--dry-run --yes` invocation is verified safe
under a subcommand hook, closing the specific scenario the issue flagged as
uniquely dangerous.

---

## Phase 6: Cross-Cutting Verification

**Purpose**: Confirm no public surface, schema, or golden-file drift, and
run the repository's required gates without lowering a policy budget. This
feature makes no exported-surface change, so there is no baseline/audit
update step.

- [X] T011 Run `make surface-check` and confirm it reports no drift (no `func:*`/`type:*` addition — both touched functions stay unexported), and manually confirm `__schema`/`__schema --as=mcp` output for a tree containing a wrapped subcommand hook contains no new annotation key by inspecting the JSON output directly
- [X] T012 Run `gofmt -s` on changed Go files, execute the feature scenarios in `specs/026-persistent-hook-context/quickstart.md`, and mark every completed task `[X]` in `specs/026-persistent-hook-context/tasks.md`
- [X] T013 Run `make test` and `make validate`, fixing every race, build-tag, format, tidy, and vet failure without weakening tests or changing the feature contract
- [X] T014 Run `make lint` (or `golangci-lint run` directly per this repo's actionlint caveat) and `make doc-coverage`, fixing every Go/Markdown lint or verified-example failure without adding a doccover exemption
- [X] T015 Run `make cover-check`, `make surface-check`, and `make size-check`, fixing failures without lowering coverage floors, changing size budgets, or accepting unintended public-surface drift
- [X] T016 Run `make bench-check` and confirm no tracked benchmark disappeared or exceeded the 5%/+1 budgets — this feature adds a one-time tree walk inside `prepareCommand`, not a tracked hot path, so no benchmark-policy change is expected
- [X] T017 Review `git diff --check`, `git status --short`, and the full diff for scope: no `CHANGELOG.md` edit, no ADR edit/deletion, no payload/schema golden drift, no new dependency, and no unrelated user change; run `grep -rn EnableTraverseRunHooks execute.go` and confirm zero hits (FR-005 — this library must never set that flag on the adopter's behalf); confirm all 17 tasks are complete in `specs/026-persistent-hook-context/tasks.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on T001 and blocks behavior work.
- **User Story 1 (Phase 3)**: Depends on T002; it is the MVP and delivers
  the tree-walk fix every later story's tests verify against.
- **User Story 2 (Phase 4)**: Depends on T005–T006 (the fixed
  implementation must exist before its exactly-once/error-propagation
  guarantees can be tested against it).
- **User Story 3 (Phase 5)**: Depends on T005–T006 for the same reason.
- **Cross-Cutting Verification (Phase 6)**: Depends on all prior phases;
  T011–T017 are ordered validation checkpoints.

### User Story Dependencies

- **US1 (P1)**: Independent MVP after design lock; delivers the tree-walk
  fix itself.
- **US2 (P2)**: Tests only, against the US1 implementation; no independent
  code change.
- **US3 (P3)**: Tests only, against the US1 implementation; no independent
  code change.

### Within Each User Story

- T003 and T004 must fail for the right reason (against current `main`)
  before T005 changes `execute.go`.
- Same-file work remains serial: `execute_test.go` (T003, T004, T007, T009)
  before/after `execute.go` (T005) as ordered above.
- T007 and T009 can be written in parallel with each other (same file,
  different test functions) once T005 has landed, since neither depends on
  the other's assertions — but both still touch `execute_test.go`, so treat
  same-file edits as serial within a single editing session regardless of
  the `[P]` convention.

## Parallel Opportunities

- None of the test-authoring tasks are marked `[P]`: all touch
  `execute_test.go`, and the test-first dependency on `execute.go` (T005)
  is intentionally serial.
- Within cross-cutting verification, read-only gate commands (`make
  cover-check`, `make surface-check`, `make size-check`) can run
  concurrently when they do not contend on build outputs; treat them as
  serial here for attributable diagnostics.

## Implementation Strategy

### MVP First (User Story 1)

1. Complete T001–T002.
2. Write T003 and T004, observe them fail against current `main` for the
   documented reason (shadowed context; tolerated-but-unverified repeat
   wrapping).
3. Implement T005 and validate with T006.
4. Stop: any command in the tree — including ones reached through a
   subcommand's own persistent hook — now gets correct agent-safety
   context.

### Incremental Delivery

1. US1 delivers the tree-walk fix and its own direct verification.
2. US2 verifies the fix is a drop-in change from the adopter's perspective.
3. US3 verifies the specific combined-flag scenario the issue flagged as
   uniquely dangerous.
4. Phase 6 confirms no surface/schema drift and runs the complete policy
   suite.

## Notes

- No governing ADR means no final ADR-retirement task is generated.
- Do not edit or create `CHANGELOG.md`; release notes come from the
  eventual Conventional Commit (`fix:`).
- Do not set `cobra.EnableTraverseRunHooks` anywhere in the implementation.
- Do not change `ax.Guard`, `ax.Confirm`, or `DryRunFromContext` — all three
  are correct today; only `execute.go`'s persistent-hook wiring changes.
- Do not fold in the separate, out-of-scope repeat-`Execute` idempotence
  defect referenced by the issue (tracked elsewhere) beyond what T004/T006
  already verify as an emergent property of the annotation mechanism.
