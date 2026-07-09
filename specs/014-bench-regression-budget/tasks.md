---

description: "Task list for Performance Regression Budget in CI"

---

# Tasks: Performance Regression Budget in CI

**Input**: Design documents from `/specs/014-bench-regression-budget/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/benchcheck-output.md, quickstart.md

**Tests**: Included — Constitution Principle VII (Test-First Discipline) is NON-NEGOTIABLE in this repo; `internal/cmd/benchcheck`'s core logic gets table-driven tests written before its implementation.

**Organization**: Tasks are grouped by user story (spec.md priorities P1/P2/P3) to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Path Conventions

Single Go project; root module `github.com/rshade/ax-go`. Paths below are
exact, taken from `plan.md`'s Project Structure section.

---

## Phase 1: Setup

**Purpose**: Project initialization

- [X] T001 Add `golang.org/x/perf` as a new direct dependency: run
      `go get golang.org/x/perf && go mod tidy` from the repo root; verify
      `go.mod`/`go.sum` gain the entry and no unrelated dependency changes.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST exist before User Story 1's
implementation can compile

**⚠️ CRITICAL**: No User Story 1 implementation task can begin until T001 is
complete.

- [X] T002 Create the `internal/cmd/benchcheck/` directory with a minimal
      `main.go` skeleton (package doc comment stating the stream-separation
      and exit-code contract per `contracts/benchcheck-output.md`; `package
      main`; empty `func main()`), mirroring `internal/cmd/covercheck/main.go`'s
      header shape.

**Checkpoint**: Foundation ready — User Story 1 implementation can begin.

---

## Phase 3: User Story 1 - Performance Regression Blocked in CI (Priority: P1) 🎯 MVP

**Goal**: A PR that regresses a tracked hot path's `ns/op` beyond 5%
(statistically significant) or `allocs/op` by more than 1 fails
`make bench-check` / the CI `bench` job, naming the offending benchmark.

**Independent Test**: Temporarily degrade a benchmarked hot path (e.g. add an
allocation to `WriteError`), run `make bench-check`, confirm exit 1 with a
correctly named violation; revert and confirm it passes again.

### Tests for User Story 1 ⚠️

> Write these first; they MUST fail (main.go is still the T002 stub).

- [X] T003 [P] [US1] Write table-driven tests in
      `internal/cmd/benchcheck/main_test.go` covering: no-regression pass
      (exit 0), `ns/op`-only regression beyond 5% (exit 1, message names the
      benchmark and delta), `allocs/op`-only regression beyond +1 (exit 1),
      and a malformed/missing baseline or current file (exit 2). Use
      synthetic multi-sample (`-count=N`-style, ≥5 samples per config)
      fixture strings — a single-sample fixture never registers as
      statistically significant (confirmed in `research.md` Decision 1).

### Implementation for User Story 1

- [X] T004 [US1] Implement `internal/cmd/benchcheck/main.go`: const block
      (`maxNsOpRegressionPercent = 5.0`, `maxAllocsOpIncrease = 1`, exit codes
      `exitOK=0`/`exitViolation=1`/`exitBadInput=2`), a bounded-read helper for
      the two input files (mirrors `covercheck`'s `maxProfileBytes` pattern),
      a `benchstat.Collection{Alpha: 0.05, DeltaTest: benchstat.UTest}`-based
      comparison, `Violation`/`CheckResult` types (per `data-model.md`), a
      pure `run(baselinePath, currentPath string, stdout, stderr io.Writer)
      int`, and `-baseline`/`-current` flag parsing in `main()`. Makes T003
      pass.
- [X] T005 [P] [US1] Add `internal/schema/schema_bench_test.go`:
      `BenchmarkBuildCommand`, constructing a representative multi-command,
      multi-flag `*cobra.Command` tree once outside `b.Loop()`, then calling
      `schema.BuildCommand` (`internal/schema/schema.go:31`) inside the loop.
      Doc comment states what allocation claim it substantiates, per
      `research.md` Decision 4.
- [X] T006 [P] [US1] Add `contract/error_bench_test.go`: `BenchmarkWriteError`,
      constructing one `*Error` via `NewError` with `WithErrorContext` and
      `WithSuggestions` populated once outside `b.Loop()`, then calling
      `WriteError(io.Discard, err)` (`contract/error.go:166`) inside the loop.
- [X] T007 [US1] Add fixed benchmark-run variables to `Makefile`:
      `BENCH_CPU?=1`, `BENCH_COUNT?=10`, and `BENCH_BASE_REF?...`, with the
      benchmark command using `-cpu=$(BENCH_CPU)` so benchmark names are
      portable across host core counts.
- [X] T008 [US1] Ensure no generated benchmark-output file is committed;
      `benchcheck` receives temporary base/current files from `make
      bench-check`, not `bench/baseline.txt`.
- [X] T009 [US1] Add a `bench-check` target to `Makefile` that creates a
      temporary git worktree at `BENCH_BASE_REF`, benchmarks base and current
      with identical `-cpu=1 -count=10 -benchmem` flags, then runs
      `go run ./internal/cmd/benchcheck -baseline <base-temp> -current
      <current-temp>`; add `bench-check` to the `ci` meta-target
      (`Makefile:12`, alongside `test validate lint doc-coverage`) and to
      `help`. Depends on T004, T007.
- [X] T010 [US1] Add a new PR-only `bench` job to
      `.github/workflows/ci.yml`, using `setup-go@v6` + `GO_VERSION:
      '1.26.4'`, `fetch-depth: 0`, `BENCH_BASE_REF` set to the pull request
      base SHA, and `make bench-check`. Depends on T009.

**Checkpoint**: User Story 1 is fully functional and independently testable —
a regressing change can be shown to fail `make bench-check` and the CI
`bench` job; an unchanged codebase passes.

---

## Phase 4: User Story 2 - Base Comparison Is Reproducible (Priority: P2)

**Goal**: A maintainer can reproduce CI's same-host base/current comparison
locally and choose the base ref explicitly.

**Independent Test**: Run `BENCH_BASE_REF=HEAD~1 make bench-check` locally
and verify the target benchmarks both refs on the same host before invoking
`benchcheck`.

### Implementation for User Story 2

- [X] T011 [US2] Exercise the update workflow end-to-end using the
      infrastructure built in US1 (T007-T009): run `BENCH_BASE_REF=HEAD~1
      make bench-check`, confirm the target creates a temporary base
      worktree, captures base/current benchmark files with `-cpu=1`, invokes
      `benchcheck`, and cleans up the worktree. This is a verification pass,
      not new code — US2 reuses US1's Makefile target by design
      (`research.md` Decision 5).

**Checkpoint**: User Stories 1 and 2 both work independently.

---

## Phase 5: User Story 3 - Regression Budget Is Documented (Priority: P3)

**Goal**: A contributor or coding agent can find the tracked benchmarks, the
regression budget, the local verification command, and the `BENCH_BASE_REF`
override in `AGENTS.md` in under 60 seconds.

**Independent Test**: Open `AGENTS.md`, search for the performance section,
and reproduce the CI check locally using only the documented command.

### Implementation for User Story 3

- [X] T012 [US3] Add a "Performance Regression Budget" section to `AGENTS.md`
      immediately after the existing "## Coverage Policy" section
      (`AGENTS.md:212-280`), mirroring its structure: a table of tracked
      benchmarks (the four hot paths from `data-model.md`'s Tracked Benchmark
      entity), the budget values (5% `ns/op`, +1 `allocs/op`), a "Local
      Verification" subsection (`make bench-check` plus `BENCH_BASE_REF`),
      an "Adjusting the Budget" subsection (edit the consts in
      `internal/cmd/benchcheck/main.go`, same as `covercheck`'s "Raising a
      Floor"), and an "Accepting Trade-Offs" subsection documenting that
      there is no committed benchmark baseline to regenerate.

**Checkpoint**: All three user stories are independently functional and
documented.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Repo-wide consistency checks that depend on the completed
implementation

- [X] T013 [P] Run `go test -cover ./internal/cmd/benchcheck/...` to get
      `benchcheck`'s actual measured coverage, then add an explicit
      `github.com/rshade/ax-go/internal/cmd/benchcheck` entry to
      `covercheck`'s `defaultFloorConfig().perPackage` map in
      `internal/cmd/covercheck/main.go`, calibrated to that measurement
      (mirrors the existing `internal/cmd/doccover` entry's precedent, per
      `research.md` Decision 6). Also add the new package's floor to the
      `AGENTS.md` Coverage Policy table.
- [X] T014 [P] Update `ROADMAP.md`: mark `- [ ] #22 ...` (currently ~line 43)
      done per the existing `[x]` convention (see `#11`'s entry at line 147
      for the precedent), and update the "Immediate Focus"/single-WIP
      pointer text (lines 24-29, 55-57) since `#18` becomes the next
      epic-promotion candidate.
- [X] T015 Run the full local gate from `quickstart.md`: `gofmt -s -l .`,
      `go vet ./...`, `golangci-lint run`, `go test -race ./...`,
      `make bench-check`, `make cover-check`, `make doc-coverage`,
      `markdownlint-cli2 "specs/014-bench-regression-budget/**/*.md"`. All
      must be clean.
- [X] T016 Execute every command in `quickstart.md` literally and confirm the
      actual output matches the documented shape (pass example, failure
      example via a temporary regression, base-ref override example) — the
      final acceptance pass against spec SC-001 through SC-006.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on T001 — blocks all of User Story 1.
- **User Story 1 (Phase 3)**: Depends on Phase 2. Fully sequential within
  itself (T004 depends on T003; T005/T006 are parallel with each other and
  with T004; T007 depends on T004-T006; T008 depends on T007; T009 depends on
  T004 and T007; T010 depends on T009).
- **User Story 2 (Phase 4)**: Depends on User Story 1 (T007-T009) being
  complete — it exercises US1's own Makefile targets.
- **User Story 3 (Phase 5)**: Depends on User Story 1 (T004-T010) for
  accurate command names and values to document; independent of US2's
  verification pass.
- **Polish (Phase 6)**: Depends on all three user stories being complete.

### Parallel Opportunities

- T005 and T006 (the two new benchmarks) can run in parallel with each other
  and with T004 (different files, no shared state).
- T013 and T014 (Polish phase) can run in parallel (different files).

---

## Parallel Example: User Story 1

```bash
# After T004 (benchcheck implementation) is underway, launch the two new
# benchmarks together — different packages, no shared state:
Task: "Add internal/schema/schema_bench_test.go: BenchmarkBuildCommand"
Task: "Add contract/error_bench_test.go: BenchmarkWriteError"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup) and Phase 2 (Foundational).
2. Complete Phase 3 (User Story 1) — this alone delivers the core safety
   guarantee: a real CI gate that fails on regression.
3. **STOP and VALIDATE**: demonstrate a regressing change fails
   `make bench-check`.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. User Story 1 → the gate exists and works → this is the MVP.
3. User Story 2 → the update procedure is proven to work end-to-end.
4. User Story 3 → the policy is documented for future contributors.
5. Polish → repo-wide consistency (coverage floor, roadmap, full gate).

---

## Notes

- No governing ADR for this feature (`plan.md`) — no ADR-retirement task.
- [P] tasks touch different files with no shared state.
- Tests (T003) MUST fail before T004 is implemented — verify this before
  writing the implementation.
- Budget constants (`maxNsOpRegressionPercent`, `maxAllocsOpIncrease`) are
  hardcoded in `internal/cmd/benchcheck/main.go`, never externalized to
  config — every change is a reviewable, `git blame`-auditable commit.
