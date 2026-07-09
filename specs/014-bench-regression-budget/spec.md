# Feature Specification: Performance Regression Budget in CI

**Feature Branch**: `014-bench-regression-budget`

**Created**: 2026-07-08

**Status**: Draft

**Input**: User description: "Performance regression budget: benchstat in CI for hot-path benchmarks (GitHub issue #22, epic-parent #11)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Performance Regression Blocked in CI (Priority: P1)

A contributor opens a PR that unintentionally adds an allocation or slows down
a hot path (the logger emit path, the `__schema` reflection path, or the error
envelope marshal path). The CI pipeline benchmarks both the PR's base commit
and the PR worktree on the same runner, then fails the check when the
regression exceeds the budget, preventing the PR from merging silently.

**Why this priority**: This is the core safety guarantee the issue asks for —
performance cannot silently regress the way it currently can. Every other
story depends on a working gate first.

**Independent Test**: Submit a change that measurably slows a benchmarked hot
path or adds an allocation to it, run the CI performance check, and verify it
fails with a message naming the offending benchmark and the measured delta.

**Acceptance Scenarios**:

1. **Given** a PR changes code on a benchmarked hot path such that its
   operation time increases beyond the budget, **When** the CI performance
   check runs, **Then** the check fails and the message names the benchmark
   and the percentage regression.
2. **Given** a PR changes code such that a benchmarked hot path allocates more
   memory per operation than the budget allows, **When** the CI performance
   check runs, **Then** the check fails and the message names the benchmark
   and the allocation increase.
3. **Given** a PR does not regress any tracked benchmark beyond the budget,
   **When** the CI performance check runs, **Then** the check passes and the
   PR can proceed to review.
4. **Given** a benchmark's measurements are noisy but not statistically
   distinguishable from the baseline, **When** the CI performance check runs,
   **Then** the check passes (noise alone does not fail the build).

---

### User Story 2 - Base Comparison Is Reproducible (Priority: P2)

A maintainer or contributor needs to reproduce the CI comparison locally
without relying on a committed benchmark file captured on different hardware.
They need a clear, documented way to choose the base ref, run both benchmark
sets on the same host, and inspect the same `benchcheck` output CI uses.

**Why this priority**: Without a reproducible same-host workflow, the gate
can fail for reasons unrelated to the code under review, especially when
developer machines and GitHub runners differ. This depends on Story 1
existing first.

**Independent Test**: Run `BENCH_BASE_REF=HEAD~1 make bench-check` locally
and verify it captures base and current benchmark files on the same host,
then invokes `internal/cmd/benchcheck` with equivalent output to CI.

**Acceptance Scenarios**:

1. **Given** a maintainer wants to compare against a specific base commit,
   **When** they set `BENCH_BASE_REF` and run `make bench-check`, **Then** the
   baseline and current benchmark runs are both captured on the same host.
2. **Given** CI runs on a pull request, **When** the performance job starts,
   **Then** it sets `BENCH_BASE_REF` to the pull request base SHA and never
   reads a committed benchmark-output file.

---

### User Story 3 - Regression Budget Is Documented (Priority: P3)

A new contributor or coding agent reads `AGENTS.md` and finds a clear,
authoritative section describing which benchmarks are tracked, what the
regression budget is, and how to reproduce the exact CI check locally before
pushing.

**Why this priority**: Without documentation, contributors discover the
budget only when CI fails, causing frustrating iteration loops. This mirrors
the existing Coverage Policy documentation pattern contributors already know.

**Independent Test**: A reader of `AGENTS.md` can find the performance
section, understand the budget, and reproduce the exact check locally using
only the documented command.

**Acceptance Scenarios**:

1. **Given** a developer opens `AGENTS.md`, **When** they look for the
   performance regression policy, **Then** they find the tracked benchmarks,
   the budget, and the local command to verify compliance.
2. **Given** the CI check fails, **When** the developer runs the documented
   local command, **Then** they get equivalent output identifying which
   benchmark(s) are over budget.

---

### Edge Cases

- What happens the very first time the check runs? The PR's base commit is
  benchmarked live in a temporary worktree on the same runner, so no
  repository baseline file must exist first.
- What happens when a new hot-path benchmark is added in a later PR that
  has no prior baseline entry? It is simply absent from the comparison (no
  prior data to regress against) until it lands on the base branch; this is
  not a failure condition.
- What happens when a previously-tracked benchmark is absent from the
  current run instead (its package fails to build, it panics, or it is
  accidentally renamed/deleted)? Unlike the new-benchmark case above, this
  is always a failure: the check must be able to tell "not yet baselined"
  apart from "used to run and now doesn't," since the latter is exactly the
  kind of silent regression this feature exists to catch.
- What happens if CI runner hardware or load causes run-to-run timing noise
  large enough to look like a regression? Base and current are benchmarked
  on the same runner in the same job, and the statistical-significance check
  (Story 1, scenario 4) absorbs normal noise; only significant,
  budget-exceeding deltas fail the build.
- What happens if a maintainer wants to accept a deliberate trade-off that
  exceeds the budget? There is no generated baseline file to update. The
  policy change must be explicit: keep the change within budget or adjust
  the budget constants in code with reviewable rationale.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The CI pipeline MUST run the repository's tracked hot-path
  benchmarks (logger emit hot path, `__schema` reflection path, error envelope
  marshal path, and Hujson/config parse path) on every pull request.
- **FR-002**: The CI pipeline MUST compare each PR's benchmark results against
  benchmark results captured from the PR base commit on the same runner and
  fail when any tracked benchmark's operation time regresses by more than the
  time budget, or its memory allocations per operation increase by more than
  the allocation budget.
- **FR-003**: The regression check MUST only fail on statistically significant
  regressions; measurement noise that is not distinguishable from the
  baseline MUST NOT fail the build.
- **FR-004**: Failure messages MUST identify which benchmark(s) exceeded the
  budget and by how much.
- **FR-005**: The benchmark command MUST use the same fixed `-cpu` value for
  base and current runs so benchmark names are portable across developer
  machines and CI runners.
- **FR-006**: The CI benchmark job MUST compare against the pull request base
  SHA in a temporary worktree on the same runner, not against a committed
  benchmark-output file.
- **FR-007**: `AGENTS.md` MUST contain a section documenting the tracked
  benchmarks, the regression budget, the local verification command, and the
  `BENCH_BASE_REF` override.
- **FR-008**: The regression check MUST be runnable locally with a single
  documented command, producing output equivalent to the CI failure message.
- **FR-009**: Any hot path named in this feature's scope that has no existing
  benchmark MUST gain one before the regression check can cover it.

### Key Entities

- **Benchmark Comparison Inputs**: The base and current benchmark-output files
  produced on the same host for one `bench-check` run. They are temporary
  files, not committed repository artifacts.
- **Regression Budget**: The maximum acceptable regression before the CI
  check fails — a time-based threshold (percentage increase in operation
  time) and an allocation-based threshold (increase in allocations per
  operation), both fixed, reviewable values.
- **Tracked Benchmark**: One of the hot paths in scope for this feature
  (logger emit, `__schema` reflection, error envelope marshal, Hujson/config
  parse) with a corresponding `testing.B` benchmark function.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A PR that regresses a tracked hot path's operation time beyond
  the budget is blocked from merging with no manual intervention required
  beyond the standard CI run.
- **SC-002**: A PR that increases a tracked hot path's allocations per
  operation beyond the budget is blocked from merging.
- **SC-003**: A developer can identify which benchmark(s) are over budget
  from the CI failure message alone, without re-running benchmarks locally.
- **SC-004**: The regression budget and update procedure are discoverable in
  `AGENTS.md` in under 60 seconds of searching.
- **SC-005**: Run-to-run measurement noise alone does not cause a CI failure
  across repeated runs of unchanged code.
- **SC-006**: A maintainer can reproduce the CI comparison locally against a
  chosen base ref using only the documented `BENCH_BASE_REF` procedure.

## Assumptions

- Source inputs: GitHub issue #22 (epic-parent #11). No governing ADR.
- The four hot paths named in FR-001 are the complete initial tracked set;
  two (logger emit, Hujson/config parse) already have benchmarks from a prior
  feature, and two (`__schema` reflection, error envelope marshal) gain new
  benchmarks as part of delivering this feature (FR-009).
- CI currently runs on GitHub Actions with a single runner image per job (no
  matrix); the regression check benchmarks base and current on that same
  runner to avoid hardware-dependent committed baselines.
- A live same-host comparison against the PR's base branch is required; a
  single stored benchmark-output file is not sufficient because GitHub
  runner hardware and developer machines vary.
- Continuous benchmarking dashboards and synthetic load testing are
  explicitly out of scope (per the issue).

## Out of Scope

- Continuous benchmarking infrastructure or dashboards beyond the CI
  pass/fail gate.
- Synthetic load testing.
