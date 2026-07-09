# Implementation Plan: Performance Regression Budget in CI

**Branch**: `014-bench-regression-budget` | **Date**: 2026-07-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/014-bench-regression-budget/spec.md`

## Summary

Add a new `internal/cmd/benchcheck` tool that compares two `go test -bench=.
-benchmem` output files (a same-host base-ref run and a fresh current run) using
`golang.org/x/perf/benchstat`'s `Collection` API, and fails when any tracked
benchmark's operation time regresses by more than 5% (statistically
significant only) or its allocations per operation increase by more than 1.
Two new benchmarks (`BenchmarkBuildCommand` for the `__schema` reflection
path, `BenchmarkWriteError` for the error-envelope marshal path) fill the
gap so all four hot paths named in the issue are covered; the existing
logger and config-parse benchmarks already cover the other two. A
same-host `bench-check` Makefile target and a new PR-only CI job wire the
check into every PR. `AGENTS.md` gains a documented budget and base-ref
override procedure, mirroring the existing Coverage Policy section's shape.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/rshade/ax-go`)

**Primary Dependencies**: `golang.org/x/perf/benchstat` (NEW direct
dependency — statistical comparison of two benchmark result sets); Go
stdlib `flag`, `os`, `io`, `fmt` (CLI plumbing, mirroring
`internal/cmd/covercheck`); `spf13/cobra` (already a dependency; used only
to build the representative command tree in the new schema benchmark's test
fixture, not by `benchcheck` itself).

**Storage**: None. Benchmark output files are temporary per `make bench-check`
run; no committed benchmark output bakes in one machine's hardware.

**Testing**: `go test -run='^$' -bench=. -benchmem ./...` for benchmark
authoring; `go test ./internal/cmd/benchcheck/...` (table-driven, synthetic
multi-sample fixtures) for the checker tool itself, per AGENTS.md
Testing-First Discipline.

**Target Platform**: Linux/macOS/Windows developer machines and the
`ubuntu-latest` GitHub Actions runner. Base and current benchmarks run in
the same job to keep timing comparable even as runner hardware varies.

**Project Type**: Single Go library (CLI foundation) plus its internal
dev-tooling. New code: one new `internal/cmd/benchcheck` package, two new
test-only benchmark files, no change to the public `ax` API surface.

**Performance Goals**: This feature ASSERTS, unlike `specs/011-hot-path-benchmarks`
which only measured. Budget: >5% regression in ns/op (statistically
significant per `benchstat`'s default U-test at α=0.05) OR >1 additional
alloc/op fails CI. These are the exact numbers `ROADMAP.md` already commits
to for issue #22.

**Constraints**: The regression check MUST run base and current benchmarks on
the same host with the same fixed `-cpu=1` value (spec FR-005/FR-006), so
benchmark names and raw `ns/op` values are portable across machines.
Statistical significance is required before failing (spec FR-003) so
single-run noise cannot fail a PR.

**Scale/Scope**: One new `internal/cmd` package (~150-200 lines, sized like
`covercheck`), two new benchmark files, one `Makefile` target, one new CI
job, one new `AGENTS.md` section, and one `covercheck` floor-map entry for
the new package.

**Governing ADR(s)**: N/A.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Relevance | Verdict |
|-----------|-----------|---------|
| I. Stream Separation | `benchcheck` writes its pass summary to stdout and violation list to stderr, matching `covercheck`'s existing contract exactly. | ✅ PASS |
| II. Deterministic Output & Exit Codes | New exit-code contract for a dev tool (0/1/2), not the `ax.Error`/CLI runtime envelope — no conflict with the constitution's runtime exit-code mapping (0/1/2/3/4), which governs CLI *user* commands, not internal build tooling (same category as `covercheck`/`doccover`). | ✅ PASS |
| III. `__schema` Discoverability | No command-tree or schema change. The new `BenchmarkBuildCommand` *measures* the reflection path; it adds no new command. | ✅ PASS (N/A) |
| IV. Agent-Safety Primitives | No CLI surface change to ax-go itself. | ✅ PASS (N/A) |
| V. Asymmetric JSON I/O | No config read/write change. | ✅ PASS (N/A) |
| VI. ADR-Governed Scope — Library, Not Application | Adds no exported `ax` surface; `benchcheck` is `internal/`, same category as `covercheck`/`doccover`/`apidiff-verdict`. | ✅ PASS |
| VII. Test-First Discipline (NON-NEGOTIABLE) | `benchcheck`'s `run()` gets table-driven tests before/alongside the CI wiring lands (synthetic multi-sample fixtures, since research confirmed single-sample fixtures never register as statistically significant). The two new benchmarks themselves fulfill "`testing.B` with `-benchmem` for any hot-path claim." No new exported `ax` symbol → no new `ExampleXxx`/doc-coverage obligation, but `benchcheck`'s own exported types (`Violation`, `CheckResult`-equivalent) still need doc comments per `godoclint`. | ✅ PASS |
| VIII. Observability & ID Discipline | No new trace/resource ID behavior; `benchcheck` is a sub-second CLI tool with no tracing surface (same recorded deviation as `covercheck`). | ✅ PASS |
| IX. Security & Resource Safety | Benchmark input files are read with a bounded size cap (mirroring `covercheck`'s `maxProfileBytes` pattern) so a malformed/oversized file is a validation error (exit 2), not unbounded memory growth. No PII, no `panic`, no network, no TLS. | ✅ PASS |
| X. Idiomatic Go & Dependency Minimalism | **New dependency**: `golang.org/x/perf/benchstat`. Justified: it is the same statistical-comparison engine the upstream `benchstat` CLI (which the issue names explicitly) uses internally; hand-rolling Welch/Mann-Whitney significance testing would be strictly worse (more code, less battle-tested) than importing it. Deprecation note: `benchstat` (the library subpackage) points to `benchproc`/`benchmath` as its replacement, but those are lower-level primitives requiring materially more integration code for the same outcome — recorded here as a deliberate, revisitable trade-off, not an oversight, per `research.md`. `benchcheck` intentionally omits `context.Context` (sub-second, no I/O beyond two bounded file reads — same recorded deviation as `covercheck`). No new mutable package-level state. | ✅ PASS (dependency justified) |
| XI. Stability & SemVer | `internal/cmd/benchcheck` is `internal/` — EXEMPT (toolchain blocks external import, no consumer to break). No change to the public `ax` API surface, `ax.Error`, or `__schema` shape. Commit type: `feat:` (new CI capability) or `chore:`/`ci:` depending on how the repo classifies tooling-only changes; not a breaking change either way. | ✅ PASS (no bump implication) |
| XII. Deprecation Lifecycle | No exported symbol deprecated or removed. (The *dependency*, not this repo's code, carries a deprecation notice — addressed under Principle X, not this principle.) | ✅ PASS (N/A) |

**ADR absorption gate (Constitution §Governance)**: N/A. "Governing ADR(s)" is
N/A; no absorption or retirement task is required.

**Initial gate result**: PASS. One dependency addition justified (Principle
X); no other violations. Complexity Tracking left empty.

**Post-Phase-1 re-check**: PASS — Phase 1 design (below) introduces no
additional dependency, no exported `ax` surface change, and no scope creep
beyond what Phase 0 already justified.

## Project Structure

### Documentation (this feature)

```text
specs/014-bench-regression-budget/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output; records the benchstat dependency trade-off
├── data-model.md        # Phase 1 output (comparison inputs, Regression Budget, Tracked Benchmark entities)
├── quickstart.md        # Phase 1 output (how to run bench-check and read a failure)
├── contracts/
│   └── benchcheck-output.md  # Phase 1 output — exit codes + stdout/stderr examples, mirrors covercheck-output.md
├── checklists/
│   └── requirements.md  # Spec quality checklist (already authored by /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
github.com/rshade/ax-go/
├── internal/cmd/covercheck/main.go       # Pattern to mirror (read-only reference)
├── internal/cmd/benchcheck/
│   ├── main.go                           # NEW — the entire tool implementation
│   └── main_test.go                      # NEW — table-driven tests, synthetic fixtures
├── internal/schema/
│   ├── schema.go                         # BuildCommand under test — not modified
│   └── schema_bench_test.go              # NEW — BenchmarkBuildCommand
├── contract/
│   ├── error.go                          # WriteError under test — not modified
│   └── error_bench_test.go               # NEW — BenchmarkWriteError
├── Makefile                              # EDIT — bench-check target; bench-check added to `ci`/`help`
├── .github/workflows/ci.yml              # EDIT — new PR-only `bench` job
├── AGENTS.md                             # EDIT — new "Performance Regression Budget" section after Coverage Policy
├── ROADMAP.md                            # EDIT — #22 marked done; Immediate Focus pointer updated
└── go.mod / go.sum                       # EDIT — golang.org/x/perf added as a direct dependency
```

**Structure Decision**: Single-project Go library; the new tool lives at
`internal/cmd/benchcheck`, matching the existing `internal/cmd/{covercheck,
doccover,apidiff-verdict}` sibling pattern exactly (own subdirectory, own
`main.go`, own tests, `package main`). The two new benchmarks are test-only
files added to the packages whose hot paths they measure (`internal/schema`,
`contract`), matching how the existing `config_bench_test.go`/
`logger_bench_test.go` live alongside the code they benchmark. No new
top-level package or directory beyond `internal/cmd/benchcheck`.

## Complexity Tracking

> No unjustified Constitution Check violations. The one flagged item
> (new dependency, Principle X) is justified above with a documented,
> revisitable trade-off — not a violation requiring a simpler-alternative
> comparison.
