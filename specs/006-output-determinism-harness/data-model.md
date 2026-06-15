# Data Model: Output-Determinism Test Harness

**Feature branch**: `006-output-determinism-harness`
**Date**: 2026-06-14

---

## Entities

### `OutputMode` (enum)

Declared in `internal/testutil/determinism.go`.

| Constant | Value | Meaning |
|----------|-------|---------|
| `ModeBoundedJSON` | `0` | Single bounded JSON object on `stdout`. Used for the integration example's default success command. |
| `ModeNDJSON` | `1` | One JSON object per line on `stdout`. Used for the integration example's `stream` subcommand. |

---

### `RunResult`

Declared in `internal/testutil/determinism.go`.

Captures the raw output of a single command invocation.

| Field | Type | Description |
|-------|------|-------------|
| `Stdout` | `[]byte` | Raw bytes written to stdout by the command. |
| `Stderr` | `[]byte` | Raw bytes written to stderr. Held for diagnostic output only; not compared. |
| `ExitCode` | `int` | Process exit code returned by the command. |

**Invariants**:
- A `RunResult` with `ExitCode != 0` or `len(Stdout) == 0` is an execution failure; the harness reports it immediately and does not proceed to comparison (FR-009).

---

### `MaskedOutput`

Derived from a `RunResult`; never stored independently — produced inline during comparison.

| Concept | Description |
|---------|-------------|
| Source | `RunResult.Stdout` bytes |
| Transform | Regex replace for the three non-deterministic fields: `trace_id`, `span_id`, `idempotency_key`. Each matching `"field":"<value>"` is replaced with `"field":"MASKED"`. |
| Result | Byte slice ready for comparison. For `ModeBoundedJSON`: one byte slice. For `ModeNDJSON`: one byte slice per NDJSON line (after trimming trailing blank lines). |

**Sentinel constant** (declared in `internal/testutil/determinism.go`):

```go
const MaskedSentinel = "MASKED"
```

---

### `CompareReport`

Produced by a comparison failure; surfaced via `t.Errorf` and optionally `t.Logf`. Not a standalone struct — the report is emitted inline via `testing.TB`.

| Failure mode | Report content |
|--------------|----------------|
| Execution failure (non-zero exit or empty stdout) | Run index (1 or 2), exit code, stderr dump. |
| Line count mismatch (NDJSON) | Run 1 line count, Run 2 line count. |
| Content mismatch (bounded JSON) | Byte offset of first divergence, excerpt showing differing bytes. |
| Content mismatch (NDJSON) | First diverging line index, run 1 line content, run 2 line content. |

---

### `DeterminismCheck` (caller-side concept, not a struct)

Not a concrete Go type — a conceptual entity representing one complete harness execution. It is parameterized by:

| Parameter | Description |
|-----------|-------------|
| Command invoker | A function `func(stdout, stderr io.Writer) int` that calls `run()` with fixed args and returns exit code. |
| Output mode | `OutputMode` value (bounded or NDJSON). |
| `testing.TB` | Test context for reporting failures. |

The check executes the invoker twice, captures both `RunResult`s, validates each, masks both, and compares. If either validation or comparison fails, the check calls `t.Errorf` with a structured report.

---

### `TimestampScan` (internal operation)

Scans a `[]byte` JSON payload for any string value that parses as RFC 3339 UTC. Not a struct; implemented as a recursive walk over `map[string]any` produced by `json.Unmarshal`.

| Step | Description |
|------|-------------|
| 1. Unmarshal | `json.Unmarshal(data, &map[string]any{})` |
| 2. Walk | Recursively traverse all string values. |
| 3. Attempt parse | `time.Parse(time.RFC3339, v)` for each string. |
| 4. UTC check | If parse succeeds, verify `t.Location()` is UTC (zone offset zero). |
| 5. Report | If any string value parses as RFC 3339 but is NOT UTC, call `t.Errorf`. |

---

## Field Masking Patterns

Three compiled `regexp.MustCompile` patterns; compiled once at package init.

| Field | Pattern | Replacement |
|-------|---------|-------------|
| `trace_id` | `"trace_id":"([^"]*)"` | `"trace_id":"MASKED"` |
| `span_id` | `"span_id":"([^"]*)"` | `"span_id":"MASKED"` |
| `idempotency_key` | `"idempotency_key":"([^"]*)"` | `"idempotency_key":"MASKED"` |

> **Note**: Because `encoding/json` marshals struct fields in declaration order and the values are
> UUIDs / hex strings (no embedded quotes), the simple `[^"]*` class is sufficient and safe.

---

## State Transitions

```
invoke run() ×2
     │
     ▼
[RunResult × 2]
     │
     ├── exit != 0 or empty stdout ──→ FAIL (execution error)
     │
     ▼
[Mask both outputs]
     │
     ├── line count mismatch (NDJSON) ──→ FAIL (structural mismatch)
     │
     ▼
[Compare masked outputs]
     │
     ├── bytes differ ──→ FAIL (determinism regression)
     │
     ▼
[Timestamp scan (optional validator)]
     │
     ├── non-UTC RFC 3339 found ──→ FAIL (format violation)
     │
     ▼
[Fully-typed unmarshal (optional validator)]
     │
     ├── unmarshal error ──→ FAIL (type contract violation)
     │
     ▼
PASS
```
