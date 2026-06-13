# ADR-0009: Structured Logger — ZeroLog

## Status

ACCEPTED — 2026-05-28.

## Context

Per the Golden Rule, structured JSON logs go to `stderr`. ax-go needs
a fast, allocation-conscious logger that integrates with OTel trace
correlation (delivered by
[`specs/004-real-otel-export`](../../specs/004-real-otel-export/)) and Loki
cardinality discipline (ADR-0006).

## Decision Drivers

- Zero or near-zero allocations on the hot path.
- Native JSON output (no extra serialization layer).
- Hook system for OTel trace_id/span_id injection on every line.
- Idiomatic Go fluent builder API.
- Type-level separation of "label fields" (Loki-indexed) from
  "payload fields" (Loki-unindexed) — see ADR-0006.

## Considered Options

### A. `rs/zerolog`

Fluent builder, zero-allocation hot path, native JSON, hook system.

Pros: best-in-class allocation profile; hook system fits the OTel
correlation pattern cleanly; mature; the user has prior experience.
Cons: external dependency; smaller community than zap; Marshal API
differs from stdlib patterns.

### B. `uber-go/zap`

Long-standing Go logging library.

Pros: fastest in some benchmarks; large user base.
Cons: more complex API (SugaredLogger vs Logger split); newer
zerolog releases close the perf gap; agents and humans both find
zerolog's fluent API easier to read.

### C. `log/slog` (Go 1.21+ stdlib)

Standard library structured logger.

Pros: no third-party dependency; standardized; otelslog bridge
exists; growing handler ecosystem.
Cons: still maturing as of 2026; performance is close-but-not-equal
to zerolog/zap on hot paths; handler interface is less ergonomic for
the field-typing discipline ADR-0006 requires.

### D. `sirupsen/logrus`

Reflection-heavy; effectively deprecated for new Go code.

## Decision

Adopt **Option A** — `github.com/rs/zerolog`.

The ax base exposes `ax.NewLogger(ctx)` as the canonical constructor,
returning an `ax.Logger` (initially backed by `*zerolog.Logger`)
pre-wired with:

- The OTel correlation hook documented in
  [`specs/004-real-otel-export`](../../specs/004-real-otel-export/)
  injecting `trace_id` and `span_id` on every line.
- Type-segregated label vs. payload field APIs supporting ADR-0006's
  Loki cardinality rule.

**One structured logger at a time.** slog and zap remain valid
alternatives but ax-go ships only ONE concrete logger backend at a
time — no parallel-pluggable runtime selection, no
`ax.WithLogger(slog)`-style hot-swap API. The `ax.Logger` interface
exists to make a future migration clean, NOT to host concurrent
implementations: the handler interface is similar enough across
zerolog / slog / zap that a slog-backed `ax.Logger` could be
introduced via a superseding ADR (`0009-v2`) without breaking consumer
call sites.

## Consequences

- Direct dependency on `github.com/rs/zerolog`.
- Consumers should NOT construct `zerolog.Logger` directly; use
  `ax.NewLogger(ctx)` so the OTel hook and field-typing are wired.
- Bench-test allocation on hot paths via `testing.B` and `-benchmem`;
  do not assert numeric performance bars without a benchmark.
