---
description: "Task list for Coverage Policy and CI Enforcement"
---

# Tasks: Coverage Policy and CI Enforcement

**Input**: Design documents from `/specs/009-coverage-policy-ci/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/covercheck-output.md, quickstart.md

**Tests**: INCLUDED. The feature is governed by Constitution Principle VII
(Test-First), and `plan.md` §A1 plus `research.md` enumerate the exact test
cases. Tests are written first and must FAIL before `main.go` exists.

**Organization**: Tasks are grouped by user story to enable independent
implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)
- Exact file paths are included in every task

## Path Conventions

This is a Go library at module root `github.com/rshade/ax-go`. New tooling lives
under `internal/cmd/covercheck/`. CI, Makefile, and docs are at the repo root.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the home for the new tool source and tests

- [X] T001 Create the `internal/cmd/covercheck/` directory (and a `testdata/` subdirectory for golden-file fixtures) at the repo root so the tool source (`main.go`), tests (`main_test.go`), and golden files have a home

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before user stories

**No foundational tasks required.** This feature has no shared runtime
infrastructure: User Story 1 delivers a self-contained Go tool, and User
Stories 2–4 are independent documentation/CI configuration. Each story can begin
immediately after Setup (Phase 1).

**Checkpoint**: Directory exists — user story implementation can begin.

---

## Phase 3: User Story 1 - Coverage Regression Blocked in CI (Priority: P1) 🎯 MVP

**Goal**: CI fails when any non-excluded package, or the repo-wide aggregate,
falls below its calibrated floor, with a message naming the offending package(s)
and the shortfall.

**Independent Test**: Run `go test -race -coverprofile=coverage.out -covermode=atomic ./...`
then `go run ./internal/cmd/covercheck -coverage coverage.out`; confirm it exits
`0` on the current baseline, and exits `1` with a named-package message when a
floor is artificially raised above baseline. Removing a test file and re-running
produces a failing check (Acceptance Scenario 1).

### Tests for User Story 1 (write FIRST, ensure they FAIL) ⚠️

- [X] T002 [US1] Write table-driven + fuzz tests in `internal/cmd/covercheck/main_test.go` covering: `TestParseCoverage` (synthetic `coverage.out` → per-package maps), `TestCheckFloors_Pass` (exit 0), `TestCheckFloors_PackageViolation` (one package below floor → exit 1, message names package), `TestCheckFloors_RepoWideViolation` (aggregate below floor → exit 1), `TestCheckFloors_ExcludedPackage` (excluded package at 0% does not fail per-package gate), `TestCheckFloors_MissingFile` (missing file → exit 2), `TestFormatOutput_Pass` and `TestFormatOutput_Fail` — assert the stdout/stderr format against **golden files** under `internal/cmd/covercheck/testdata/` (output is stable-by-contract per contracts/; Constitution Principle VII; refresh only via an explicit `-update` flag), and `FuzzParseCoverageProfile` (random bytes must not panic). Verify all FAIL/do-not-compile before implementation.

### Implementation for User Story 1

- [X] T003 [US1] Define entity types in `internal/cmd/covercheck/main.go` per data-model.md: `PackageCoverage`, `Violation`, `CheckResult`, and the `FloorConfig` constants/vars (`repoWideFloor = 70.0`, `packageFloors` map, `defaultPackageFloor = 25.0`, `excludedPackages` slice)
- [X] T004 [US1] Implement the coverage-profile parser in `internal/cmd/covercheck/main.go`: read the `-coverage` file, accumulate `numStmt`/covered statements per import path; return exit-code-2 errors on missing/unreadable/malformed input (per contracts/ exit-code table)
- [X] T005 [US1] Implement floor-checking logic in `internal/cmd/covercheck/main.go`: compute per-package `Pct`, skip `excludedPackages` for the per-package gate, apply `packageFloors`/`defaultPackageFloor`, compute the repo-wide aggregate `sum(covered)/sum(stmts)`, and assemble `CheckResult`/`Violation` list
- [X] T006 [US1] Implement output formatting in `internal/cmd/covercheck/main.go` per contracts/covercheck-output.md: pass summary (one line per checked package + repo-wide + excluded list) to **stdout**; violations (count + per-violation actual%/floor%/delta) to **stderr**; deterministic, no timestamps (Constitution Principles I & II)
- [X] T007 [US1] Implement `main()` flag parsing (`-coverage`) and exit-code wiring (0 pass / 1 violation / 2 bad input) in `internal/cmd/covercheck/main.go`; run `go test -race ./internal/cmd/covercheck/...` and confirm every T002 test now PASSES
- [X] T008 [US1] Add a `.PHONY` `cover-check` target to `Makefile` that depends on `test-cover` and runs `go run ./internal/cmd/covercheck -coverage coverage.out` (per plan §A3)
- [X] T009 [US1] Add an "Enforce coverage floors" step to `.github/workflows/ci.yml` immediately after the existing race-detector+coverage step (same `test` job, `run: go run ./internal/cmd/covercheck -coverage coverage.out`)

**Checkpoint**: CI now blocks coverage regressions; the gate runs identically locally via `make cover-check`. MVP complete.

---

## Phase 4: User Story 2 - Coverage Policy Is Documented (Priority: P2)

**Goal**: `AGENTS.md` carries an authoritative Coverage Policy section a
contributor or agent can find and reproduce locally.

**Independent Test**: Search `AGENTS.md` for "coverage"; confirm the section
states the per-package floors, the repo-wide floor, the excluded packages, and a
local command that reproduces the CI check (Acceptance Scenarios 1–2).

### Implementation for User Story 2

- [X] T010 [US2] Add a "Coverage Policy" section to `AGENTS.md` (insert after "Testing-First Discipline") with: the floors table (repo-wide 70% / per-package default 25%), the per-package override table, the "Excluded from Per-Package Floor Enforcement" table, the "Local Verification" command (`make cover-check`), the "Raising a Floor" steps, and the "Escalation Path" — per plan §A5 (satisfies FR-005, FR-009)

**Checkpoint**: Policy is discoverable and the documented local command matches what CI runs.

---

## Phase 5: User Story 3 - Coverage Delta Visible on PRs (Priority: P3)

**Goal**: Every PR surfaces the overall coverage percentage and delta from the
base branch via Codecov status checks/comment.

**Independent Test**: Open a PR that changes coverage; confirm a Codecov status
check or comment shows the overall percentage and signed delta, and that a
below-floor-but-above-Codecov-threshold change still lets the (advisory) Codecov
check pass while `covercheck` remains the authoritative gate.

### Implementation for User Story 3

- [X] T011 [P] [US3] Create `.codecov.yml` at the repo root with `coverage.status.project.default` (target 70%, threshold 1%), `coverage.status.patch.default` (target 70%, threshold 5%), and a `comment` block (`layout: "reach,diff,flags,files"`, `require_changes: true`) per plan §B1 (satisfies FR-006)
- [X] T012 [US3] Verify the existing `codecov/codecov-action@v7` step in `.github/workflows/ci.yml` keeps `fail_ci_if_error: false` (Codecov upload must not block PRs — `covercheck` is the gate) and add a comment documenting the `CODECOV_TOKEN` fallback for private-repo use per plan §B2

**Checkpoint**: PR authors and reviewers see coverage deltas without running coverage locally.

---

## Phase 6: User Story 4 - Coverage Badge in README (Priority: P4)

**Goal**: The README shows a live repo-wide coverage badge.

**Independent Test**: Open `README.md`; confirm a Codecov coverage badge renders
and links to the Codecov dashboard (Acceptance Scenarios 1–2).

### Implementation for User Story 4

- [X] T013 [US4] Add the Codecov coverage badge to `README.md` (after existing CI badges, or as a new badge row): `[![Coverage](https://codecov.io/gh/rshade/ax-go/branch/main/graph/badge.svg)](https://codecov.io/gh/rshade/ax-go)` per plan §C1 (satisfies FR-007)

**Checkpoint**: All four user stories independently functional.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Follow-ups and final validation

- [X] T014 [P] File GitHub follow-up issues for the three excluded packages lacking tests (`internal/cli`, `internal/mcp`, `internal/schema`) so they can be moved into per-package floor enforcement and the repo-wide floor raised (per research.md exclusion-list rationale)
- [X] T015 Run `quickstart.md` validation end-to-end: `make cover-check` must exit 0 on the current baseline, confirming the first CI run does not fail (SC-007 / FR-010)
- [X] T016 Run `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./...`, and `make doc-coverage`; all must be clean before handing the feature back
- [ ] T017 [P] Post-merge verification of US3/US4 (Codecov-dependent, not locally testable): after the first PR merges to `main`, confirm the Codecov `project`/`patch` status checks and PR comment appear (FR-006) and the README badge renders the current percentage (FR-007); if upload fails on a private-repo move, add `CODECOV_TOKEN` to GitHub Secrets per plan §B2

> No governing ADR for this feature (plan.md: "No governing ADR") — no ADR-retirement task.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: None — no blocking tasks
- **User Story 1 (Phase 3)**: Depends only on Setup (T001)
- **User Story 2 (Phase 4)**: Independent — documents US1's behavior but can be written in parallel with US1
- **User Story 3 (Phase 5)**: Independent of US1/US2
- **User Story 4 (Phase 6)**: Logically follows US3 (badge depends on the Codecov integration existing), but the README edit itself has no code dependency
- **Polish (Phase 7)**: After the targeted user stories are complete

### Within User Story 1 (single-file ordering)

- T002 (tests) FIRST and must FAIL
- T003 → T004 → T005 → T006 → T007 are sequential — all edit `internal/cmd/covercheck/main.go`
- T008 (Makefile) and T009 (ci.yml) follow once the tool compiles and passes (T007)

### Parallel Opportunities

- After T001, the four user stories can be staffed in parallel by different developers (US1 is the critical path; US2/US3/US4 are docs/config)
- T011 `[P]` (`.codecov.yml`) and T013 (`README.md`) touch different files from the US1 work and can proceed alongside it
- T014 `[P]` (filing issues) is independent of all code tasks
- Within US1, the `main.go` tasks (T003–T007) are **NOT** parallel — same file

---

## Parallel Example: cross-story staffing after Setup

```bash
# Once T001 is done, these independent tracks can run concurrently:
Track A (critical path): T002 → T003 → T004 → T005 → T006 → T007 → T008 → T009   # US1 tool + CI gate
Track B: T010                                                                     # US2 AGENTS.md policy
Track C: T011 → T012 → T013                                                       # US3 Codecov + US4 badge
Track D: T014                                                                     # Polish: file follow-up issues
# T017 (post-merge Codecov/badge verification) runs after the first PR merges to main
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Skip Phase 2 (no foundational tasks)
3. Complete Phase 3: User Story 1 (T002–T009) — TDD: tests fail, then pass
4. **STOP and VALIDATE**: `make cover-check` blocks regressions locally and in CI
5. This alone satisfies the core safety guarantee (FR-001–FR-004, FR-008, FR-010)

### Incremental Delivery

1. Setup → US1 (the floor gate, MVP) → ship
2. Add US2 (AGENTS.md policy) → ship
3. Add US3 (Codecov PR deltas) → ship
4. Add US4 (README badge) → ship
5. Polish (follow-up issues + full lint/test sweep)

### Parallel Team Strategy

After T001: Developer A owns US1 (critical path); Developer B writes US2 docs;
Developer C wires US3+US4 Codecov/badge; all integrate independently.

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps each task to a user story for traceability
- US1 is the only story with internal sequential ordering (single `main.go`)
- Verify T002 tests FAIL before writing T003–T007 (Constitution Principle VII)
- Floors are hardcoded constants in `main.go` so every change is auditable via `git blame` (research.md Decision 2)
- `covercheck` output format (T002 `TestFormatOutput_*`) is guarded by golden files under `internal/cmd/covercheck/testdata/`; regenerate only via an explicit `-update` flag (Constitution Principle VII)
- `covercheck` intentionally omits `context.Context` (sub-second `cmd/` main, one bounded file read) — recorded deviation from Principle X, see plan.md §A2
- T017 (US3/US4 verification) is Codecov-dependent and runs **after** the first merge to `main`; it is the only task that cannot be validated locally
- `CHANGELOG.md` is managed by release-please — do NOT hand-edit it; capture user-facing changes in Conventional Commit messages
