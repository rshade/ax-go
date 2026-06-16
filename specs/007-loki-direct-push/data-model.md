# Phase 1 Data Model: Loki Direct-Push Addon

**Feature**: `007-loki-direct-push` | **Date**: 2026-06-14

This feature persists no data (Constitution Principle VI — no state). The
"entities" are the runtime values that configure and drive the Loki push sink,
mapped to concrete Go types and the ax-go public surface.

---

## Entities

### LoggerConfig extension (`additionalSinks`)

- **What**: the slice of additional `logSink` values appended to the
  zerolog writer fan-out inside `loggerConfig`. Each sink receives a copy of
  every log line via `io.MultiWriter`.
- **Added to**: `loggerConfig` struct in `logger.go` (existing type); field type
  is an unexported `[]logSink` interface (`io.Writer` plus `drain(context.Context)
  error`) — no Loki import enters `logger.go`.
- **Fields**:

  | Field | Type | Default |
  |-------|------|---------|
  | `additionalSinks` | `[]logSink` | `nil` (no extra sinks) |

- **Rules**:
  - When `len(additionalSinks) > 0`, `NewLogger` wraps the configured writer
    and all sinks with `io.MultiWriter`.
  - Each sink's `drain(ctx)` is called by `zerologLogger.flush()`, invoked via
    `ax.Flush(ctx, logger)`. Draining is non-destructive; sinks remain usable
    after a flush.
  - Write errors from sinks are silently swallowed (the zerolog writer returns
    its own error; sinks are best-effort).

---

### `lokiWriter` (new type, unexported, in `loki.go`)

- **What**: a non-blocking `logSink` that queues log lines in a bounded
  channel and a background goroutine batches them into Loki push API requests.
- **Unexported** — callers interact only through the `Logger` interface and the
  `WithLokiFromEnv()` option and `ax.Flush()` function.
- **Fields**:

  | Field | Type | Notes |
  |-------|------|-------|
  | `pushURL` | `string` | resolved Loki push endpoint (`/loki/api/v1/push`) |
  | `authToken` | `string` | value of `AX_LOKI_AUTH_TOKEN`; empty = no auth header |
  | `errorWriter` | `io.Writer` | configured logger writer for non-2xx diagnostics |
  | `ch` | `chan lokiEntry` | bounded channel (default cap 256); Write sends here |
  | `flushRequests` | `chan chan struct{}` | in-band non-destructive flush requests |
  | `client` | `*http.Client` | from `ax.HTTPClient()`; OTel-instrumented, TLS-secured |
  | `done` | `chan struct{}` | closed by the background goroutine on exit |

- **Rules**:
  - `Write(p []byte)`: records an enqueue timestamp, copies `p` into a
    `lokiEntry`, and sends it into `ch` non-blocking (select with default);
    returns `(len(p), nil)` always — callers must never observe a write error
    from a sink (FR-004/FR-005).
  - `drain(ctx)`: sends an in-band flush request to the background goroutine and
    waits for acknowledgement up to the caller's context deadline or 2 seconds,
    whichever is shorter. It does not stop the goroutine.
  - Background goroutine exits only when the logger context is cancelled. On
    exit it drains remaining entries in `ch` (bounded by `flushTimeout = 2s`),
    then closes `done`.

---

### `lokiStreamKey` (new type, unexported, in `loki.go`)

- **What**: the complete grouping key for one Loki stream. It is built from the
  emitted zerolog JSON line so option-order changes and `Logger.WithLabels(...)`
  labels are reflected in Loki's indexed labels.
- **Fields**:

  | Field | Loki label key | Source |
  |-------|----------------|--------|
  | `environment` | `"environment"` | emitted log line field |
  | `application` | `"application"` | emitted log line field |
  | `host` | `"host"` | emitted log line field |
  | `version` | `"version"` | emitted log line field |
  | `level` | `"level"` | extracted from each log line's `"level"` JSON field |

- **Rules**:
  - Label map is constructed per distinct stream key in a batch.
  - Empty-string label values are omitted from the map (consistent with how
    `applyLabels` in `logger.go` handles empty fields).
  - `trace_id`, `span_id`, `user_id`, and all other zerolog fields remain in
    the log line body (never promoted to labels).

---

### `lokiPushBody` (new type, unexported, in `loki.go`)

- **What**: the JSON request body sent to `POST /loki/api/v1/push`.
  Corresponds exactly to Loki's HTTP API v1 push format.
- **Fields**:

  ```
  lokiPushBody
  └── Streams []lokiStream
      └── lokiStream
          ├── Stream map[string]string   // the label map (≤5 keys)
          └── Values [][2]string         // [nanosecond timestamp, log line JSON]
  ```

- **Rules**:
  - Timestamps are `strconv.FormatInt(entry.ts, 10)` strings, where `entry.ts`
    is captured when `Write` accepts the log line — Loki expects
    nanosecond-precision Unix timestamps as strings.
  - A batch groups entries by full stream key identity, because labels may vary
    between base and derived loggers sharing the same sink.
  - Maximum batch size: 100 entries or 1-second flush interval.
  - Content-Type: `application/json`.

---

### State Transitions: `lokiWriter` lifecycle

```
                    WithLokiFromEnv() called; AX_LOKI_URL set
                              │
                              ▼
                   ┌─────────────────────┐
                   │   CONSTRUCTED       │  background goroutine running
                   │   Write() accepted  │  ch draining → batches POSTed
                   └─────────────────────┘
                              │
                    ax.Flush(ctx, logger)
                              │
                              ▼
                   ┌─────────────────────┐
                   │   FLUSHING          │  goroutine drains ch;
                   │   (≤2 seconds)      │  current batch posted
                   └─────────────────────┘
                              │
                     flush request acked
                              │
                              ▼
                   ┌─────────────────────┐
                   │   RUNNING           │  later Write() calls accepted
                   └─────────────────────┘
                              │
                    logger context cancelled
                              │
                              ▼
                   ┌─────────────────────┐
                   │   STOPPED           │  done closed; later entries are
                   │                     │  not delivered
                   └─────────────────────┘
```

- In `FLUSHING`: new `Write()` calls still attempt a channel send
  non-blocking. Entries accepted before the flush request is processed are
  included in that flush; later entries are delivered by the next interval or
  explicit `Flush`.
- In `STOPPED`: `Write()` may enqueue until the channel fills, but no goroutine
  remains to deliver entries; callers still see `(len(p), nil)` per fail-open
  semantics.
