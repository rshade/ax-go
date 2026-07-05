# Quickstart: Hot-Path Benchmarks with `-benchmem`

**Feature**: `011-hot-path-benchmarks` | **Date**: 2026-06-26

How to run the logger benchmark suite and read the allocation profile. This is
the verification path for spec SC-001…SC-005.

## Run the whole logger suite

```bash
go test -run '^$' -bench '^BenchmarkLogger' -benchmem ./...
```

- `-run '^$'` disables normal tests (run benchmarks only).
- `-bench '^BenchmarkLogger'` selects this feature's suite by its naming prefix
  (FR-001).
- `-benchmem` adds the `B/op` and `allocs/op` columns (FR-002). The suite also
  calls `b.ReportAllocs()`, so those columns appear even if `-benchmem` is
  omitted.

## Reading the output

```text
BenchmarkLoggerEmit/enabled/no_fields-8                  N    XX ns/op    Y B/op    Z allocs/op
BenchmarkLoggerEmit/disabled_level-8                     N    ...         ...       ...
BenchmarkLoggerTracingHook/no_trace_context-8            N    ...         ...       ...
BenchmarkLoggerTracingHook/active_trace_context-8        N    ...         ...       ...
BenchmarkLoggerFieldShapes/typed_fields-8                N    ...         ...       ...
BenchmarkLoggerFieldShapes/with_labels-8                 N    ...         ...       ...
```

The suite is split across three functions grouped by user story —
`BenchmarkLoggerEmit` (enabled vs. filtered), `BenchmarkLoggerTracingHook`
(trace-context cost), and `BenchmarkLoggerFieldShapes` (field/label shapes) —
all matched by the `^BenchmarkLogger` prefix above.

The two columns that substantiate the ADR-0009 claim are **`B/op`** and
**`allocs/op`**. Read them per variant — do not average across variants
(SC-002), because:

- `BenchmarkLoggerEmit/disabled_level` is zerolog's filtered fast path
  (expected cheapest).
- `BenchmarkLoggerTracingHook/no_trace_context` takes the zero-ID constant path
  (expected no/low alloc).
- `BenchmarkLoggerTracingHook/active_trace_context` formats hex trace/span IDs
  (expected to allocate — this is the documented "near-zero" exception, not a
  defect).

## Focus a single variant

```bash
go test -run '^$' -bench 'BenchmarkLoggerTracingHook/active_trace_context' -benchmem -count=5 ./...
```

`-count=5` repeats the measurement so you can eyeball variance before recording a
number. Record the result (variant + field shape + trace-context state) in
`research.md`'s results table (SC-004).

## Acceptance verification map

| Spec criterion | How this quickstart verifies it |
|----------------|---------------------------------|
| SC-001 (one-command allocation numbers) | the single `go test -bench` command above prints `B/op`/`allocs/op` with no extra setup |
| SC-002 (distinct profiles for ≥4 paths) | the six-row output across the three `BenchmarkLogger*` functions shows enabled, disabled, no-context, and active-context separately |
| SC-003 (claim reconciled) | compare `BenchmarkLoggerTracingHook/no_trace_context` allocs to the ADR-0009 "near-zero" claim; confirm or revise wording in `README.md` |
| SC-004 (reproducible conditions) | each row's name encodes its conditions; numbers + conditions recorded in `research.md` |
| SC-005 (no network / runs in CI) | the command needs no env vars, services, or network; runs anywhere `go test` runs |

## Full local gate (before handing work back)

```bash
gofmt -l .            # must print nothing
go vet ./...
golangci-lint run
go test -race ./...   # the benchmark file must compile & its tests/benchmarks build under -race
make doc-coverage     # unaffected (no new exported symbol), must stay clean
markdownlint-cli2 "specs/011-hot-path-benchmarks/**/*.md"
```
