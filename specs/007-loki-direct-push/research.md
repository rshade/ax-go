# Phase 0 Research: Loki Direct-Push Addon

**Feature**: `007-loki-direct-push` | **Date**: 2026-06-14

This document resolves all open design decisions for the feature and **absorbs
governing ADR-0006** (Grafana Loki Backend Integration), satisfying the
Constitution §Governance ADR-absorption gate. ADR-0006 will be retired as the
feature's final task. No `NEEDS CLARIFICATION` markers remain.

---

## ADR-0006 Absorption (Decision Records Absorbed)

### ADR-0006 — Grafana Loki Backend Integration

**Status**: ACCEPTED 2026-05-28

**Decision**: Adopt Approach 3 (both decoupled and direct-push, env-gated).

- **Default (Approach 1)**: CLI writes JSON to `stderr`; Promtail/Alloy
  DaemonSet tails it in containerized environments.
- **Opt-in (Approach 2)**: When `AX_LOKI_URL` is set, enable a direct HTTP push
  sink alongside `stderr`.

**Considered alternatives**:

| Option | Description | Verdict |
|--------|-------------|---------|
| Approach 1 only | Decoupled via Promtail/Alloy | Rejected — useless for standalone binaries |
| Approach 2 only | Always direct push | Rejected — adds network overhead even for containerized CLIs that have sidecars |
| Approach 3 (chosen) | Default stderr; push when `AX_LOKI_URL` set | Accepted — covers both deployment shapes |

**Consequences** (absorbed from ADR):

- Approach 1 alone keeps the base zero-dependency for log shipping.
- Approach 2/3 adds retry/drop-on-full-buffer logic.
- Cardinality enforcement requires the public logger API to separate label fields
  from payload fields; ZeroLog's context-builder pattern carries label state.
- The Loki push must live in a **separate source file** (`loki.go`) — never
  coupled into `logger.go`.

**ADR file**: `docs/adr/0006-loki-integration.md` — retirement is the final
task in `tasks.md`.

---

## Current-State Diagnosis

`logger.go` contains a clean functional-options `NewLogger` that accepts an
`io.Writer` sink (default `os.Stderr`) and a `Labels` struct. There is no Loki
dependency anywhere in the module. The `Logger` interface has four logging
methods plus `WithLabels` and `Zerolog()`. A `tracingHook` attached to zerolog
injects `trace_id`/`span_id` on every event.

`http.go` provides `ax.HTTPClient()` returning an `*http.Client` already
instrumented with OTel propagation. This client satisfies the "secure transport
defaults" requirement (it inherits `http.DefaultTransport` which enforces TLS
by default).

`go.mod` already includes `net/http` (stdlib) and `encoding/json` (stdlib)
transitively via existing deps. No third-party Loki client library is needed —
the Loki v1 push API is a simple `POST /loki/api/v1/push` with a JSON body.

---

## Decisions

### D1 — No third-party Loki client; stdlib HTTP only

- **Decision**: Implement the Loki push using `net/http` + `encoding/json`
  (both stdlib). No new `go.mod` entry.
- **Rationale**: The Loki push API v1 is a simple HTTP POST with a JSON body
  (`{"streams":[...]}`). A dedicated client library adds maintenance burden
  (Principle X: Dependency Minimalism). Stdlib already satisfies all needs:
  TLS, JSON encoding, context cancellation.
- **Alternatives rejected**:
  - `grafana/loki-client-go` — heavy, introduces Prometheus metrics and
    protobuf; overkill for a single endpoint.
  - `go-kit/log/loki` — unmaintained.
- **Import isolation consequence**: Because `loki.go` uses only stdlib packages
  already present in the module, the `ax` package's overall import graph does
  not grow. `logger.go` gains no new imports.

### D2 — `additionalSinks []logSink` field in `loggerConfig` / `zerologLogger`

- **Decision**: Add a single `additionalSinks []logSink` field to `loggerConfig`
  (functional-options config struct) and to `zerologLogger` (concrete Logger
  type). `logSink` is an unexported interface composed from stdlib
  `io.Writer` plus `drain(context.Context) error`. `NewLogger` fans out to
  `io.MultiWriter(cfg.writer, sinks...)` when any sinks are present. No Loki
  import enters `logger.go`.
- **Rationale**: This is the minimum surgery on `logger.go` needed to support
  any optional write-through sink. The field is fully generic (not Loki-specific)
  so the same hook could support future sinks (e.g. Splunk, Cloud Logging).
- **Alternatives rejected**:
  - **Global hook var + `init()`** — mutable package-level state, explicitly
    prohibited by Constitution Principle X.
  - **New `NewLokiLogger` function** — would duplicate the entire `NewLogger`
    option chain; violates DRY and couples the API to Loki.
  - **Separate package `internal/loki`** — requires the CLI author to import it
    explicitly and wire it; adds package-graph complexity.

### D3 — `WithLokiFromEnv() LoggerOption` as the activation mechanism

- **Decision**: `loki.go` exports a `WithLokiFromEnv() LoggerOption`. When
  called (inside `NewLogger`), it reads `AX_LOKI_URL`; if empty, it is a no-op.
  If set, it constructs a `*lokiWriter` and appends it to `cfg.additionalSinks`.
- **Rationale**: Reads the env var at construction time (not at `init()` or
  package-level), satisfying the constitution's prohibition on `init()` that
  reads runtime environment. The CLI author adds `ax.WithLokiFromEnv()` once to
  their `NewLogger` call; the env var controls behavior per deployment.
- **The "auto" qualifier from ADR-0006** means: once the CLI author opts in with
  `WithLokiFromEnv()`, the operator controls activation with just the env var.
  No code changes are needed to enable/disable push per environment.
- **Alternatives rejected**:
  - **Auto-wire inside `NewLogger` without an option** — couples `logger.go` to
    `loki.go` (calling `newLokiWriter` from `logger.go`); violates the
    separation requirement.
  - **`WithLoki(url string) LoggerOption`** — forces the CLI author to thread the
    URL through their config; the env-var pattern is simpler.

### D4 — Non-blocking channel-based writer with bounded buffer

- **Decision**: `lokiWriter.Write(p []byte)` copies the log line into a
  `chan []byte` with a fixed capacity (default 256 entries). If the channel is
  full, the entry is silently dropped and `Write` returns `(len(p), nil)`
  immediately (never blocks the caller). A single background goroutine drains
  the channel, batches entries (up to `maxBatchSize=100` entries or
  `flushInterval=1s`, whichever comes first), and POSTs the batch to Loki.
- **Rationale**: Caller goroutines must never wait on network I/O (FR-004,
  SC-003). Fixed channel capacity prevents unbounded memory growth. `Write`
  returning `nil` even on drop means zerolog never sees an error and the CLI
  continues normally (FR-005, SC-004).
- **Alternatives rejected**:
  - **Blocking write** — violates FR-004 (non-blocking mandate).
  - **`sync.Pool` of byte slices** — optimizes allocation but over-engineers
    for this phase; address if benchmarks show need.

### D5 — `Flush(ctx context.Context, l Logger) error` as public shutdown API

- **Decision**: `loki.go` exports `Flush(ctx context.Context, l Logger) error`.
  It type-asserts `l` against an unexported `flusher` interface
  (`flush(context.Context) error`). `zerologLogger` implements `flush()` by
  calling `drain(ctx)` on each additional sink. Returns nil if no Loki sink is
  present.
- **Rationale**: The public `Logger` interface must not change (breaking API
  change) but CLI authors need a hook for graceful shutdown. The unexported
  interface + package-level function pattern (common in the stdlib — e.g.
  `http.Flusher`) is idiomatic and non-breaking. `lokiWriter.drain(ctx)` sends
  an in-band flush request to the background goroutine, waits up to 2 seconds,
  and leaves the goroutine running so later writes remain deliverable.
- **Flush deadline**: `lokiWriter.drain(ctx)` respects the caller's context but
  applies a hard ceiling of 2 seconds if the context is longer (CLI shutdown
  budget from ADR-0006 consequences; matches `defaultTelemetryShutdownTimeout`).
- **Alternatives rejected**:
  - **`Logger.Close() error`** — API break on the `Logger` interface.
  - **`runtime.SetFinalizer`** — non-deterministic, unsuitable for network flushes.
  - **Signal handler in `loki.go`** — out of scope for a library; the adopting
    CLI owns the signal handler.

### D6 — Cardinality enforcement via `LokiStream` label extraction

- **Decision**: When building the Loki push body, extract exactly five stream
  labels from the emitted zerolog JSON line:
  `environment`, `application`, `host`, `version`, plus `level` (extracted by
  parsing the `"level"` field from the zerolog JSON line). All other fields
  remain in the log-line JSON body. This is enforced by constructing the label
  map from a fixed allowlist, not from arbitrary JSON fields. Entries are
  grouped by the full stream key, not just by level, because base and derived
  loggers can share a Loki sink while emitting different labels.
- **Rationale**: ADR-0006 cardinality rule; Constitution Principle VIII; FR-009.
  Loki streams are indexed by label map; high-cardinality fields (`trace_id`,
  `span_id`, `user_id`) would degrade cluster performance if promoted. Extracting
  from the emitted line ensures `WithLoggerLabels(...)` option order and
  `Logger.WithLabels(...)` derivation cannot make visible log fields diverge
  from Loki's indexed labels.
- **Level extraction**: zerolog always emits `"level":"info"` (or debug/warn/error)
  as the first field. A `bytes.Index` scan (not a full JSON unmarshal) extracts
  it for stream-label purposes; the four optional label fields are read from the
  JSON line with `encoding/json` so unknown numeric payload fields do not affect
  label parsing.

### D7 — `AX_LOKI_AUTH_TOKEN` for bearer-token authentication

- **Decision**: If `AX_LOKI_AUTH_TOKEN` is set, each push request includes
  `Authorization: Bearer <token>`. Both env vars are read once at construction
  time (inside `WithLokiFromEnv`) and stored in the `lokiWriter`. mTLS is out
  of scope for v1.
- **Rationale**: FR-008; simplest auth mechanism for Loki BasicAuth/token setups.
  Reading at construction time satisfies the prohibition on runtime env reads
  during goroutine execution.

### D8 — Invalid `AX_LOKI_URL` handling

- **Decision**: `WithLokiFromEnv` validates the URL with `url.Parse`; if
  malformed or scheme-less, it emits a warning to `cfg.writer` (stderr at the
  time of construction) and returns the no-op option.
- **Rationale**: FR-002 (no-op when URL absent); spec edge case (invalid URL →
  fallback to stderr-only). Failing open (stderr warning + no Loki) is
  preferable to `NewLogger` returning an error (the function currently has one
  return value; adding error would break the API).

### D9 — `go test -race ./...` passes; no data races

- **Decision**: All shared state in `lokiWriter` is mediated by the channel and
  in-band flush request acknowledgements. No `sync.Mutex` is needed in the write
  path (the channel is the synchronization primitive). The background goroutine
  is the sole reader. Concurrent `Flush` calls each send a request and wait on
  their own acknowledgement.
- **Rationale**: FR-010, SC-003. The channel-based design eliminates most
  race conditions by construction, and `Flush` does not close shared state while
  writers may still be active.

### D10 — Import-graph test verifies no third-party Loki symbols

- **Decision**: Add `TestLokiImportIsolation` in `loki_test.go` that runs
  `go list -json -deps github.com/rshade/ax-go` and asserts no package with
  "loki" in its import path appears. Since we use only stdlib (D1), this test
  should trivially pass — but it serves as a regression guard if someone
  later adds a Loki client library and forgets the isolation requirement.
- **Rationale**: FR-012; the test makes the import-graph constraint machine-
  checkable rather than documentation-only.

### D11 — HTTP/1.1 via `ax.HTTPClient()` with 10-second per-request timeout

- **Decision**: The background goroutine uses `ax.HTTPClient()` (already
  OTel-instrumented, inherits `http.DefaultTransport` TLS defaults) with a
  10-second request timeout derived from a `context.WithTimeout` wrapping the
  background context. Non-2xx responses are treated as transient failures:
  reported to the logger's configured writer, batch dropped, goroutine continues.
- **Rationale**: FR-007 (secure transport); FR-005 (graceful failure); D4
  (never block caller). 10 seconds is generous for a local or regional Loki
  endpoint; a future iteration can make this configurable.

---

## Open Questions (None)

All spec `NEEDS CLARIFICATION` markers were pre-resolved. No open questions
remain.
