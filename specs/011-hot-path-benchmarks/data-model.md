# Phase 1 Data Model: Hot-Path Benchmarks with `-benchmem`

**Feature**: `011-hot-path-benchmarks` | **Date**: 2026-06-26

This feature introduces **no runtime data types and no exported identifiers**.
The "entities" below are conceptual — they describe the structure of the
benchmark suite and the shape of the measurement it produces. They map the
spec's Key Entities to concrete test constructs so `/speckit-tasks` can decompose
the work.

---

## Entity: Benchmark Variant

A single, independently-reported measurement of one logging path. Realized as one
row in a table-driven `b.Run(name, ...)` sub-benchmark.

| Attribute | Meaning | Values for this feature |
|-----------|---------|-------------------------|
| `name` | Full benchmark ID — `BenchmarkLogger<Group>/<case>`, grouped by user story into three functions | `BenchmarkLoggerEmit/enabled/no_fields`, `BenchmarkLoggerEmit/disabled_level`, `BenchmarkLoggerTracingHook/no_trace_context`, `BenchmarkLoggerTracingHook/active_trace_context`, `BenchmarkLoggerFieldShapes/typed_fields`, `BenchmarkLoggerFieldShapes/with_labels` |
| `level` | Configured minimum level of the logger | `InfoLevel` for all; `disabled_level` emits at `Debug` to exercise the filtered path |
| `ctx` | Context passed to the emit call | `context.Background()` (zero IDs) for most; a populated `SpanContext` for `hook/active_trace_context` |
| `labels` | Labels applied at construction | empty except `enabled/with_labels` (`Application`, `Environment`) |
| `fields` | Typed payload fields attached before `.Msg` | none except `enabled/typed_fields` (`.Str`, `.Int`) |
| `sink` | Output destination | `io.Discard` for all (FR-007) |

**Validation / invariants**:

- Every variant constructs its logger with `WithLoggerWriter(io.Discard)`
  (FR-007).
- Per-variant setup that allocates (decoding span IDs, building the context,
  constructing the logger) happens **outside** `b.Loop()` so it is excluded from
  the measured profile (Decision 4).
- The measured body is exactly one logical emit (`<level>(ctx)[.fields].Msg(...)`)
  so `allocs/op` is per-line.
- `BenchmarkLoggerEmit/disabled_level` and
  `BenchmarkLoggerTracingHook/active_trace_context` MUST exist and be reported
  separately (SC-002).

**State transitions**: none — benchmarks are stateless and idempotent.

---

## Entity: Allocation Profile

The measured result for one Benchmark Variant — the output Go's testing
framework emits per benchmark line.

| Field | Source | Meaning |
|-------|--------|---------|
| ops | `b.Loop()` iteration count | iterations executed (throughput basis) |
| ns/op | testing framework | wall-clock nanoseconds per emit |
| B/op | `-benchmem` / `b.ReportAllocs()` | bytes allocated per emit |
| allocs/op | `-benchmem` / `b.ReportAllocs()` | heap allocations per emit |

**Validation / invariants**:

- `B/op` and `allocs/op` MUST be present in output (FR-002) — guaranteed by
  `b.ReportAllocs()` and/or the `-benchmem` flag.
- The recorded profile in `research.md` MUST state the variant, field shape, and
  trace-context state under which it was taken (SC-004) so it is reproducible.

**Expected relationships** (hypotheses the measurement will confirm or revise —
Decision 5, not asserted as gates):

- `BenchmarkLoggerEmit/disabled_level` allocs/op ≤ every enabled variant
  (early-return fast path).
- `BenchmarkLoggerTracingHook/no_trace_context` allocs/op ≤
  `BenchmarkLoggerTracingHook/active_trace_context` allocs/op (the active path
  formats hex IDs, which allocate).
- `BenchmarkLoggerFieldShapes/typed_fields` ≥ `BenchmarkLoggerEmit/enabled/no_fields`
  (more fields → more work).

---

## Entity: Logging Hot Path (system under test — unchanged)

The existing per-line emit operation being measured; defined in `logger.go`, not
modified by this feature except for one doc-comment redirect on ADR retirement.

| Step | Code | Allocation relevance |
|------|------|----------------------|
| acquire event at level | `zerologLogger.Info/Debug/...(ctx)` (`logger.go:124-138`) | filtered levels short-circuit (cheap) |
| attach typed fields | `*zerolog.Event.Str/.Int/...` | field-shape dependent |
| tracing hook fires | `tracingHook.Run` (`logger.go:190`) | always runs; calls `traceIDs(ctx)` |
| resolve IDs | `traceIDs` (`trace.go:38`) | zero IDs → constants (no alloc); active span → `.String()` hex (allocates) |
| finalize | `.Msg("...")` → write to sink | sink is `io.Discard` (no write cost) |

---

## Summary

- **New runtime types**: none.
- **New exported identifiers**: none → no `go-apidiff` impact, no SemVer bump.
- **New test artifact**: `logger_bench_test.go` (`package ax`) containing the
  `BenchmarkLogger*` suite encoding the six Benchmark Variants above.
- **Produced artifact**: an Allocation Profile per variant, recorded in
  `research.md` and reflected in benchmark doc comments.
