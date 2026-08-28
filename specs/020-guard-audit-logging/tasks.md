# Tasks: Default-On Audited Guard/Perform

**Input**: Design documents from `/specs/020-guard-audit-logging/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/public-api.md, quickstart.md

**Tests**: INCLUDED and REQUIRED, not optional. This project's Testing-First
Discipline (`AGENTS.md`) mandates the test asserting a contract lands before the
implementation that satisfies it, and `plan.md`'s Testing section enumerates the
exact tests this feature must add/correct (truth-table tests, stderr-capture
tests, a log-forging adversarial test, an extended determinism test, two gated
`ExampleXxx`). Within each user story below, write/update its tests first and
confirm they fail for the right reason before implementing.

**Organization**: This is a small, tightly-coupled single-file feature — all
production code lands in `guard.go`, all new/updated tests in `guard_test.go`,
new examples in `example_test.go`. Tasks are still grouped by user story (per
`spec.md`'s priorities) so each story's acceptance scenarios stay independently
verifiable, but because most tasks share the same three files, `[P]` parallel
markers are used sparingly — only where a task genuinely touches a file no
other in-flight task is editing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)
- Every task names an exact file path

## Path Conventions

Single Go library at the module root (per `plan.md`'s Project Structure):
`guard.go`, `guard_test.go`, `example_test.go`, `README.md` — no new package,
no new directories.

---

## Phase 1: Setup

**Purpose**: Record the pre-feature green baseline before any edits.

- [X] T001 Run `go test -race ./...`, `go vet ./...`, and `golangci-lint run`
      on branch `020-guard-audit-logging` with no files modified yet, to
      confirm the starting point is green (no code changes in this task)

**Checkpoint**: Baseline confirmed green — safe to start Foundational work.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The internal audit engine shared by all four entry points
(`research.md` D1–D4, D6). No user story's tests can exercise the new
`WithAudit` opt-out or the extended truth tables until this phase lands.

**⚠️ CRITICAL**: Must complete before Phase 3 (User Story 1) begins.

- [X] T002 In `guard.go`, add an unexported `auditContextKey` type, exported
      `WithAudit(ctx context.Context, enabled bool) context.Context` (mirrors
      `WithDryRun`'s exact shape — same package, same "boolean toggle carried
      in context" idiom), and unexported `auditEnabledFromContext(ctx
      context.Context) bool` that returns `true` when no prior `WithAudit`
      call is present on the context chain (default-on; the one deliberate
      departure from `DryRunFromContext`'s zero-value-default pattern — see
      `research.md` D2) and the stored value otherwise
- [X] T003 In `guard.go`, add unexported audit-logging helper(s) implementing
      the three constant-message, structured-field log lines from
      `data-model.md`'s "Audit log lines" table: `"ax: about to run effect"`
      (Info), `"ax: effect succeeded"` (Info), `"ax: effect failed"` (Error,
      with `.Err(err)`) — each via `NewLogger(ctx)`, with `ax_helper`
      (`"Guard"`/`"Perform"`) and `description` (always present, possibly
      `""`) as structured fields; no value is ever formatted into the message
      string (FR-007, no log forging)
- [X] T004 In `guard.go`, add unexported `guard(ctx context.Context,
      description string, effect func(context.Context) error) (bool, error)`
      implementing the extended truth table in `data-model.md`'s
      "Guard/GuardWithAudit truth table": dry-run rows are byte-identical to
      feature 012 (`logDryRunSkip`, never consults audit-enabled state — see
      D5); on the real, non-nil-effect row, when `auditEnabledFromContext(ctx)`
      is `true`, call the T003 helpers around `effect(ctx)` (about-to-run
      before, succeeded/failed after)
- [X] T005 In `guard.go`, add unexported `perform(ctx context.Context,
      description string, rehearse, commit func(context.Context) error)
      error` implementing the extended truth table in `data-model.md`'s
      "Perform/PerformWithAudit truth table", analogous to T004

**Checkpoint**: The internal engine exists but has no caller yet — an expected
transient state (T004/T005 and parts of T002/T003 are unused until Phase 3
wires them in; this is resolved within Phase 3, not before it, since
`golangci-lint`/`unused` verification happens at each phase's own checkpoint
task, not mid-phase).

---

## Phase 3: User Story 1 - Every existing Guard/Perform call gets baseline audit visibility with no code change (Priority: P1) 🎯 MVP

**Goal**: `Guard`/`Perform` emit default "about to run" / "succeeded"/"failed"
audit lines on every real run, with zero call-site code changes, plus a
per-call opt-out — closing the actual production gap that motivated this
feature (issue #179).

**Independent Test**: Take an existing plain-`Guard` call site, run it for
real, and confirm stderr shows the structured about-to-run line followed by
the outcome line, with no call-site changes; confirm the opt-out silences both.

### Tests for User Story 1 (write first, confirm they fail against current Guard/Perform)

- [X] T006 [US1] In `guard_test.go`, update `TestGuardSuppressionLogged`'s
      real-run case: it currently asserts real-run stderr is **empty** — per
      `research.md` D8 that assertion is now wrong by design. Replace it with
      an assertion that stderr contains exactly the about-to-run line followed
      by the succeeded line (structured JSON, `ax_helper="Guard"`,
      `description=""`); leave the dry-run and nil-effect cases in the same
      test unchanged (still valid)
- [X] T007 [US1] In `guard_test.go`, update `TestPerformSuppressionLogged`
      analogously for `Perform`'s real-run case; leave the dry-run cases
      unchanged
- [X] T008 [US1] In `guard_test.go`, add table-driven test(s) covering the
      full extended `Guard` truth table from `data-model.md` (dry-run ×
      nil/non-nil effect × `auditEnabledFromContext`), including: the
      `WithAudit(ctx, false)` opt-out row suppresses both audit lines on a
      real run (Acceptance Scenario 3); a real-run failing effect produces
      about-to-run + a **Error**-level failed line carrying the error via the
      `error` field (Acceptance Scenario 2); `errors.Is`/`errors.As` still
      work through the returned error
- [X] T009 [US1] In `guard_test.go`, add table-driven test(s) covering the
      full extended `Perform` truth table from `data-model.md` (dry-run ×
      rehearse/commit combinations × `auditEnabledFromContext`), including the
      opt-out row and the real-run failing-commit case

### Implementation for User Story 1

- [X] T010 [US1] In `guard.go`, change `Guard` to delegate to `guard(ctx, "",
      effect)` (T004); rewrite its doc comment per `contracts/public-api.md`'s
      Doc-comment contract: describe the new default-on audit behavior, the
      `WithAudit(ctx, false)` opt-out, unchanged dry-run suppression, unchanged
      nil-effect handling, wrap-chain preservation, and that no exit code is
      mapped
- [X] T011 [US1] In `guard.go`, change `Perform` to delegate to `perform(ctx,
      "", rehearse, commit)` (T005); rewrite its doc comment analogously
- [X] T012 [US1] Run T006–T009, confirm all pass; run `go test -race ./...`,
      `go vet ./...`, `golangci-lint run` and fix any findings (this is the
      first checkpoint where the Phase 2 engine has real callers, resolving
      the transient unused-code state noted at the end of Phase 2)

**Checkpoint**: User Story 1 is fully functional and independently
testable/demoable — every existing `Guard`/`Perform` call site gets baseline
audit visibility on its next real run, with a working opt-out.

---

## Phase 4: User Story 2 - Developer upgrades to a rich, described audit trail (Priority: P2)

**Goal**: `GuardWithAudit`/`PerformWithAudit` let a call site carry a
human-meaningful description into every audit line instead of the generic
default.

**Independent Test**: Wrap an effect with `GuardWithAudit`, supply a
description, run for real, and confirm the audit lines carry that description
as a structured field instead of `""`.

### Tests for User Story 2

- [X] T013 [US2] In `guard_test.go`, add test(s) asserting `GuardWithAudit`'s
      about-to-run/outcome lines carry the caller-supplied `description` field
      verbatim on a real run, and that its dry-run behavior is byte-identical
      to `Guard`'s (same suppression line, `description` plays no role in the
      dry-run branch)
- [X] T014 [US2] In `guard_test.go`, add test(s) asserting `PerformWithAudit`'s
      lines carry the description analogously, including the real-run failing
      `commit` case

### Implementation for User Story 2

- [X] T015 [US2] In `guard.go`, add exported `GuardWithAudit(ctx
      context.Context, description string, effect func(context.Context)
      error) (bool, error)` delegating to `guard(ctx, description, effect)`
      (T004), with a doc comment per `contracts/public-api.md`
- [X] T016 [US2] In `guard.go`, add exported `PerformWithAudit(ctx
      context.Context, description string, rehearse, commit
      func(context.Context) error) error` delegating to `perform(ctx,
      description, rehearse, commit)` (T005), with a doc comment per
      `contracts/public-api.md`
- [X] T017 [US2] In `example_test.go`, add `ExampleGuardWithAudit`,
      demonstrating `WithAudit(ctx, false)` inline (per Principle VII: `WithX`
      options are shown inside a parent example, not gated individually)
- [X] T018 [US2] In `example_test.go`, add `ExamplePerformWithAudit`
- [X] T019 [US2] Run T013–T014, `make doc-coverage` (verifies
      `ExampleGuardWithAudit`/`ExamplePerformWithAudit` are present and
      pass), `go test -race ./...`, `go vet ./...`, `golangci-lint run`; fix
      any findings

**Checkpoint**: User Stories 1 AND 2 both work independently — the named
audited variants ship with description support and gated examples.

---

## Phase 5: User Story 3 - Dry-run behavior is unchanged (Priority: P3)

**Goal**: Prove dry-run stderr output stays byte-for-byte identical to
feature 012, regardless of entry point (plain or named-audited) or opt-out
state, and that the machine envelope's determinism is unaffected.

**Independent Test**: Run each of the four entry points under `--dry-run` and
confirm stderr contains exactly the existing single suppression line, nothing
else.

- [X] T020 [US3] In `guard_test.go`, add test(s) running each of the four
      entry points (`Guard`, `Perform`, `GuardWithAudit`, `PerformWithAudit`)
      under dry-run with the opt-out both set and unset, asserting stderr
      contains exactly the existing single suppression line and nothing else
      (SC-005, FR-006, spec User Story 3 Acceptance Scenario 1) — also cover
      Acceptance Scenario 2: a failing `rehearse` under dry-run produces no
      suppression line and no about-to-run/failed line
- [X] T021 [US3] In `guard_test.go`, extend `TestEnvelopeDeterministicUnderDryRun`
      to also route a no-op through `GuardWithAudit`/`PerformWithAudit`
      (in addition to the existing `Guard`/`Perform` calls) before building
      the envelope, proving the extended default/audited path never perturbs
      the `stdout` envelope's byte-for-byte determinism (SC-002)
- [X] T022 [US3] Run T020–T021 and the full `go test -race ./...`; fix any
      regressions

**Checkpoint**: Dry-run parity and envelope determinism are proven across all
four entry points and both opt-out states.

---

## Phase 6: User Story 4 - Every description is safe to log (Priority: P4)

**Goal**: An adversarial caller-supplied description can never forge or
corrupt a log line.

**Independent Test**: Call `GuardWithAudit`/`PerformWithAudit` with a
description containing newlines/control characters and confirm the log
output stays well-formed, one JSON object per line, with the description
confined to its own field.

- [X] T023 [US4] In `guard_test.go`, add an adversarial test calling
      `GuardWithAudit`/`PerformWithAudit` with a description containing
      newlines and control characters; capture stderr, split on newlines, and
      assert: exactly the expected number of lines, each parses as valid JSON,
      and the raw description text appears only as the `description` field's
      value — never concatenated into `message` or splitting output into
      extra forged lines (FR-007, SC-004)
- [X] T024 [US4] Run T023 and the full `go test -race ./...`; fix any
      regressions

**Checkpoint**: Log-forging safety is proven for the new audit path.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, gates, and the governed breaking-change release
process (this feature ships as `feat!:`/`BREAKING CHANGE:`, not a plain
`feat:` — see `research.md` D8).

- [X] T025 [P] Update `README.md`: document the default audit behavior for
      `Guard`/`Perform`, the named `GuardWithAudit`/`PerformWithAudit`
      variants, the `WithAudit(ctx, false)` opt-out, and the migration note
      verbatim from `research.md` D8 / `contracts/public-api.md`
- [X] T026 Run `make cover-check`; confirm root `ax` package (floor 85%) stays
      at or above its floor with the new/updated tests; add coverage for any
      newly-uncovered branch
- [X] T027 Run `make surface-check`; it is expected to report drift (three new
      exported symbols: `GuardWithAudit`, `PerformWithAudit`, `WithAudit`) —
      run `make surface-update` and review every line of the
      `internal/cmd/surfacecheck/baseline.json` diff before accepting
- [X] T028 Run the full verification block from `quickstart.md`: `go test
      -race ./...`, `go vet ./...`, `golangci-lint run`, `make doc-coverage`,
      `make cover-check`; confirm all pass
- [ ] T029 [FINAL] Prepare the governed breaking-change commit/PR: `feat!:`
      subject with a `BREAKING CHANGE:` trailer using the exact migration note
      from `research.md` D8 / `contracts/public-api.md` ("Guard and Perform
      now emit two structured stderr log lines... To restore the previous
      silent behavior for a specific call site, wrap its context with
      `ax.WithAudit(ctx, false)`."), and apply the `breaking-change-approved`
      label to the PR — required per Constitution Principle XI even though
      `go-apidiff` will report this change as purely additive and will NOT
      request the label itself (D8)

No governing ADR names this feature (`research.md`: "Decision Records
Absorbed = N/A"), so no ADR-retirement task is included.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user
  stories (T004/T005's `guard`/`perform` engine and T002's `WithAudit` are
  used by every story below).
- **User Story 1 (Phase 3)**: Depends on Foundational. Delivers the MVP.
- **User Story 2 (Phase 4)**: Depends on Foundational (uses T004/T005
  directly) and, practically, on User Story 1 having landed first, since
  `GuardWithAudit`/`PerformWithAudit` are new siblings of the just-updated
  `Guard`/`Perform` in the same file.
- **User Story 3 (Phase 5)**: Its full acceptance scope ("plain or the named
  audited variant") needs both User Story 1 and User Story 2's entry points to
  exist.
- **User Story 4 (Phase 6)**: Needs `GuardWithAudit`/`PerformWithAudit` (User
  Story 2), since only the named variants accept a caller-supplied
  description to attack.
- **Polish (Phase 7)**: Depends on all four user stories being complete.

### Within Each User Story

- Tests are written/updated first and confirmed to fail against the
  pre-implementation code for the right reason, then implementation makes
  them pass (Testing-First Discipline).
- Each story's own checkpoint task (T012/T019/T022/T024) re-runs
  `go test -race ./...` (and, for Phases 3–4, `go vet`/`golangci-lint`) before
  moving to the next phase.

### Parallel Opportunities

This feature's small, single-file footprint (`guard.go` for production code,
`guard_test.go` for nearly all tests) limits genuine file-level parallelism —
most tasks are intentionally sequenced within their file. The one clearly
independent task is:

- T025 (`README.md`) can be drafted in parallel with Phase 6/7's remaining
  verification tasks, since it doesn't share a file with any of them.

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (the shared audit engine).
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: every existing `Guard`/`Perform` call site now gets
   default audit visibility with no code change, and the opt-out works — this
   alone closes the production gap that motivated issue #179.

### Incremental Delivery

1. Setup + Foundational → engine ready.
2. User Story 1 → validate independently → MVP.
3. User Story 2 → validate independently → richer described audit trail.
4. User Story 3 → validate independently → dry-run parity proven across all
   four entry points.
5. User Story 4 → validate independently → log-forging safety proven.
6. Polish → docs, gates, and the governed breaking-change release process.

---

## Notes

- `[P]` is used sparingly here — this is a small, tightly-coupled single-file
  feature, and most tasks intentionally touch `guard.go` or `guard_test.go`
  sequentially to avoid edit conflicts.
- This feature is a deliberate breaking **behavior** change (Constitution
  Principle XI), even though every new/changed Go signature is additive —
  see `research.md` D8 and Phase 7's T029. Do not let a green `go-apidiff` run
  substitute for the manual `breaking-change-approved` label.
- Verify each story's tests fail before implementing it; commit after each
  task or logical group; stop at any checkpoint to validate a story
  independently.
