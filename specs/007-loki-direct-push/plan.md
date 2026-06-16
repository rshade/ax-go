# Implementation Plan: Loki Direct-Push Addon

**Branch**: `007-loki-direct-push` | **Date**: 2026-06-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/007-loki-direct-push/spec.md`

## Summary

Add a non-blocking, env-gated Loki direct-push sink to ax-go's logger as a
separate source file (`loki.go`). When `AX_LOKI_URL` is set and the CLI author
includes `ax.WithLokiFromEnv()` in their `NewLogger` call, log lines are fanned
out to both `stderr` and the Loki push endpoint. `logger.go` gains no Loki
imports — the only coupling is a generic `additionalSinks []logSink` field
(`io.Writer` plus `drain(context.Context) error`) on `loggerConfig` and
`zerologLogger`. ADR-0006 is absorbed and retired as the final task.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: `net/http` (stdlib), `encoding/json` (stdlib),
`github.com/rs/zerolog` v1.35.1 (existing). No new `go.mod` entries.

**Storage**: N/A — feature persists no data.

**Testing**: `go test -race ./...`, `go vet ./...`, `golangci-lint run`,
`make doc-coverage`. Fuzz tests via `FuzzXxx`. `httptest.NewServer` for unit
tests of the Loki push path.

**Target Platform**: Any Go-supported platform (Linux primary; macOS for dev).

**Project Type**: Library (package `ax` at module root `github.com/rshade/ax-go`).

**Performance Goals**: `Write()` path must not block the caller goroutine
(FR-004). Buffer capacity: 256 entries default; burst of 1,000 log lines/sec
must not cause any goroutine to block (SC-003).

**Constraints**: Logger interface unchanged (no API break). No new `go.mod`
dependencies. No `init()` that reads env. No mutable package-level state.
TLS verification never skipped. `go test -race` must pass.

**Scale/Scope**: Single `loki.go` file (~200-250 LoC), one `loki_test.go`
file (~200 LoC), minimal changes to `logger.go` (~15 lines).

**Governing ADR(s)**: `docs/adr/0006-loki-integration.md` — absorbed into
`research.md` (above); will be deleted as the final task in `tasks.md`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | ✅ PASS | Loki push is an additional sink alongside stderr; stdout untouched |
| II. Deterministic Output | ✅ PASS | Loki push does not affect stdout payload |
| III. `__schema` | ✅ PASS | No new commands; no schema change |
| IV. Agent-Safety Primitives | ✅ PASS | No changes to idempotency-key or dry-run paths |
| V. Asymmetric JSON I/O | ✅ PASS | Loki push is write-only; no read path affected |
| VI. ADR-Governed Scope | ✅ PASS | `loki.go` adds no domain commands, no persistent state, no global mutable state |
| VII. Test-First Discipline | ✅ PASS | Tests and fuzz target land before implementation |
| VIII. Observability & ID Discipline | ✅ PASS | `trace_id`/`span_id` in payload only; five-label cardinality rule enforced |
| IX. Security & Resource Safety | ✅ PASS | TLS via `ax.HTTPClient()`; bounded buffer; no PII in labels; no panic |
| X. Idiomatic Go | ✅ PASS | Functional options; stdlib only; `context.Context` first; `defer Close()` |

**ADR absorption gate**: ADR-0006 decisions, alternatives, and consequences are
recorded in `research.md`. Tasks.md final task will delete `docs/adr/0006-loki-integration.md`
and update all references.

**Post-Phase 1 re-check**: All principles still satisfied. The
`additionalSinks []logSink` field added to `loggerConfig` is a generic
extension; it introduces no Loki-specific coupling into `logger.go`.

## Project Structure

### Documentation (this feature)

```text
specs/007-loki-direct-push/
├── plan.md              # This file
├── research.md          # Phase 0: ADR-0006 absorbed; all decisions resolved
├── data-model.md        # Phase 1: entity definitions
├── contracts/
│   └── public-api.md   # Phase 1: exported symbols and env-var contract
├── quickstart.md        # Phase 1: operator and CLI-author guide
├── checklists/
│   └── requirements.md  # Spec quality checklist (all items pass)
└── tasks.md             # Phase 2 (/speckit-tasks — not yet created)
```

### Source Code Changes

```text
# Modified files (minimal changes)
logger.go                    # +15 lines: additionalSinks field + MultiWriter fan-out + flush() method on zerologLogger
examples/integration/main.go # +WithLokiFromEnv() option + ax.Flush() in shutdown path

# New files
loki.go                      # ~220 LoC: WithLokiFromEnv, Flush, lokiWriter, lokiStreamKey, lokiPushBody
loki_test.go                 # ~230 LoC: unit tests, race tests, import-graph test, cardinality test
```

**Structure Decision**: Module-root-flat layout (existing convention). No new
packages; `loki.go` is in package `ax` alongside `logger.go`. All Loki logic
is encapsulated in `loki.go`; `logger.go` has only a generic fan-out hook.

## Complexity Tracking

> No Constitution Check violations to justify. All principles satisfied.

---

## Implementation Guide

### 1. Changes to `logger.go`

**Add `additionalSinks` to `loggerConfig`**:

```go
type loggerConfig struct {
    writer          io.Writer
    level           zerolog.Level
    labels          Labels
    additionalSinks []logSink  // optional extra write-through sinks
}
```

**Add `sinks` to `zerologLogger`**:

```go
type zerologLogger struct {
    logger zerolog.Logger
    sinks  []logSink  // mirrors cfg.additionalSinks for flush access
}
```

**Update `NewLogger` fan-out**:

```go
w := cfg.writer
if len(cfg.additionalSinks) > 0 {
    writers := make([]io.Writer, 0, 1+len(cfg.additionalSinks))
    writers = append(writers, cfg.writer)
    for _, s := range cfg.additionalSinks {
        writers = append(writers, s)
    }
    w = io.MultiWriter(writers...)
}
// build zerolog.Logger with w instead of cfg.writer
```

**Add unexported `flush` method on `zerologLogger`** (called by `ax.Flush`):

```go
func (l zerologLogger) flush(ctx context.Context) error {
    var errs []error
    for _, s := range l.sinks {
        if err := s.drain(ctx); err != nil {
            errs = append(errs, err)
        }
    }
    // return combined error or nil
}
```

### 2. New `loki.go`

**Public surface** (see `contracts/public-api.md`):

- `WithLokiFromEnv() LoggerOption` — reads env, builds `*lokiWriter`, appends
  to `cfg.additionalSinks`, also stores in `cfg.sinks` for `zerologLogger`.
- `Flush(ctx context.Context, l Logger) error` — type-asserts to unexported
  `flusher`, calls `flush(ctx)`.

**`lokiWriter` internals**:

```
ch            chan lokiEntry       cap=256; Write sends here non-blocking
flushRequests chan chan struct{}   in-band non-destructive flush requests
done          chan struct{}        goroutine closes when it exits
client        *http.Client         ax.HTTPClient()
pushURL       string
authToken     string
errorWriter   io.Writer            configured stderr/log writer for push diagnostics
```

**Background goroutine loop**:

```
ticker := time.NewTicker(1 * time.Second)
batch := make([]lokiEntry, 0, 100)
for {
    select {
    case entry := <-lw.ch:
        batch = append(batch, entry)
        if len(batch) >= 100 {
            flush(batch); batch = batch[:0]
        }
    case <-ticker.C:
        if len(batch) > 0 {
            flush(batch); batch = batch[:0]
        }
    case ack := <-lw.flushRequests:
        // drain remaining
        drain:
        for {
            select {
            case entry := <-lw.ch:
                batch = append(batch, entry)
            default:
                break drain
            }
        }
        if len(batch) > 0 {
            flush(batch)
        }
        close(ack)
        batch = batch[:0]
    case <-ctx.Done():
        // drain remaining once, then exit and close lw.done via defer
        return
    }
}
```

**`flush(batch)` helper** (inside background goroutine):

1. Extract `level` from each line using a fast scan for `"level":"`.
2. Extract only the permitted low-cardinality label fields (`environment`,
   `application`, `host`, `version`) from the emitted zerolog JSON line.
3. Build `lokiPushBody` with one stream per distinct full stream key so base and
   derived loggers sharing a sink cannot mix entries under the wrong labels.
4. `json.Marshal(body)`.
5. `http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))`.
6. Set `Content-Type: application/json` and optionally `Authorization: Bearer ...`.
7. `client.Do(req)` with a 10-second per-request timeout.
8. Non-2xx → `fmt.Fprintf(lw.errorWriter, ...)` and return; network errors are
   dropped silently.
9. `resp.Body.Close()`.

### 3. Level extraction (performance note)

zerolog always serializes level as `"level":"info"` (or debug/warn/error). A
fast scan for `"level":"` prefix followed by the level name avoids a full JSON
parse for that field. Use `strings.Index`/`bytes.Index`; read characters until
the closing `"`. If not found, default to `"unknown"`. The optional label fields
are decoded from the emitted JSON line with `encoding/json` and a fixed
allowlist so unknown numeric payload fields do not affect label parsing.

### 4. Tests to write first (TDD gate)

Before any implementation, these tests must exist and fail for the right reason:

**`loki_test.go`**:

1. `TestLokiWriterNoop_NoEnvVar` — `WithLokiFromEnv()` with `AX_LOKI_URL`
   unset; assert `len(l.(zerologLogger).sinks) == 0`.
2. `TestLokiWriterPushes_ValidURL` — spin up `httptest.NewServer`; set
   `AX_LOKI_URL`; write a log line; call `ax.Flush`; assert server received a
   POST with valid JSON body.
3. `TestLokiCardinality` — capture push body; assert `streams[*].stream` map
   has no key outside the five permitted; assert `trace_id` is in the log line
   body, not in the stream map.
4. `TestLokiWriter_NetworkFailure` — server returns 503; assert CLI does not
   exit and `Flush` returns without error.
5. `TestLokiWriter_BufferFull` — fill the 256-entry buffer; assert no goroutine
   blocks (use `go test -timeout 5s`).
6. `TestLokiWriter_Race` — concurrent writes from 10 goroutines, `Flush` from
   main; `go test -race` must pass.
7. `TestLokiImportIsolation` — `go list -json -deps github.com/rshade/ax-go`
   asserts no import path contains "loki" (regression guard for D1/D10).
8. `FuzzLokiWriter` — fuzz `Write` with arbitrary bytes; assert no panic,
   `Write` always returns `(len(p), nil)`.

### 5. `examples/integration` update

Update `examples/integration/main.go` to add:
- `ax.WithLokiFromEnv()` option in the `NewLogger` call.
- `defer ax.Flush(ctx, logger)` in the command body.

Update the `ExampleNewLogger` function in `example_test.go` to demonstrate
the `WithLokiFromEnv()` option (no-op when env unset, so the test remains
`// Output:` clean).

### 6. ADR retirement (final task)

- Transcription in `research.md` already complete.
- Final task: delete `docs/adr/0006-loki-integration.md`; update:
  - `README.md` ADR index/links
  - `CONTEXT.md` references
  - `AGENTS.md` references
  - `ROADMAP.md` references (if any)
  - Go doc-comments in `logger.go` and `loki.go` that cite ADR-0006 by file name

---

## Artifact Summary

| Artifact | Path | Status |
|----------|------|--------|
| Spec | `specs/007-loki-direct-push/spec.md` | ✅ Complete |
| Research (ADR-0006 absorbed) | `specs/007-loki-direct-push/research.md` | ✅ Complete |
| Data model | `specs/007-loki-direct-push/data-model.md` | ✅ Complete |
| Public API contract | `specs/007-loki-direct-push/contracts/public-api.md` | ✅ Complete |
| Quickstart | `specs/007-loki-direct-push/quickstart.md` | ✅ Complete |
| Tasks | `specs/007-loki-direct-push/tasks.md` | ⏳ `/speckit-tasks` |
