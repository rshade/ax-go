# Public API Contract: Loki Direct-Push Addon

**Feature**: `007-loki-direct-push` | **Date**: 2026-06-14

This document defines the public API surface added by `loki.go`. All items here
are stable-by-contract and MUST be guarded by golden-file or compilation tests.

---

## New Exported Symbols

### `WithLokiFromEnv() LoggerOption`

```go
// WithLokiFromEnv returns a LoggerOption that enables direct Loki push when the
// AX_LOKI_URL environment variable is set. It reads AX_LOKI_URL and
// AX_LOKI_AUTH_TOKEN at construction time. If AX_LOKI_URL is empty or
// malformed, the option is a no-op and a warning is written to the logger's
// configured writer. Push is non-blocking; network failures are silently dropped
// and do not affect the CLI's exit code. The caller must invoke ax.Flush to
// drain buffered entries at shutdown.
func WithLokiFromEnv() LoggerOption
```

**Invariants**:
- Returns a no-op `LoggerOption` when `AX_LOKI_URL` is unset or empty.
- Reads `AX_LOKI_URL` and `AX_LOKI_AUTH_TOKEN` via `os.Getenv` at call time.
- Never panics, never returns an error (errors surface as no-op + stderr warning).
- Thread-safe: the returned option is safe to pass to multiple `NewLogger` calls.

---

### `Flush(ctx context.Context, l Logger) error`

```go
// Flush performs a best-effort, non-destructive drain of any buffered Loki log
// entries for the given Logger. It blocks until the buffer is empty, the context
// is cancelled, or an internal 2-second deadline elapses — whichever comes
// first. Remaining entries are dropped after the deadline.
//
// Flush is a no-op (returns nil) when:
//   - l has no Loki sink (AX_LOKI_URL was not set)
//   - l is nil
//   - the sink's background goroutine already stopped because its logger context
//     was cancelled
//
// Callers may invoke Flush multiple times; later writes remain deliverable by a
// later Flush call. Callers should invoke Flush in their shutdown path, before
// os.Exit or cobra.Command cleanup, to ensure in-flight log lines reach Loki.
func Flush(ctx context.Context, l Logger) error
```

**Invariants**:
- Safe to call multiple times.
- Does not close the underlying `Logger` or Loki sink; the logger remains usable
  after `Flush`.
- The Loki sink reports nil even when its flush deadline elapses; undelivered
  entries are dropped per the fail-open contract. `Flush` may return non-nil
  only if another sink reports a drain error.

---

## `Logger` Interface (unchanged)

The existing `Logger` interface is **not modified**. `Flush` uses an unexported
`flusher` interface via type assertion inside the `ax` package — no change to
the public `Logger` contract.

---

## Environment Variables (new)

| Variable | Required | Description |
|----------|----------|-------------|
| `AX_LOKI_URL` | No | Full URL of the Loki push endpoint (e.g. `http://loki:3100`). When set, `WithLokiFromEnv()` enables direct push. |
| `AX_LOKI_AUTH_TOKEN` | No | Bearer token for Loki authentication. When set, requests include `Authorization: Bearer <token>`. |

**Notes**:
- Both variables are read once at `NewLogger` construction time (inside
  `WithLokiFromEnv`), never in background goroutines or at package init.
- Changing variables after `NewLogger` has been called has no effect; restart
  the process to pick up changes.

---

## Loki Push API Contract (consumed, not provided)

The addon targets the **Loki HTTP API v1**:

```
POST <AX_LOKI_URL>/loki/api/v1/push
Content-Type: application/json
Authorization: Bearer <AX_LOKI_AUTH_TOKEN>   // omitted when unset

{
  "streams": [
    {
      "stream": {
        "environment": "<Labels.Environment>",
        "application":  "<Labels.Application>",
        "host":         "<Labels.Host>",
        "version":      "<Labels.Version>",
        "level":        "<zerolog level field>"
      },
      "values": [
        ["<unix_nanoseconds_string>", "<zerolog JSON line>"]
      ]
    }
  ]
}
```

**Cardinality contract** (FR-009):
- `stream` map contains **at most** the five permitted keys above.
- Empty-string label values are omitted.
- `trace_id`, `span_id`, `user_id`, and all other fields appear only in the log
  line JSON (the second element of each `values` entry).

**Compatibility**: Loki ≥ 2.0 (HTTP push API v1). The addon does NOT use the
protobuf push path or Loki API v2.

---

## Backward Compatibility

- `NewLogger` signature is unchanged: `func NewLogger(ctx context.Context, opts ...LoggerOption) Logger`.
- All existing `LoggerOption` values (`WithLoggerWriter`, `WithLoggerLevel`,
  `WithLoggerLabels`) continue to work without modification.
- Adding `WithLokiFromEnv()` to an existing `NewLogger` call is the only
  adoption step. It is additive and safe in environments where `AX_LOKI_URL` is
  not set.
