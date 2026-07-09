# Phase 1 Data Model: Performance Regression Budget in CI

**Feature**: `014-bench-regression-budget` | **Date**: 2026-07-08

This feature introduces one new `internal/` package (`internal/cmd/benchcheck`)
with its own small set of types, plus two new test-only benchmarks. No change
to the public `ax` API surface — no `go-apidiff` impact, no SemVer bump.

---

## Entity: Benchmark Comparison Inputs

The two same-host benchmark-output files that `benchcheck` compares: one
captured from `BENCH_BASE_REF` and one captured from the current worktree.

| Attribute | Meaning |
|-----------|---------|
| Location | Temporary files created by `make bench-check` |
| Format | Raw `go test -run='^$' -bench=. -cpu=1 -count=10 -benchmem ./...` stdout |
| Base selection | `BENCH_BASE_REF`; CI sets it to the pull request base SHA, local runs default to `git merge-base HEAD origin/main`, then `HEAD~1` |
| Consumers | `make bench-check` feeds the files to `benchcheck -baseline` and `benchcheck -current` |

**Validation / invariants**:

- MUST be captured on the same host and same Go toolchain invocation so raw
  `ns/op` values are comparable without baking in local hardware.
- MUST be produced with `-count=10` so `benchstat`'s significance test has
  enough samples per benchmark line to be meaningful (verified during
  planning research: single-sample fixtures never register as significant).
- MUST be produced with `-cpu=1` so benchmark names match across machines
  and do not encode host core count.

---

## Entity: Regression Budget

The fixed, reviewable thresholds that separate "normal variation" from "a
regression that fails CI."

| Attribute | Value | Constant name (planned) |
|-----------|-------|--------------------------|
| Time regression budget | 5% increase in `ns/op`, counted only when `benchstat` marks the delta statistically significant (`Change == -1`) | `maxNsOpRegressionPercent = 5.0` |
| Allocation regression budget | +1 increase in mean `allocs/op` (absolute, not percent) | `maxAllocsOpIncrease = 1` |
| Significance level | α = 0.05 (benchstat default) | `benchstat.Collection{Alpha: 0.05}` |

**Validation / invariants**:

- Both budgets are hardcoded Go constants in `internal/cmd/benchcheck/main.go`
  (spec FR-002/FR-004; mirrors `covercheck`'s `defaultFloorConfig` pattern) —
  every change is a reviewable, `git blame`-auditable commit, not external
  config.
- A benchmark failing either budget independently fails the check (spec
  Key Entities); a benchmark can fail on time, allocations, or both, and the
  failure message names each independently (spec FR-004/SC-003).

---

## Entity: Tracked Benchmark

One hot path in scope for this feature, with a corresponding `testing.B`
function.

| Benchmark ID | Hot path | Status |
|--------------|----------|--------|
| `BenchmarkLoggerEmit/*`, `BenchmarkLoggerTracingHook/*`, `BenchmarkLoggerFieldShapes/*` | Logger emit path | Existing (`logger_bench_test.go`, feature 011) |
| `BenchmarkParseConfigBoundedRead`, `BenchmarkParseConfigDefaultCapRead` | Hujson/config parse path | Existing (`config_bench_test.go`) |
| `BenchmarkBuildCommand` | `__schema` reflection path | **NEW** (`internal/schema/schema_bench_test.go`) |
| `BenchmarkWriteError` | Error envelope marshal path | **NEW** (`contract/error_bench_test.go`) |

**Validation / invariants**:

- Every Tracked Benchmark is included in the same `go test -bench=.`
  invocation used for both base and current checks (spec FR-001) — no
  benchmark is selectively excluded from the comparison.
- A benchmark added after this feature ships (not in this table) is simply
  absent from `benchstat`'s comparison until it lands on the base branch —
  not a failure condition (spec Edge Cases).

---

## Entity: `benchcheck` internal types (`internal/cmd/benchcheck/main.go`)

Mirrors `covercheck`'s `Violation`/`CheckResult` shape.

| Type | Fields | Purpose |
|------|--------|---------|
| `Violation` | `Benchmark string`, `Metric string` (`"ns/op"` or `"allocs/op"`), `Delta float64`, `Budget float64` | One regression-budget failure, named and quantified for the stderr message (spec FR-004) |
| `CheckResult` | `Benchmarks []string`, `Violations []Violation`, `Missing []string` | The complete outcome of one `benchcheck` run, including comparable benchmarks and baseline benchmarks missing from the current run |

**Validation / invariants**:

- `run(baselinePath, currentPath string, stdout, stderr io.Writer) int` is
  the pure, testable core (mirrors `covercheck.run`), returning the process
  exit code: `0` pass, `1` violation, `2` bad input.
- Reading either input file is bounded (mirrors `covercheck`'s
  `maxProfileBytes` pattern) so a malformed/oversized file is a deterministic
  exit-2 validation error, not unbounded memory growth (Constitution
  Principle IX).

---

## Summary

- **New exported `ax` API surface**: none.
- **New `internal/` package**: `internal/cmd/benchcheck` (`Violation`,
  `CheckResult`, `run`, budget constants).
- **New test artifacts**: `internal/schema/schema_bench_test.go`,
  `contract/error_bench_test.go`, `internal/cmd/benchcheck/main_test.go`.
- **New data artifact**: none; benchmark outputs are temporary per run.
