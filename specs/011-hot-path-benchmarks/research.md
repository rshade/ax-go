# Phase 0 Research: Hot-Path Benchmarks with `-benchmem`

**Feature**: `011-hot-path-benchmarks` | **Date**: 2026-06-26

This document resolves the technical unknowns for the benchmark suite and
absorbs the governing ADR (ADR-0009) so it can be retired as the feature's
final task. There are no open `NEEDS CLARIFICATION` items.

---

## Decision 1 — Benchmark idiom: mirror `config_bench_test.go`

**Decision**: Write `logger_bench_test.go` in `package ax` using `b.Loop()`
(Go 1.24+) with table-driven sub-benchmarks via `b.Run(name, ...)`, and call
`b.ReportAllocs()` so allocation columns appear even when the suite is run
without the global `-benchmem` flag.

**Rationale**: The repository already has exactly one benchmark file,
`config_bench_test.go`, and it establishes the idiom: `package ax`, `for b.Loop()`,
table of named cases, a doc comment on each `BenchmarkXxx` stating what the
numbers substantiate. FR-011 requires consistency with existing conventions, so
the new suite copies this shape rather than inventing a second style.
`b.ReportAllocs()` makes the allocation profile self-reporting (SC-001: one
command, no extra flags strictly required), while the canonical invocation still
passes `-benchmem`.

**Alternatives considered**:

- *Pre-1.24 `for i := 0; i < b.N; i++`*: rejected — `b.Loop()` is the current
  idiom already in use here and avoids the classic "loop variable elided by the
  optimizer" footgun.
- *External `package ax_test`*: rejected — `config_bench_test.go` is `package ax`,
  and an internal package lets a future white-box variant reach the unexported
  `tracingHook`/`traceIDs` without exporting anything. Black-box measurement of
  the public `NewLogger` path is still the default; internal package does not
  force white-box use.

---

## Decision 2 — Variant matrix: separate the paths that allocate differently

**Decision**: Provide these benchmark variants, each reported separately:

| Variant (full benchmark ID) | What it measures | Spec link |
|-----------------------------|------------------|-----------|
| `BenchmarkLoggerEmit/enabled/no_fields` | `Info(ctx).Msg("...")` at an enabled level, no active span | FR-003, SC-001 |
| `BenchmarkLoggerFieldShapes/typed_fields` | `Info(ctx).Str(...).Int(...).Msg("...")` | FR-006, US3 |
| `BenchmarkLoggerFieldShapes/with_labels` | logger built with `WithLoggerLabels(...)`, then emit | FR-006, US3 |
| `BenchmarkLoggerEmit/disabled_level` | `Debug(ctx).Msg("...")` on an `InfoLevel` logger (filtered) | FR-005, SC-002 |
| `BenchmarkLoggerTracingHook/no_trace_context` | emit with `context.Background()` (zero IDs path) | FR-004, SC-002 |
| `BenchmarkLoggerTracingHook/active_trace_context` | emit with a populated `SpanContext` (hex-format path) | FR-004, SC-002 |

**Rationale**: The tracing hook (`logger.go:190`) runs on every line and calls
`traceIDs(ctx)` (`trace.go:38`). When no span is active it returns the constant
`ZeroTraceID`/`ZeroSpanID` (no allocation); when a span is active it calls
`sc.TraceID().String()` / `sc.SpanID().String()`, which format hex strings and
**allocate**. Reporting one blended number would mask exactly the allocation the
ADR claim concerns. The disabled-level path is zerolog's early-return fast path
and is expected to be the cheapest; conflating it with an emitted line hides a
meaningful difference (spec Edge Cases).

**Alternatives considered**:

- *Single "typical line" benchmark*: rejected — unrepresentative and hides the
  hook's context-dependent cost (the whole point of FR-004).
- *Benchmark the multi-sink (`io.MultiWriter`) fan-out*: deferred — spec scope is
  the primary single-writer emit path (spec Assumptions); the Loki sink already
  has its own tests. Out of scope unless trivially added.

---

## Decision 3 — Output sink: `io.Discard`

**Decision**: Construct every benchmarked logger with
`WithLoggerWriter(io.Discard)`.

**Rationale**: FR-007 requires the measured profile to reflect logger
allocations, not OS write syscalls or console formatting. `io.Discard` is the
zero-cost sink the existing `loki_test.go` already uses for the same reason
(`loki_test.go:237` and elsewhere). Writing to a `bytes.Buffer` would add
buffer-growth allocations that contaminate the profile; writing to `os.Stderr`
would add syscall cost and is non-deterministic across environments (violating
FR-008).

**Alternatives considered**:

- *`bytes.Buffer`*: rejected — its `grow` allocations distort `allocs/op`.
- *A counting writer*: unnecessary; throughput is `b.Loop()`'s job, not the
  sink's.

---

## Decision 4 — Active-span construction without a live SDK

**Decision**: Build the active-trace-context variant with
`oteltrace.ContextWithSpanContext(ctx, oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: tid, SpanID: sid}))`,
using fixed hex IDs decoded once outside the measured loop via
`oteltrace.TraceIDFromHex(...)` / `SpanIDFromHex(...)`.

**Rationale**: The tracing hook only needs `SpanContextFromContext(ctx)` to
report `HasTraceID()`/`HasSpanID()` true; it does not require a running tracer
provider or exporter. This keeps the benchmark deterministic and network-free
(FR-008/SC-005). This is the exact construction already used in `config_test.go:558`
and `json_test.go:44`, so it is a proven pattern in this repo. IDs are decoded
before `b.Loop()` so decode cost is excluded from the measured path.

**Alternatives considered**:

- *Start a real span via an SDK tracer*: rejected — pulls in provider setup,
  is heavier, and is non-deterministic; unnecessary for what the hook reads.

---

## Decision 5 — Reconciling the claim (FR-010): documented, evidence-led

**Decision**: After the suite runs, record the actual `allocs/op` and `B/op` for
each variant in this `research.md` (a results table appended during
implementation) and reflect the headline numbers in each benchmark's doc
comment. Then reconcile ADR-0009's "zero or near-zero allocation hot path"
claim: if the no-trace-context enabled path is at/near zero allocations, the
claim is CONFIRMED for that path and the active-context path's hex-format
allocations are documented as the expected, bounded exception ("near-zero" =
small, bounded, documented — spec Assumptions). If measurement contradicts the
claim, the claim text is REVISED to match the evidence. No numeric pass/fail gate
is added (spec Assumptions: existing `config_bench_test.go` is not gated either).

**Rationale**: The constitution (Principle VII) forbids asserting a performance
bar without a benchmark; the resolution is to let the benchmark drive the wording,
not to defend a pre-written number. Recording the numbers in `research.md` plus
doc comments satisfies FR-009/SC-004 (reproducible conditions) without a separate
doc system. The public claim currently lives only in ADR-0009 (being retired) and
in `README.md:167`; on retirement, `README.md` is the surviving home for any
revised wording.

**Alternatives considered**:

- *Add a CI allocation gate (fail build above N allocs/op)*: rejected for this
  feature — out of scope per spec Assumptions; a regression gate is a clean
  follow-up once a stable baseline exists. Documented here so the omission is
  intentional, not an oversight.

---

## Measured Allocation Profile (Results)

> Fulfils FR-009 / SC-004: the numbers and the conditions under which they were
> taken, recorded so they are discoverable without re-running the suite.

**Conditions of measurement**: `go test -run '^$' -bench '^BenchmarkLogger'
-benchmem -count=5 ./...`; Go 1.26.4, `linux/amd64`, Intel Core i5-14450HX
(`GOMAXPROCS=16`). Every variant writes to `io.Discard` (FR-007). `allocs/op` and
`B/op` are deterministic across machines (they depend on code path, not
hardware); `ns/op` is hardware-dependent and shown only for orientation.

| Benchmark ID | B/op | allocs/op | ns/op (approx.) | Path characterised |
|--------------|------|-----------|-----------------|--------------------|
| `BenchmarkLoggerEmit/enabled/no_fields` | 0 | 0 | ~122 | enabled emit, no active span (the headline hot path) |
| `BenchmarkLoggerEmit/disabled_level` | 0 | 0 | ~5 | filtered fast path (`Debug` on an `InfoLevel` logger) — cheapest |
| `BenchmarkLoggerTracingHook/no_trace_context` | 0 | 0 | ~122 | hook with `context.Background()` (zero-ID constant path) |
| `BenchmarkLoggerTracingHook/active_trace_context` | 48 | 2 | ~200 | hook with a populated `SpanContext` (hex-format path) |
| `BenchmarkLoggerFieldShapes/typed_fields` | 0 | 0 | ~149 | `.Str().Int()` typed payload fields |
| `BenchmarkLoggerFieldShapes/with_labels` | 0 | 0 | ~122 | logger built with low-cardinality labels |

**Confirmed hypotheses** (data-model "Expected relationships"):

- `disabled_level` (0 allocs, ~5 ns) ≤ every enabled variant. ✅
- `no_trace_context` (0 allocs) ≤ `active_trace_context` (2 allocs). ✅
- `typed_fields` ≥ `enabled/no_fields` in time (~149 vs ~122 ns), both at 0
  allocs — zerolog's typed field methods write into the event buffer without
  per-call heap allocation. ✅

**Reconciliation (FR-010, Decision 5)**: the ADR-0009 "zero or near-zero
allocation hot path" claim is **CONFIRMED**. The enabled emit path, the filtered
path, the no-trace-context hook path, the typed-fields path, and the
labelled-logger path all allocate **0 B/op and 0 allocs/op**. The single
allocating path is `active_trace_context` at **2 allocs/op (48 B/op)**, incurred
by `TraceID.String()` / `SpanID.String()` formatting hex into fresh strings — the
expected, bounded, documented "near-zero" exception, not a defect. No claim
wording needs to be weakened; `README.md` is updated to cite this evidence.

---

## Decision Records Absorbed

> Constitution §Governance / ADR-absorption gate. ADR-0009's decision,
> considered alternatives, and consequences are transcribed here so the ADR file
> can be deleted as this feature's final task. The ADR MUST NOT be deleted until
> this section exists (it now does).

### ADR-0009 — Structured Logger: ZeroLog (ACCEPTED 2026-05-28) — absorbed, file RETIRED

**Context**: Per Stream Separation, structured JSON logs go to `stderr`. ax-go
needs a fast, allocation-conscious logger that integrates with OTel trace
correlation (feature `004-real-otel-export`) and Loki cardinality discipline
(the label/payload split, now Constitution Principle VIII).

**Decision drivers**: (1) zero or near-zero allocations on the hot path;
(2) native JSON output with no extra serialization layer; (3) a hook system for
OTel `trace_id`/`span_id` injection on every line; (4) an idiomatic Go fluent
builder API; (5) type-level separation of label fields (Loki-indexed) from
payload fields (Loki-unindexed).

**Considered options**:

- **A. `github.com/rs/zerolog`** — fluent builder, zero-allocation hot path,
  native JSON, hook system. *Pros*: best-in-class allocation profile; hook system
  fits OTel correlation cleanly; mature; prior maintainer experience. *Cons*:
  external dependency; smaller community than zap; Marshal API differs from
  stdlib.
- **B. `uber-go/zap`** — *Pros*: fastest in some benchmarks; large user base.
  *Cons*: more complex API (Sugared vs. core split); newer zerolog releases close
  the gap; zerolog's fluent API reads more easily for humans and agents.
- **C. `log/slog` (stdlib)** — *Pros*: no third-party dep; standardized; otelslog
  bridge exists. *Cons*: still maturing as of 2026; perf close-but-not-equal on
  hot paths; handler interface less ergonomic for the field-typing discipline.
- **D. `sirupsen/logrus`** — reflection-heavy, effectively deprecated for new Go.

**Decision**: Adopt **Option A — `github.com/rs/zerolog`**. Expose
`ax.NewLogger(ctx)` as the canonical constructor returning an `ax.Logger`
(initially backed by zerolog) pre-wired with the OTel correlation hook
(`trace_id`/`span_id` on every line) and type-segregated label-vs-payload field
APIs. **One structured logger at a time** — no parallel-pluggable runtime
selection, no `ax.WithLogger(...)` hot-swap; the `ax.Logger` interface exists
solely as a migration seam so a future superseding decision (e.g. a slog-backed
implementation) can land without breaking consumer call sites.

**Consequences**:

- Direct dependency on `github.com/rs/zerolog`.
- Consumers MUST NOT construct `zerolog.Logger` directly; use `ax.NewLogger(ctx)`
  so the OTel hook and field typing are wired.
- **Bench-test allocation on hot paths via `testing.B` and `-benchmem`; do not
  assert numeric performance bars without a benchmark.** ← This is the last
  outstanding obligation of ADR-0009 and is precisely what feature `011`
  delivers; the present feature discharges it and is therefore the correct
  feature to retire the ADR.

**Where these decisions now live (post-retirement)**: the zerolog choice, the
single-backend guardrail, the trace-correlation hook, and the label/payload
cardinality split are Constitution **Principle VIII** (and Principle VI for the
"no pluggable backend" guardrail). The "no second logger backend / interface is a
migration seam only" guardrail is also stated in `AGENTS.md`. The allocation
claim's surviving home is `README.md` (revised per Decision 5 if measurement
requires). Nothing standing in ADR-0009 is lost by deletion.

**Retirement note** — `tasks.md`'s FINAL task deletes
`docs/adr/0009-logger-zerolog.md` and updates its references. Reference set
(verified 2026-06-26):

- `logger.go:30` — doc comment "Logger is the ADR-0009 logging surface…":
  redirect to Constitution §VIII (Go doc-comment edit; not an API change).
- `README.md:186` — ADR link in prose; `README.md:213` — ADR index table row:
  remove the row / redirect the link to Constitution §VIII per the established
  index-pruning pattern.
- `ROADMAP.md:82` — the "substantiate the claim (ADR-0009) with a real
  `testing.B`" line: mark done, drop the `(ADR-0009)` tag (this feature does it).
  `ROADMAP.md:213` — "zerolog logger + trace-correlation hook (ADR-0009) [M]":
  resolve the tag (already `[x]`). `ROADMAP.md:238` — "Add a pluggable logger
  backend while ADR-0009 stands": redirect to Constitution §VI.
- `CONTEXT.md:71` — "…logger, while ADR-0009 stands": redirect to Constitution
  §VI/§VIII.
- `AGENTS.md:110` — "…logger implementation while ADR-0009 stands": redirect to
  Constitution §VI.

**Historical spec-reference hygiene**: other features' point-in-time specs that
mention the structured-logger decision (002, 004, 008) are rewritten only enough
to avoid dead links or wording that assumes the retired ADR file still exists.
Their historical conclusions are preserved and redirected to this research file
where the decision is absorbed. Sibling-ADR cross-references inside ADR-0009
itself (e.g. lines pointing to `specs/004`) vanish with the file and need no
separate edit.

---

## Resolved Unknowns Summary

| Unknown | Resolution |
|---------|-----------|
| Benchmark style/placement | `package ax`, `b.Loop()`, table-driven, `logger_bench_test.go` (mirror `config_bench_test.go`) — Decision 1 |
| Which paths to measure | 6 variants separating enabled/disabled, fields/labels, no-span/active-span — Decision 2 |
| Sink choice | `io.Discard` — Decision 3 |
| Active span without SDK | `NewSpanContext` + `ContextWithSpanContext`, IDs decoded outside loop — Decision 4 |
| How the claim is reconciled & documented | Numbers in research.md + doc comments; claim confirmed-or-revised in README; no CI gate — Decision 5 |
| ADR-0009 absorption & retirement | Absorbed above; deleted by final task with enumerated reference set |
