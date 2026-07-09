# Contract: benchcheck Output and Exit Codes

**Tool**: `go run ./internal/cmd/benchcheck -baseline <base-file> -current <current-file>`
**Contract version**: 1.0 (pre-v1.0 module; governed by Constitution Principle XI)

---

## Exit Codes

| Code | Meaning | When |
|------|---------|------|
| `0` | Within budget | No tracked benchmark exceeds the ns/op or allocs/op regression budget |
| `1` | Budget violation | One or more benchmarks regressed beyond the ns/op or allocs/op budget |
| `2` | Bad input | `-baseline` or `-current` is omitted, or the named file is not found, unreadable, oversized, or malformed `go test -bench` output |

---

## Standard Output (exit 0 — pass)

Written to **stdout**. One line per checked benchmark plus a summary.

```text
PASS  performance budget met (8 benchmarks checked)
  BenchmarkLoggerEmit/enabled/no_fields:               within budget
  BenchmarkLoggerEmit/disabled_level:                  within budget
  BenchmarkLoggerTracingHook/no_trace_context:         within budget
  BenchmarkLoggerTracingHook/active_trace_context:     within budget
  BenchmarkLoggerFieldShapes/typed_fields:             within budget
  BenchmarkLoggerFieldShapes/with_labels:               within budget
  BenchmarkBuildCommand:                                within budget
  BenchmarkWriteError:                                  within budget
```

---

## Standard Error (exit 1 — violation)

Written to **stderr**. The first line names the failure count; subsequent
lines identify each failing benchmark, sorted alphabetically by benchmark
name (violations further sorted by metric within a benchmark). Two kinds of
failure line can appear:

- **Budget violation** — the benchmark ran in both baseline and current, and
  a tracked metric regressed beyond budget. Identifies the metric, the
  measured delta, and the budget it exceeded.
- **Missing benchmark** — the benchmark is present in the baseline but
  absent from the current run entirely (e.g. it was renamed/deleted, its
  package failed to build, or it panicked). This is always a hard failure,
  distinct from a regression: benchstat's comparison silently drops any
  benchmark missing from either side, so a previously-tracked benchmark that
  stops running would otherwise vanish from the check without a trace.

```text
FAIL  3 performance budget failure(s):
  BenchmarkBuildCommand:     allocs/op  +3 > 1 budget
  BenchmarkLoggerEmit/enabled/no_fields:  missing from current run (tracked in baseline; check for a build failure or panic)
  BenchmarkWriteError:       ns/op  +12.4% > 5.0% budget
```

---

## Standard Error (exit 2 — bad input)

Written to **stderr**.

```text
benchcheck: -baseline is required
```

---

## Guarantees

- **Determinism**: Given the same baseline and current benchmark files, the
  tool produces byte-identical output on every run — the comparison is a pure
  function of the two input files, no timestamps or random values.
- **Noise tolerance**: A `ns/op` delta only counts as a violation when
  `golang.org/x/perf/benchstat` marks it statistically significant (α=0.05);
  measurement noise alone never produces exit `1` (spec FR-003).
- **Missing-benchmark detection**: a benchmark present in `-baseline` but
  absent from `-current` always produces exit `1`, distinct from a budget
  violation. This is the mirror image of the tolerance for a *new*
  benchmark not yet baselined (which is silently absent from the check, not
  a failure) — the reverse direction, a previously-tracked benchmark that
  disappears, is never silently tolerated.
- **Stream discipline**: Violations go to stderr; the pass summary goes to
  stdout. Nothing else is written to stdout (Constitution Principle I).
- **No side effects**: The tool reads two files and exits; it does not modify
  any file, regenerate benchmark output, or make network calls. Capturing the
  base/current benchmark files is the Makefile's responsibility.
- **Additive tolerance**: Future versions may add lines to the output (e.g.,
  additional per-benchmark detail) without bumping the contract version.
  Consumers MUST tolerate unknown lines.

---

## Invocation

```bash
# Minimal (explicit paths)
go run ./internal/cmd/benchcheck -baseline /tmp/bench-base.txt -current /tmp/bench-current.txt

# Via Makefile (captures base and current with -cpu=1, then checks them)
make bench-check
```

---

## Stability

This contract is internal (`internal/cmd/benchcheck`) and governed by
Constitution Principle XI. The exit-code contract (0/1/2) and the
human-readable output format are considered stable-by-intent once the
feature ships; format changes that would break CI scripts require a note in
the commit message.
