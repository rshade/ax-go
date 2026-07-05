# Implementation Plan: Hot-Path Benchmarks with `-benchmem`

**Branch**: `011-hot-path-benchmarks` | **Date**: 2026-06-26 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/011-hot-path-benchmarks/spec.md`

## Summary

Add a `BenchmarkLogger*` suite (`testing.B`, run with `-benchmem`) that measures
the logger emit hot path — including the always-on tracing hook — across the
paths that have genuinely different allocation behavior: enabled-level emit,
disabled-level (filtered) fast path, emit with no active trace context, emit
with an active trace context, a typed-payload-field variant, and a
labels-configured variant. Output is directed to `io.Discard` so sink write cost
does not distort the allocation profile. The measured numbers are recorded in
`research.md` (and reflected in per-benchmark doc comments), and the
unsubstantiated "zero or near-zero allocation hot path" claim from ADR-0009 is
reconciled with the evidence — confirmed or revised. Because ADR-0009 governs
this feature and its last outstanding obligation (the benchmark) is satisfied
here, ADR-0009 is absorbed into `research.md` and retired as the final task.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/rshade/ax-go`, package `ax`)

**Primary Dependencies**: `github.com/rs/zerolog` (logger under test);
`go.opentelemetry.io/otel/trace` (span-context construction for the
active-context variant); Go stdlib `testing` (`testing.B`, `b.Loop()`, `b.ReportAllocs()`),
`io` (`io.Discard`). No new dependency is introduced.

**Storage**: N/A — benchmarks are in-memory, output discarded.

**Testing**: `go test -run '^$' -bench '^BenchmarkLogger' -benchmem ./...`; the
suite uses `b.Loop()` (Go 1.24+) and table-driven sub-benchmarks, mirroring the
existing `config_bench_test.go`.

**Target Platform**: Linux/macOS/Windows developer machines and CI runners
(any platform the existing test suite runs on).

**Project Type**: Single Go library (CLI foundation). New code is a single
test-only file `logger_bench_test.go` at the module root, package `ax`.

**Performance Goals**: This feature MEASURES rather than asserts. No numeric
allocation target is gated; the deliverable is a documented per-operation
allocation profile and a reconciled claim (spec SC-003/SC-004). The benchmark
is the evidence; the claim follows the evidence.

**Constraints**: Deterministic and self-contained — no network, no external
services, no environment-specific configuration (spec FR-008/SC-005). Output
MUST go to a discard sink so OS write cost does not dominate (spec FR-007).

**Scale/Scope**: One new test file; ~6 benchmark variants; no exported API
change; no `__schema`/`ax.Error` payload change. ADR-0009 retirement touches a
fixed reference set (see research "Retirement note").

**Governing ADR(s)**: `docs/adr/0009-logger-zerolog.md` — its decision,
considered alternatives, and consequences are absorbed into `research.md`
(§Decision Records Absorbed) in Phase 0; it is deleted by `tasks.md`'s FINAL
task with all references redirected.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Relevance | Verdict |
|-----------|-----------|---------|
| I. Stream Separation | Test-only code; benchmark output is discarded, nothing reaches process `stdout`. The discard-sink requirement (FR-007) reinforces it. | ✅ PASS |
| II. Deterministic Output & Exit Codes | No runtime payload, no exit-code path touched. Benchmark is deterministic (fixed inputs). | ✅ PASS (N/A to payload) |
| III. `__schema` Discoverability | No command-tree or schema change. | ✅ PASS (N/A) |
| IV. Agent-Safety Primitives | No CLI surface change. | ✅ PASS (N/A) |
| V. Asymmetric JSON I/O | No config read/write change. | ✅ PASS (N/A) |
| VI. ADR-Governed Scope — Library, Not Application | Adds no exported surface, no second logger backend, no pluggable selection. Pure measurement of the existing logger. | ✅ PASS |
| VII. Test-First Discipline (NON-NEGOTIABLE) | This feature DIRECTLY fulfills the mandate "`testing.B` with `-benchmem` for any allocation/performance claim." Benchmark variants land before the claim is reconciled. No new exported symbol → no new `ExampleXxx`/doc-coverage obligation; doc-comment presence rule does not apply to test functions but each variant carries a contract doc comment stating what it substantiates. | ✅ PASS (this IS the principle) |
| VIII. Observability & ID Discipline | Exercises the existing trace-correlation hook; introduces no new ID behavior. | ✅ PASS |
| IX. Security & Resource Safety | No PII/secret logging; no `panic`; no TLS; no unbounded input. Benchmark messages are static literals. | ✅ PASS |
| X. Idiomatic Go & Dependency Minimalism | No new dependency; `context.Context` first param honored by the API under test; no package-level mutable state. | ✅ PASS |
| XI. Stability & SemVer | No change to the public Go surface or to `ax.Error`/`__schema` shapes → not a breaking change; `go-apidiff` is unaffected. ADR file deletion + doc-comment text edits are not API changes. Commit type: `test:` / `chore:` (plus `docs:` for retirement edits). | ✅ PASS (no bump implication) |
| XII. Deprecation Lifecycle | No symbol deprecated or removed. | ✅ PASS (N/A) |

**ADR absorption gate (Constitution §Governance)**: TRIGGERED. "Governing ADR(s)"
is `docs/adr/0009-logger-zerolog.md` (not N/A). Therefore `research.md` MUST
contain a "Decision Records Absorbed" section recording ADR-0009's decision,
considered alternatives, and consequences (✅ authored in Phase 0 below), AND
`tasks.md` MUST include the final ADR-retirement task (specified here for
`/speckit-tasks` to emit; not created by `/speckit-plan`). The ADR file MUST NOT
be deleted until its decisions are absorbed into `research.md` — sequencing
enforced by making retirement the LAST task.

**Initial gate result**: PASS. No violations; Complexity Tracking left empty.

**Post-Phase-1 re-check**: PASS — no design artifact introduced exported surface,
new dependency, or scope creep. Verdict unchanged.

## Project Structure

### Documentation (this feature)

```text
specs/011-hot-path-benchmarks/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output; holds "Decision Records Absorbed" for ADR-0009
├── data-model.md        # Phase 1 output (conceptual entities: benchmark variant, allocation profile)
├── quickstart.md        # Phase 1 output (how to run the suite + read the numbers)
├── checklists/
│   └── requirements.md  # Spec quality checklist (already authored by /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

No `contracts/` directory: this feature exposes **no external interface**. The
benchmark is a test-only artifact (`*_test.go`) with no exported identifier,
no machine payload, and no CLI surface. The only "contract" is the benchmark
naming convention (`BenchmarkLogger*`) and the documented allocation profile,
both captured in `data-model.md` and `quickstart.md`. Per the plan workflow's
contracts rule, a contracts directory is skipped for purely internal artifacts.

### Source Code (repository root)

```text
github.com/rshade/ax-go/
├── logger.go                 # Logger under test (NewLogger, tracingHook) — doc comment at :30 edited on retirement
├── trace.go                  # traceIDs() — exercised by the hook; not modified
├── logger_test.go            # Existing logger unit tests — not modified
├── config_bench_test.go      # Existing benchmark — the idiom to mirror
├── logger_bench_test.go      # NEW — BenchmarkLogger* suite (the entire implementation)
├── docs/adr/0009-logger-zerolog.md   # DELETED by final retirement task
├── README.md                 # ADR index row + link redirected on retirement (:186, :213)
├── ROADMAP.md                # benchmark line marked done; ADR tags resolved (:82, :213, :238)
├── AGENTS.md                 # "while ADR-0009 stands" → constitution §VI (:110)
└── CONTEXT.md                # "while ADR-0009 stands" → constitution §VI/VIII (:71)
```

**Structure Decision**: Single-project Go library; root package `ax`. The whole
implementation is one new test file, `logger_bench_test.go`, in package `ax`
(internal benchmark so it can reach the unexported `tracingHook`/`traceIDs` if a
white-box variant is wanted, and to match `config_bench_test.go` which is
`package ax`). Retirement of ADR-0009 edits a fixed, enumerated set of existing
files; no new packages or directories are introduced.

## Complexity Tracking

> No Constitution Check violations. Table intentionally empty.
