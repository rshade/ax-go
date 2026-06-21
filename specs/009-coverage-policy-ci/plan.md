# Implementation Plan: Coverage Policy and CI Enforcement

**Branch**: `009-coverage-policy-ci` | **Date**: 2026-06-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/009-coverage-policy-ci/spec.md`

## Summary

Add test-coverage floor enforcement to ax-go's CI pipeline: a custom Go tool
(`internal/cmd/covercheck`) checks per-package and repo-wide floors against a
generated `coverage.out` profile; the CI workflow calls this tool after the
existing coverage step; `AGENTS.md` documents the policy; Codecov provides the
PR delta comment and README badge.

Initial floors are calibrated to the 2026-06-16 baseline (repo-wide 70.8% →
floor 70%; per-package values per `research.md`). Three packages with 0%
baseline are explicitly excluded from per-package enforcement and listed in
`AGENTS.md` pending follow-up issues.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: None new — `covercheck` uses only the Go standard
library. Codecov integration uses the existing `codecov/codecov-action@v7`.

**Storage**: N/A (reads `coverage.out` from disk; no persistent state)

**Testing**: `go test -race ./...` (already required; Constitution Principle VII)

**Target Platform**: Linux (GitHub Actions `ubuntu-latest`); local dev on any
platform Go supports.

**Project Type**: Library — this feature adds internal tooling and CI config
only; no public API surface changes.

**Performance Goals**: `covercheck` must complete in < 1 second (the
`coverage.out` file is small; no I/O beyond one file read).

**Constraints**: Must not add new external Go module dependencies (Constitution
Principle X). Floors must not fail the first CI run (FR-010 — calibrated to
baseline per `research.md`).

**Scale/Scope**: Nine packages today; tool must handle O(100) packages without
performance concern.

**Governing ADR(s)**: N/A — spec explicitly states "No governing ADR."

## Constitution Check

*Pre-design gate: all pass — no Complexity Tracking entry required.*

| Principle | Verdict | Evidence |
|-----------|---------|---------|
| I. Stream Separation | PASS | `covercheck` stdout = pass summary; stderr = violation messages |
| II. Deterministic Output | PASS | Same `coverage.out` in → same output; no timestamps in output |
| VI. Scope (Library) | PASS | No public API or exported symbol changes; `internal/` only |
| VII. Test-First | PASS | `covercheck` tests written before implementation |
| IX. Security | PASS | No PII; file read is bounded (coverage profiles are small) |
| X. Idiomatic Go | PASS | No new dependencies; follows `internal/cmd/doccover` pattern |
| XI. Stability & SemVer | PASS | No exported symbols; no breaking change |
| ADR absorption | N/A | No governing ADR |

## Project Structure

### Documentation (this feature)

```text
specs/009-coverage-policy-ci/
├── plan.md              # This file
├── research.md          # Phase 0 — baseline, decisions, constitution check
├── data-model.md        # Phase 1 — key entities
├── quickstart.md        # Phase 1 — local developer workflow
├── contracts/           # Phase 1 — covercheck output/exit-code contract
│   └── covercheck-output.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code Changes

```text
# New files
internal/
└── cmd/
    └── covercheck/
        ├── main.go          # Floor enforcement tool (FR-002, FR-003, FR-004)
        └── main_test.go     # Table-driven tests (Constitution Principle VII)

.codecov.yml                 # Codecov configuration (FR-006, FR-007)

# Modified files
.github/
└── workflows/
    └── ci.yml               # Add cover-check step (FR-001, FR-002, FR-003)

Makefile                     # Add cover-check target
AGENTS.md                    # Add Coverage Policy section (FR-005, FR-009)
README.md                    # Add Codecov badge (FR-007)
```

## Implementation Phases

### Phase A — Core Floor Enforcement (Stories 1 + 2, P1 + P2)

**Goal**: CI fails when any package or the repo falls below its floor; policy
is documented.

#### A1. Write `covercheck` tests (TDD — tests first)

File: `internal/cmd/covercheck/main_test.go`

Test cases (table-driven):

1. `TestParseCoverage` — parses a synthetic `coverage.out` into per-package maps
2. `TestCheckFloors_Pass` — all packages above floor → exit 0
3. `TestCheckFloors_PackageViolation` — one package below floor → exit 1, message names the package
4. `TestCheckFloors_RepoWideViolation` — aggregate below floor → exit 1
5. `TestCheckFloors_ExcludedPackage` — excluded package at 0% → does not fail per-package gate
6. `TestCheckFloors_MissingFile` — non-existent coverage file → exit 2
7. `TestFormatOutput_Pass` — pass summary format, asserted against a **golden
   file** under `internal/cmd/covercheck/testdata/` (the output is stable-by-
   contract per `contracts/covercheck-output.md`; Constitution Principle VII
   requires golden files for stable-by-contract output, and a golden file fails
   loudly on drift)
8. `TestFormatOutput_Fail` — failure format (package name, actual%, floor%,
   delta), asserted against a golden file under the same `testdata/` directory

Update golden files only via an explicit `-update` test flag, never by hand, so
drift remains a deliberate, reviewable change.

Fuzz test: `FuzzParseCoverageProfile` — feed random bytes to the profile parser;
must not panic (Constitution Principle VII).

#### A2. Implement `covercheck`

File: `internal/cmd/covercheck/main.go`

```text
package main

Entities:
- packageCoverage: map[importPath]struct{stmts, covered int}
- floorConfig:
    repoWideFloor = 70.0  // baseline 70.8%
    packageFloors = map[string]float64{ ... per research.md table ... }
    excludedPkgs  = []string{ internal/cli, internal/mcp, internal/schema }

Flow:
  1. Parse -coverage flag; open file (exit 2 on missing/unreadable)
  2. Parse coverage.out: accumulate stmts/covered per package
  3. Compute per-package coverage %
  4. Check each non-excluded package vs its floor (or a default floor
     if not in the explicit map)
  5. Compute aggregate coverage % = sum(covered) / sum(stmts)
  6. Check aggregate vs repoWideFloor
  7. Print pass summary to stdout OR violations to stderr; exit accordingly
```

The `defaultPackageFloor` for packages not in the explicit map: `25.0%` (the
lowest initial floor in the table). This ensures any new package added to the
module without explicit configuration faces a non-trivial gate.

**`context.Context` deviation (intentional)**: Constitution Principle X makes
`context.Context` the first parameter of functions doing I/O. `covercheck` is a
sub-second `internal/cmd/` main that reads a single, bounded file and exits;
there is no cancellation, network, or goroutine surface to govern. The parser
and floor-check functions therefore deliberately omit `context.Context`. This is
a recorded decision, not an oversight — if `covercheck` ever grows I/O that can
block or be canceled, thread context at that point.

#### A3. Add Makefile target

<!-- markdownlint-disable MD010 -->
```makefile
.PHONY: cover-check
cover-check: test-cover
	@echo "Checking coverage floors..."
	go run ./internal/cmd/covercheck -coverage coverage.out
```

<!-- markdownlint-enable MD010 -->

`cover-check` depends on `test-cover` to ensure `coverage.out` is fresh.

#### A4. Add CI step

In `ci.yml`, after the existing "Run tests with race detector and coverage" step:

```yaml
- name: Enforce coverage floors
  run: go run ./internal/cmd/covercheck -coverage coverage.out
```

This step runs only in the `test` job (same job that produces `coverage.out`).

#### A5. Add AGENTS.md Coverage Policy section

New section in `AGENTS.md` — insert after the Testing-First Discipline section:

````markdown
## Coverage Policy

### Floors

| Scope | Initial Floor | Aspirational Target |
|-------|---------------|---------------------|
| Repo-wide (aggregate) | 70% | 85% |
| Per-package default | 25% | 80% |

Per-package overrides (from 2026-06-16 baseline):

| Package | Initial Floor |
|---------|---------------|
| `github.com/rshade/ax-go` | 80% |
| `github.com/rshade/ax-go/examples/integration` | 85% |
| `github.com/rshade/ax-go/internal/cmd/doccover` | 45% |
| `github.com/rshade/ax-go/internal/config` | 65% |
| `github.com/rshade/ax-go/internal/telemetry` | 60% |
| `github.com/rshade/ax-go/internal/testutil` | 25% |

### Excluded from Per-Package Floor Enforcement

These packages have 0% baseline coverage and are pending test implementation:

| Package | Reason |
|---------|--------|
| `github.com/rshade/ax-go/internal/cli` | No tests written; follow-up issue |
| `github.com/rshade/ax-go/internal/mcp` | No tests written; follow-up issue |
| `github.com/rshade/ax-go/internal/schema` | No tests written; follow-up issue |

Excluded packages still count toward the repo-wide aggregate. Their 0%
contribution is why the repo-wide initial floor is 70%, not 85%.

### Local Verification

Run the exact same check CI runs:

```bash
make cover-check
```

Or step by step:

```bash
go test -race -coverprofile=coverage.out -covermode=atomic ./...
go run ./internal/cmd/covercheck -coverage coverage.out
```

### Raising a Floor

1. Improve coverage in the target package.
2. Edit the `packageFloors` map in `internal/cmd/covercheck/main.go`.
3. Verify locally with `make cover-check`.
4. The commit message records why the floor was raised (floor changes are
   auditable via `git blame`).

### Escalation Path

Floors escalate in 5pp increments as tests are added. The goal is to reach
80% per-package and 85% repo-wide by moving excluded packages into scope first.
````

---

### Phase B — PR Delta Visibility (Story 3, P3)

**Goal**: PR authors and reviewers see coverage delta without running locally.

#### B1. Add `.codecov.yml`

```yaml
coverage:
  status:
    project:
      default:
        target: 70%       # matches repoWideFloor
        threshold: 1%     # allow 1pp drift on the PR before flagging
    patch:
      default:
        target: 70%
        threshold: 5%

comment:
  layout: "reach,diff,flags,files"
  behavior: default
  require_changes: true   # only comment when coverage changes
```

#### B2. Verify Codecov token is configured

The `codecov/codecov-action@v7` step in `ci.yml` does not currently pass a
`token` parameter. For public repos, Codecov does not require a token for
upload. Verify the upload succeeds in CI after this PR merges; if it does not,
add `CODECOV_TOKEN` to GitHub Secrets and pass it as `token: ${{ secrets.CODECOV_TOKEN }}`.

---

### Phase C — README Badge (Story 4, P4)

**Goal**: Repo README shows a live coverage badge.

#### C1. Add badge to README.md

Add after the existing CI badges (or create a badge row if none exist):

```markdown
[![Coverage](https://codecov.io/gh/rshade/ax-go/branch/main/graph/badge.svg)](https://codecov.io/gh/rshade/ax-go)
```

---

## Complexity Tracking

> No violations — table not required.

## Risk Register

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Codecov token missing (private repo) | Low (repo is public) | Document fallback in Phase B |
| coverage.out not produced on CI timeout | Low | `cover-check` exits 2 (bad input) with clear message; separate concern |
| A new package added with 0% coverage | Medium | `defaultPackageFloor = 25%` gate catches it; contributor must fix or add to exclusion list with justification |
| Baseline drift between now and merge | Low | 0.8pp buffer on repo-wide floor; 2-5pp buffer on per-package floors |
