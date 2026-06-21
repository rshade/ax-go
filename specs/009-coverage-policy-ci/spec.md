# Feature Specification: Coverage Policy and CI Enforcement

**Feature Branch**: `009-coverage-policy-ci`

**Created**: 2026-06-16

**Status**: Draft

**Input**: User description: "Test coverage policy: floor, tracking, CI enforcement (GitHub issue #21)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Coverage Regression Blocked in CI (Priority: P1)

A contributor opens a PR that adds new behavior without writing corresponding
tests. The CI pipeline detects that coverage has dropped below the established
floor and fails the check, preventing the PR from merging until tests are added.

**Why this priority**: This is the core safety guarantee — coverage cannot
silently decay without a developer being forced to address it. All other stories
depend on a working floor first.

**Independent Test**: Submit a PR that removes a test file and verify CI fails
with a clear coverage-floor violation message identifying the affected package.

**Acceptance Scenarios**:

1. **Given** a PR removes an existing test file, **When** the CI coverage check
   runs, **Then** the check fails and the message names the package(s) that fell
   below the floor.
2. **Given** a PR adds a new package with no tests, **When** the CI coverage
   check runs, **Then** the check fails when per-package coverage falls below
   the package floor.
3. **Given** a PR adds code with adequate tests, **When** the CI coverage check
   runs, **Then** the check passes and the PR can proceed to review.

---

### User Story 2 - Coverage Policy Is Documented (Priority: P2)

A new contributor or coding agent reads `AGENTS.md` and finds a clear,
authoritative section describing the project's coverage floors, what is and
isn't counted, and how to run coverage locally before pushing.

**Why this priority**: Without documentation, developers discover the floor
only when CI fails, causing frustrating iteration loops. Documented policy
enables proactive compliance.

**Independent Test**: A reader of `AGENTS.md` can find the coverage section,
understand the floors, and reproduce the exact check locally using only the
documented command.

**Acceptance Scenarios**:

1. **Given** a developer opens `AGENTS.md`, **When** they search for
   "coverage", **Then** they find a section stating the per-package floor, the
   repo-wide floor, and the local command to verify compliance.
2. **Given** the CI check fails, **When** the developer runs the documented
   local command, **Then** they get equivalent output that identifies which
   package(s) are below the floor.

---

### User Story 3 - Coverage Delta Visible on PRs (Priority: P3)

A PR author and reviewers can see at a glance how a PR changes coverage —
which packages improved or regressed and the overall delta — without running
coverage locally.

**Why this priority**: Visible deltas encourage proactive improvement and make
code review conversations about test quality concrete and data-driven. Valuable
but not blocking: PRs can merge once they pass the floor.

**Independent Test**: Open a PR that changes test coverage; the status check or
automated comment shows overall percentage and delta from the base branch.

**Acceptance Scenarios**:

1. **Given** a PR is opened, **When** CI completes, **Then** a status check or
   automated comment surfaces the overall coverage percentage and delta from the
   base branch.
2. **Given** a PR increases coverage, **When** the status is reported, **Then**
   the delta is shown as positive.
3. **Given** a PR decreases coverage but stays above the floor, **When** the
   status is reported, **Then** the delta is shown as negative with a warning,
   but the check still passes.

---

### User Story 4 - Coverage Badge in README (Priority: P4)

A prospective user or contributor visits the repository and sees the current
coverage percentage in the README, giving immediate confidence in the project's
test quality.

**Why this priority**: Improves project credibility and provides a trailing
indicator of policy health. Depends on story 3's coverage service integration.

**Independent Test**: Open the repository README and see a coverage badge that
displays the current percentage; clicking the badge shows detailed coverage history.

**Acceptance Scenarios**:

1. **Given** a user visits the README, **When** the page loads, **Then** they
   see a coverage badge showing the current percentage.
2. **Given** coverage changes after a merge, **When** the README is viewed,
   **Then** the badge reflects the updated value within one CI run of the merge.

---

### Edge Cases

- What happens when a brand-new package has zero tests? The per-package floor
  applies immediately; there is no grace period for new packages.
- How does the floor apply to genuinely untestable inputs (e.g., generated
  code, `testdata/` fixtures)? These are excluded from measurement **natively by
  the Go cover tool** — they never appear in `coverage.out` — so no per-feature
  handling is required. Separately, testable packages that simply have no tests
  yet (`internal/cli`, `internal/mcp`, `internal/schema`, all 0% baseline) are
  excluded at the **package level** from per-package enforcement, documented in
  `AGENTS.md` with follow-up issues; they still count toward the aggregate.
- What happens if the repo-wide floor passes but one package is below the
  per-package floor? Both floors must independently pass; the repo-wide floor
  does not override the per-package floor.
- What happens when a package is deleted? Its removal improves the aggregate
  automatically; no special handling required.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The CI pipeline MUST measure statement-level test coverage on
  every pull request and push to the main branch.
- **FR-002**: The CI pipeline MUST fail when any package's coverage falls below
  the per-package floor (aspirational target: 80%; the **initial** enforced
  floors are calibrated to the measured baseline per FR-010 — see `research.md`).
- **FR-003**: The CI pipeline MUST fail when the aggregate repository-wide
  coverage falls below the repo-wide floor (aspirational target: 85%; the
  **initial** enforced floor is calibrated to the measured baseline per FR-010 —
  see `research.md`).
- **FR-004**: Failure messages MUST identify which packages are below the floor
  and by how much.
- **FR-005**: `AGENTS.md` MUST contain a Coverage Policy section documenting
  the per-package floor, the repo-wide floor, the local verification command,
  and the list of excluded packages or paths.
- **FR-006**: The CI pipeline MUST surface the coverage percentage and delta
  from the base branch for every PR (via status check or automated comment).
- **FR-007**: The README MUST display a badge showing the current repo-wide
  coverage percentage.
- **FR-008**: Coverage measurement MUST run with the race detector enabled,
  consistent with the existing test requirements (Constitution Principle VII).
- **FR-009**: Packages or files explicitly excluded from floor enforcement MUST
  be listed in `AGENTS.md` alongside the policy.
- **FR-010**: The initial floors MUST be set at or below the actual baseline
  measurement so that the first CI run does not fail; floors are then escalated
  toward the targets incrementally.

### Key Entities

- **Coverage Report**: The aggregate and per-package test coverage output from
  a single CI run. Attributes: commit SHA, per-package percentages, aggregate
  percentage, pass/fail status relative to each floor.
- **Per-Package Floor**: A minimum acceptable coverage percentage for each
  individual package in the module (aspirational target: 80%; initial enforced
  floors are calibrated to baseline per FR-010 — a per-package override map plus
  a 25% default). Catches isolated low-coverage packages that an aggregate floor
  would mask.
- **Repo-Wide Floor**: A minimum acceptable aggregate coverage percentage
  across the entire module (aspirational target: 85%; initial enforced floor
  calibrated to baseline per FR-010, currently 70%). Catches global trend
  regressions that per-package averaging might miss.
- **Coverage Baseline**: The measured coverage at the time this policy is first
  enforced; used to set initial floor values that do not immediately break CI.
- **Exclusion List**: The set of **package-level import paths** exempt from
  *per-package* floor enforcement. In this feature the exclusions are packages
  with a 0% measured baseline whose tests are pending (`internal/cli`,
  `internal/mcp`, `internal/schema`) — they are testable, not untestable, and
  each is tracked by a follow-up issue. Excluded packages still count toward the
  repo-wide aggregate. Note: `testdata/` fixtures and any generated code are
  excluded from measurement **natively by the Go cover tool**, not by this
  feature's exclusion list. The exclusion list MUST be documented in `AGENTS.md`
  and version-controlled (it lives as a typed slice in
  `internal/cmd/covercheck/main.go`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: PRs that cause any package's coverage to fall below the
  per-package floor are blocked from merging with no manual intervention
  required beyond the standard CI run.
- **SC-002**: PRs that cause repo-wide coverage to fall below the repo-wide
  floor are blocked from merging.
- **SC-003**: A developer can identify which package(s) are failing the floor
  from the CI failure message alone, without running coverage locally.
- **SC-004**: Coverage policy is discoverable in `AGENTS.md` in under 60
  seconds of searching.
- **SC-005**: Every PR surfaces a coverage delta so reviewers see trend
  direction at the time of review.
- **SC-006**: The README badge reflects the most recent main-branch coverage
  within one CI run of a merge (typically within minutes).
- **SC-007**: The initial CI run after this policy is adopted passes without
  any code changes (floors are calibrated to the baseline, not the aspirational
  targets).

## Assumptions

- Source inputs: GitHub issue #21. No governing ADR.
- The current test suite is assumed to meet or exceed the proposed floor values;
  if the baseline measurement during planning reveals otherwise, the initial
  floors will be set to the actual baseline values and escalated toward 80%/85%
  incrementally (FR-010).
- Generated code and test fixture paths (e.g., `testdata/`,
  `internal/cmd/doccover/testdata/`) may be excluded from per-package floor
  enforcement; the exact exclusion list is confirmed in the plan's research
  phase via the baseline run.
- CI currently runs on GitHub Actions; the coverage step will be added to the
  existing workflow rather than a new file.
- The race detector (`-race`) flag is already part of the standard test
  invocation; coverage will be collected in the same run to avoid doubling CI
  time.
- A coverage service (e.g., Coveralls or Codecov) is in scope for the badge
  and PR delta display; the specific service is a planning-phase decision based
  on integration cost and free-tier availability.
- The coverage badge and PR delta display (stories 3 and 4) depend on a
  coverage service integration; if no service is chosen, a GitHub Actions
  summary can serve as a lighter-weight alternative for the delta display.
