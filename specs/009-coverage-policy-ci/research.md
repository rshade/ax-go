# Research: Coverage Policy and CI Enforcement

**Feature**: `009-coverage-policy-ci`
**Phase**: 0 — Outline & Research
**Date**: 2026-06-16

---

## Baseline Coverage Measurement

Coverage run: `go test -race -coverprofile=coverage.out -covermode=atomic ./...`
(run 2026-06-16; go 1.26.4; `-race` required per Constitution Principle VII)

### Per-Package Results

| Package | Baseline | Status |
|---------|----------|--------|
| `github.com/rshade/ax-go` | 83.7% | Above 80% aspirational target |
| `github.com/rshade/ax-go/examples/integration` | 87.5% | Above 85% aspirational target |
| `github.com/rshade/ax-go/internal/cli` | 0.0% | **Excluded** — no tests written yet |
| `github.com/rshade/ax-go/internal/cmd/doccover` | 49.6% | Below aspirational target |
| `github.com/rshade/ax-go/internal/config` | 69.6% | Below aspirational target |
| `github.com/rshade/ax-go/internal/mcp` | 0.0% | **Excluded** — no tests written yet |
| `github.com/rshade/ax-go/internal/schema` | 0.0% | **Excluded** — no tests written yet |
| `github.com/rshade/ax-go/internal/telemetry` | 65.8% | Below aspirational target |
| `github.com/rshade/ax-go/internal/testutil` | 29.4% | Below aspirational target |
| **Total (aggregate)** | **70.8%** | Below aspirational 85% target |

### Why Three Packages Are at 0%

`internal/cli`, `internal/mcp`, and `internal/schema` are implementation
packages with exported functions but no `_test.go` files. They are fully
internal (Go toolchain blocks external import), so they have no existing
downstream consumers whose behavior validates the code. They are not generated
code and are not `testdata/` fixtures — they require unit tests, which this
feature treats as follow-up work (filed as GitHub issues during implementation).

---

## Decision 1: Initial Floor Values (FR-010)

**Decision**: Set initial floors calibrated to the baseline, then escalate
incrementally toward aspirational targets.

### Repo-Wide Floor

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Initial floor | 70% | Baseline is 70.8% — 70% provides a 0.8pp safety buffer |
| Aspirational target | 85% | From spec; reached by incrementally raising |

### Per-Package Floors (initial)

Per-package floor enforced only on non-excluded packages. Each initial floor is
set at the next 5%-multiple at or below the baseline to allow minor baseline
drift without immediately breaking CI.

| Package | Baseline | Initial Floor | Aspirational |
|---------|----------|---------------|--------------|
| `github.com/rshade/ax-go` | 83.7% | 80% | 80% ← already met |
| `examples/integration` | 87.5% | 85% | 85% ← already met |
| `internal/cmd/doccover` | 49.6% | 45% | 80% |
| `internal/config` | 69.6% | 65% | 80% |
| `internal/telemetry` | 65.8% | 60% | 80% |
| `internal/testutil` | 29.4% | 25% | 80% |

### Exclusion List (FR-009)

Excluded from per-package floor enforcement; still included in the aggregate
(their 0% counts toward the repo-wide total):

| Package | Reason | Follow-up |
|---------|--------|-----------|
| `internal/cli` | 0% baseline — no tests written | File GitHub issue |
| `internal/mcp` | 0% baseline — no tests written | File GitHub issue |
| `internal/schema` | 0% baseline — no tests written | File GitHub issue |

**Rationale**: Excluding them from the per-package floor prevents day-zero CI
failure while keeping them visible in the aggregate metric. The exclusion list
is documented in `AGENTS.md` (FR-009); shrinking it is the most direct path to
raising the repo-wide floor.

**Alternatives considered**:
- Set a single per-package floor of 0% (defeats the purpose).
- Grace-period flag per package (more complex, less transparent).

---

## Decision 2: Floor Enforcement Mechanism

**Decision**: Custom Go tool at `internal/cmd/covercheck/main.go`.

**Rationale**: The project already uses a custom Go tool for doc coverage
(`internal/cmd/doccover`). A sister tool is idiomatic, testable, has no new
dependencies, and can produce output consistent with ax-go's `ax.Error` style.
The tool reads `coverage.out` (the standard `go test -coverprofile` output)
and enforces floors.

**Alternatives considered**:
- Shell script in `Makefile`: brittle, hard to test, harder to keep cross-platform.
- Third-party tool (`gocovcheck`, `gocover-cobertura`): adds a dependency; most
  don't support per-package floors with exclusion lists out of the box.
- GitHub Actions bot (e.g., `go-test-coverage` Action): harder to run locally;
  couples policy to CI vendor; violates the "run locally first" principle.

### covercheck Tool Behaviour

Input: `-coverage coverage.out` flag pointing to a standard Go coverage profile.

Output on pass (stdout):
```
PASS  coverage floors met
  repo-wide:          83.7% >= 70.0% floor
  github.com/rshade/ax-go:     83.7% >= 80.0% floor
  ... (one line per non-excluded package)
```

Output on failure (stderr):
```
FAIL  coverage floor violations:
  github.com/rshade/ax-go/internal/config: 55.0% < 65.0% floor (-10.0pp)
exit status 1
```

Exit codes: `0` pass, `1` floor violation, `2` bad input (missing/malformed
coverage file).

### Floor Configuration

Floors are **hardcoded as typed constants** in `covercheck/main.go`. This is
intentional: every floor change is a Go-code commit, which means git blame
records who raised the floor and why. A separate config file would separate the
policy from its audit trail.

---

## Decision 3: Coverage Service for Badge and PR Delta (Stories 3–4)

**Decision**: Use Codecov, which is already integrated in `ci.yml`.

The CI pipeline already has:
```yaml
- uses: codecov/codecov-action@v7
  with:
    files: coverage.out
    fail_ci_if_error: false
```

**What to add**:
1. `.codecov.yml` configuration to enable PR comments, status checks, and badge.
2. `fail_ci_if_error: false` remains (Codecov upload failure must not block
   a PR; the custom `covercheck` tool is the authoritative gate).
3. Codecov badge URL added to `README.md`.

**Codecov status checks**: Codecov's `coverage/project` and `coverage/patch`
status checks will surface coverage delta on every PR. These are configured via
`.codecov.yml`. They are informational / advisory — the mandatory floor gate
is the `covercheck` tool step, not the Codecov status check.

**Alternatives considered**:
- Coveralls: fewer free features; similar integration cost.
- GitHub Actions summary only: no persistent badge, no historical trend.
- Custom script comparing against base branch: Codecov already handles this
  with less maintenance.

---

## Decision 4: Makefile Target Name

**Decision**: `make cover-check` (enforces floors) and extend existing `make test-cover` to still produce the profile.

Updated developer workflow:
```bash
make test-cover      # runs tests with -race + coverage; produces coverage.out
make cover-check     # reads coverage.out, enforces floors, exits 1 on violation
```

Both steps are composed in CI as sequential shell commands.

---

## Constitution Check

| Principle | Impact | Status |
|-----------|--------|--------|
| I. Stream Separation | `covercheck` stdout = pass summary; stderr = violations | ✅ |
| II. Deterministic Output | Same `coverage.out` → identical output | ✅ |
| VI. Scope (Library, Not Application) | No public API changes; `internal/` only | ✅ |
| VII. Test-First | `covercheck` tests written before implementation | ✅ |
| IX. Security | No PII/secrets; no unbounded input | ✅ |
| X. Idiomatic Go | No new external dependencies; `internal/cmd/` pattern | ✅ |
| XI. Stability & SemVer | No exported symbols changed; no breaking change | ✅ |
| ADR absorption | spec.md: "No governing ADR" | N/A |

No violations — Complexity Tracking table not required.

---

## Open Questions (Resolved)

| # | Question | Resolution |
|---|----------|------------|
| 1 | Does Codecov have a free tier for public repos? | Yes — unlimited for public repos |
| 2 | Is a Codecov token already configured in GitHub Secrets? | Assumed yes (action already present); if not, the action fails silently (`fail_ci_if_error: false`) |
| 3 | Should `examples/integration` be excluded? | No — it has 87.5% coverage, well above the 85% initial floor; keep it in scope |
| 4 | Should generated code be excluded? | No generated code exists in this module today; `testdata/` directories are already excluded by Go's coverage tool |
| 5 | Per-package floor: single value or per-package map? | Per-package map (see Decision 1); single value would force us to the minimum (25%) across all packages, losing the ability to gate the root `ax` package at 80% |
