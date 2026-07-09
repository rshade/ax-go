# Phase 0 Research: Performance Regression Budget in CI

**Feature**: `014-bench-regression-budget` | **Date**: 2026-07-08

This document resolves the technical unknowns for the CI regression gate.
There is no governing ADR to absorb. There are no open `NEEDS CLARIFICATION`
items.

---

## Decision 1 — Comparison engine: `golang.org/x/perf/benchstat`'s `Collection` API

**Decision**: Import `golang.org/x/perf/benchstat` as a library inside
`internal/cmd/benchcheck`. Build a `benchstat.Collection{Alpha: 0.05,
DeltaTest: benchstat.UTest}`, call `AddConfig("baseline", baselineBytes)` and
`AddConfig("current", currentBytes)`, then iterate `Tables()`. Each `*Table`
corresponds to one metric (`time/op`, `alloc/op`, `allocs/op`); each `*Row`
gives `Benchmark`, `PctDelta`, and `Change` (`-1` = significantly worse, `+1`
= significantly better, `0` = not significant).

**Rationale**: This is verified working — compiled and run against a real
5-sample-per-config fixture during planning research, confirming
`Change == -1` + `PctDelta` is the correct pair to threshold on. It is also
the exact engine the upstream `benchstat` CLI tool (which the issue names
explicitly) uses internally, so `benchcheck`'s verdicts match what a
developer running `benchstat` by hand would see.

**Important caveat recorded here**: the `benchstat` library subpackage
carries a deprecation notice pointing to `golang.org/x/perf/benchproc` and
`golang.org/x/perf/benchmath` as its replacement. It still compiles and
functions correctly. `benchproc`/`benchmath` expose lower-level primitives
(explicit query/projection and statistics-computation building blocks) that
would require substantially more integration code to reach the same
`Row{Benchmark, PctDelta, Change}` comparison `Collection` already provides
in ~10 lines. This is a deliberate, revisitable trade-off: if `benchstat` is
ever removed from `x/perf` (not merely deprecated), `benchcheck` migrates to
the newer packages then, not now.

**Alternatives considered**:

- *Shell out to the `benchstat` CLI and text-scrape its output*: rejected —
  string-parsing a formatted table is more fragile than calling a typed Go
  API directly, and the CLI would become an unmanaged external tool
  dependency (not `go.mod`-tracked, not vendorable, version drift across
  developer machines and CI).
- *`benchproc`/`benchmath` directly*: rejected for now per the caveat above —
  more integration code for an equivalent result. Recorded as the natural
  next step if `benchstat` is ever fully removed.
- *Hand-rolled Welch's t-test or Mann-Whitney U-test*: rejected — reinventing
  a well-tested statistics library is exactly the kind of dependency
  Constitution Principle X says to avoid duplicating.

---

## Decision 2 — Budget thresholds: >5% ns/op (significant only), >1 alloc/op absolute

**Decision**: `maxNsOpRegressionPercent = 5.0` (only counted when `benchstat`
marks the row `Change == -1`, i.e. statistically significant at α=0.05) and
`maxAllocsOpIncrease = 1` (an absolute increase in mean allocations per
operation, not a percentage).

**Rationale**: These are the exact numbers already committed in
`ROADMAP.md`'s description of issue #22 ("regression budget (>5% ns/op or >1
alloc/op)") — not a new decision, a confirmation. Gating `ns/op` only on
significant deltas satisfies spec FR-003/SC-005 (noise alone must not fail
CI); most of today's benchmarks report `0 allocs/op` (per
`specs/011-hot-path-benchmarks/research.md`'s measured profile), so an
absolute `+1` threshold is meaningful and simple, whereas a percentage
threshold against a `0` baseline is undefined/infinite and would need a
special case anyway.

**Alternatives considered**:

- *Percentage threshold for allocs/op too*: rejected — division by a `0`
  baseline is either undefined or requires an arbitrary "treat 0 as 1"
  fudge; an absolute threshold is simpler and matches the issue's literal
  wording.
- *Single combined score instead of independent thresholds*: rejected —
  the issue and spec (FR-002) ask for two independent failure conditions
  with clear, separately-named causes (SC-003 requires identifying which
  metric was exceeded).

---

## Decision 3 — Baseline strategy: same-host base-ref comparison, not a static committed file

**Decision**: `make bench-check` benchmarks `BENCH_BASE_REF` in a temporary
git worktree and benchmarks the current worktree on the same host, then
passes those two files to `internal/cmd/benchcheck`. CI sets
`BENCH_BASE_REF` to the pull request base SHA. Local runs default to
`git merge-base HEAD origin/main`, then `HEAD~1` if `origin/main` is not
available, and can override the ref explicitly. Both benchmark runs use
`-cpu=1`.

**Rationale**: `benchcheck` enforces raw `ns/op` and `allocs/op` budgets, so
the two input files must be produced on comparable hardware. A committed
benchmark-output file inevitably records one laptop or one GitHub runner
generation and can fail unchanged code when the check runs somewhere else.
Capturing base and current in the same job avoids that portability problem
while preserving the reviewable policy surface: budget thresholds remain
hardcoded constants in Go code. Pinning `-cpu=1` also makes benchmark names
portable (`BenchmarkFoo-1`) instead of encoding host core count.

**Trade-off, accepted**: the CI job runs two benchmark passes instead of one.
That is a deliberate cost to keep the performance gate meaningful across
variable GitHub runners and developer machines.

**Alternatives considered**:

- *Static committed `bench/baseline.txt`*: rejected after review — it bakes
  arbitrary local hardware into the repository and cannot be compared
  reliably against `ubuntu-latest` runners or other developer machines.
- *Compare against a generated baseline from the current worktree*: rejected
  because it would always compare the code to itself and fail to detect
  regressions.

---

## Decision 4 — New benchmark fixtures: representative, not exhaustive

**Decision**:

- `internal/schema/schema_bench_test.go`: `BenchmarkBuildCommand` builds a
  small but representative `*cobra.Command` tree (a root command, 2-3
  subcommands, each with a handful of flags of different types) once outside
  the timed loop, then calls `schema.BuildCommand` inside `b.Loop()`.
- `contract/error_bench_test.go`: `BenchmarkWriteError` builds one `*Error`
  via `NewError` with `WithErrorContext` (a few fields) and `WithSuggestions`
  (a few entries) populated once outside the timed loop — the realistic
  worst case for the marshal path — then calls `WriteError(io.Discard, err)`
  inside `b.Loop()`.

**Rationale**: Matches the idiom `specs/011-hot-path-benchmarks` already
established (fixture construction outside the loop, `io.Discard` sink,
`b.Loop()`). A "representative" tree/error is sized to reflect real CLI
command trees and real error envelopes seen in this codebase, not a
worst-case stress test — the goal is a stable, low-noise baseline for
regression detection, not a load-testing benchmark (explicitly out of scope
per the issue and spec Out of Scope).

**Alternatives considered**:

- *Multiple size variants (small/medium/large command tree)*: rejected as
  unnecessary complexity for a regression-detection benchmark — a single
  representative fixture is enough to catch an allocation regression in the
  reflection or marshal path itself; size-scaling behavior is not what this
  feature is measuring.

---

## Decision 5 — CI wiring: PR-only dedicated `bench` job

**Decision**: Add one new job named `bench` to `.github/workflows/ci.yml`,
pinned to `ubuntu-latest` with the same `setup-go@v6` + `GO_VERSION:
'1.26.4'` shape as the existing jobs, running `make bench-check` only for
pull requests. The job fetches full history so the base SHA is available,
sets `BENCH_BASE_REF` to `github.event.pull_request.base.sha`, and lets the
Makefile capture base and current benchmarks on the same runner.

**Rationale**: A dedicated job (rather than folding into the existing `test`
job) keeps the `-count=10` benchmark run's added CI time isolated and
visible as its own status check, and avoids coupling benchmark timing to the
`-race` build's coverage instrumentation (which itself has measurable
overhead that would contaminate benchmark timing if run in the same
process). Limiting the job to pull requests avoids a post-merge push job
turning `main` red after a maintainer intentionally accepts a performance
trade-off through normal review/override policy; future PRs compare against
the merged base branch state.

**Alternatives considered**:

- *Fold `bench-check` into the existing `test` job*: rejected — couples
  unrelated concerns (coverage vs. performance) into one job's pass/fail
  signal and risks coverage instrumentation overhead polluting benchmark
  timing.
- *Run the benchmark job on push to `main` too*: rejected for this gate —
  same-host base/current comparison is a PR review signal. A post-merge push
  comparison against `before` would make intentionally accepted trade-offs
  fail after they were already merged.

---

## Decision 6 — New package's coverage floor

**Decision**: Add an explicit `internal/cmd/benchcheck` entry to
`covercheck`'s `defaultFloorConfig().perPackage` map (in
`internal/cmd/covercheck/main.go`), calibrated to `benchcheck`'s actual
measured coverage once its tests are written — matching the existing
`internal/cmd/doccover` entry's precedent (`45.0`) rather than leaving it to
fall through to the 25% default.

**Rationale**: `internal/cmd/doccover` — the most similar sibling tool — has
its own explicit, calibrated floor rather than relying on the generic
default; `benchcheck` follows the same precedent for consistency and so its
floor is a reviewable, intentional number from day one rather than an
accidental default.

**Alternatives considered**:

- *Rely on the 25% default floor*: rejected — every other `internal/cmd/*`
  tool has an explicit, calibrated entry; leaving `benchcheck` to the
  default would be an inconsistent, unexplained exception.

---

## Resolved Unknowns Summary

| Unknown | Resolution |
|---------|-----------|
| Statistical comparison engine | `golang.org/x/perf/benchstat`'s `Collection` API, despite its deprecation notice — Decision 1 |
| Regression budget values | >5% ns/op (significant only), >1 alloc/op absolute — Decision 2 (confirms `ROADMAP.md`'s existing numbers) |
| Baseline strategy | Same-host `BENCH_BASE_REF` worktree comparison, not static committed output — Decision 3 |
| New benchmark fixture shape | One representative command tree / one representative `*Error`, not multiple size variants — Decision 4 |
| CI job placement | Dedicated PR-only `bench` CI job running `make bench-check` with `BENCH_BASE_REF` set to the PR base SHA — Decision 5 |
| `benchcheck`'s coverage floor | Explicit calibrated entry in `covercheck`'s per-package map, mirroring `doccover`'s precedent — Decision 6 |
