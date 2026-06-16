# Feature Specification: Loki Direct-Push Addon

**Feature Branch**: `007-loki-direct-push`

**Created**: 2026-06-14

**Status**: Draft

**Input**: User description: "Loki direct-push addon (loki.go, ADR-0006) — ADR-0006 makes direct Loki push opt-in via AX_LOKI_URL. It must live as a separate addon (loki.go), never coupled into the core logger (logger.go) — the logger must stay shippable with no Loki dependency. Push is non-blocking; network failures never break the CLI's primary work. Secure transport. Race-tested."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Standalone binary operator enables Loki push (Priority: P1)

A developer deploys an ax-go–based CLI on a workstation or edge server
(outside Kubernetes). They set `AX_LOKI_URL` in the process environment.
When they run any CLI command, log lines that would normally go only to
`stderr` are also forwarded to the Loki push endpoint — without slowing
down the command or losing stderr output.

**Why this priority**: This is the entire raison d'être of the addon.
The decoupled-default (Promtail/Alloy sidecar) covers containerized
deployments; this story covers the standalone binary gap. Without P1,
the feature has no delivered value.

**Independent Test**: Can be tested by: setting `AX_LOKI_URL` pointing
at a local or test Loki instance, running the CLI, then querying Loki
and confirming the log line arrived. Standalone value: operators get
centralized logs from non-containerized deployments.

**Acceptance Scenarios**:

1. **Given** `AX_LOKI_URL` is set to a reachable Loki endpoint, **When** the CLI emits a log line at any level, **Then** that line is pushed to Loki in the expected label+stream format AND still appears on stderr.
2. **Given** `AX_LOKI_URL` is set and Loki is temporarily unreachable, **When** the CLI emits a log line, **Then** the push silently drops or retries within a bounded time and the CLI exits normally with the correct exit code.
3. **Given** `AX_LOKI_URL` is set and the push buffer is full, **When** new log lines arrive, **Then** excess lines are dropped without blocking the caller goroutine.

---

### User Story 2 - Operator leaves Loki push disabled (Priority: P1)

A developer or CI operator runs an ax-go–based CLI without setting
`AX_LOKI_URL`. The binary behaves exactly as it did before the addon was
introduced: logs go to stderr only, no network connections are made, and
the compiled binary carries no Loki client code in its import graph.

**Why this priority**: Import-graph isolation is the primary constraint
from ADR-0006 and the GitHub issue. Failure here violates the
"logger.go stays shippable without Loki dependency" requirement, which
is a constitution violation (Principle VI: scope creep).

**Independent Test**: Can be tested by: running `go tool nm` (or
equivalent) on a binary built without the addon and confirming no Loki
client symbols are present. The logger_test.go suite must also remain
green in isolation from loki.go.

**Acceptance Scenarios**:

1. **Given** `AX_LOKI_URL` is not set, **When** the CLI runs, **Then** no outbound HTTP connections are attempted and stderr output is unchanged.
2. **Given** `AX_LOKI_URL` is not set, **When** `logger.go` is compiled alone (loki.go excluded from the build), **Then** it has no dependency on any Loki client package.

---

### User Story 3 - Operator uses mTLS / authenticated Loki (Priority: P2)

An operator runs a Loki endpoint that requires TLS and/or bearer-token
authentication. They set `AX_LOKI_URL` (and optionally
`AX_LOKI_AUTH_TOKEN`). Log pushes use the secure transport defaults
mandated by Constitution Principle IX — no skipped TLS verification.

**Why this priority**: Security is non-negotiable (Constitution Principle
IX). However, testing authenticated Loki is an integration concern; the
unit-testable shape of the feature (non-blocking push, import isolation)
comes first.

**Independent Test**: Can be tested by: pointing `AX_LOKI_URL` at an
HTTPS endpoint with a self-signed cert (no skip-TLS flag set) and
confirming the push fails with a TLS error rather than a panic or
connection to an insecure server.

**Acceptance Scenarios**:

1. **Given** `AX_LOKI_URL` points to an HTTPS Loki instance and `AX_LOKI_AUTH_TOKEN` is set, **When** the CLI logs, **Then** the push request includes a `Authorization: Bearer <token>` header and uses a TLS-verified connection.
2. **Given** `AX_LOKI_URL` is an HTTP URL (not HTTPS), **When** the CLI logs, **Then** the push proceeds (downgrade allowed for dev/loopback) but a warning is emitted to stderr about insecure transport.
3. **Given** `AX_LOKI_URL` is HTTPS with an invalid/expired cert and skip-TLS is not configured, **When** the CLI logs, **Then** the push fails gracefully (drop + warning to stderr) with no panic or CLI exit-code change.

---

### User Story 4 - Platform engineer verifies cardinality discipline (Priority: P2)

A platform engineer audits the log stream pushed to Loki and verifies
that only the five permitted label fields appear as Loki stream labels:
`environment`, `application`, `level`, `host`, `version`. High-cardinality
fields such as `trace_id`, `span_id`, and `user_id` appear only in the
JSON payload body, never as labels.

**Why this priority**: Cardinality violations degrade Loki performance
cluster-wide (ADR-0006 cardinality rule). Enforcing this at the API
level (not just documentation) is explicitly required.

**Independent Test**: Can be tested by: capturing a Loki push request
body in a unit test and asserting the `streams[*].stream` map contains
only the five allowed keys.

**Acceptance Scenarios**:

1. **Given** a logger with labels `{environment: "prod", application: "myapp", host: "box1", version: "1.2.3"}`, **When** a log event with `trace_id` in the payload is pushed, **Then** the Loki push body's stream map contains exactly the five permitted labels and `trace_id` appears only in the log line JSON, never as a stream key.
2. **Given** a caller attempts to pass `trace_id` as a label field (e.g., by manipulating Labels struct), **When** the addon processes the push, **Then** it is rejected or silently omitted from the stream map.

---

### Edge Cases

- What happens when `AX_LOKI_URL` is set to an invalid URL (malformed, no
  scheme)? The addon must fail at initialization with a clear error logged
  to stderr; the CLI proceeds normally using only stderr logging.
- What happens when the Loki endpoint returns a non-2xx HTTP status? The
  push should be treated as a transient failure: logged to stderr at debug
  level, dropped, and the CLI continues normally.
- What happens under concurrent goroutines all emitting logs simultaneously?
  The push buffer must be safe for concurrent writers (race detector must
  pass).
- What happens when the CLI process exits before the buffer is fully
  flushed? A best-effort flush (bounded by a short deadline) should be
  attempted on shutdown; remaining buffered lines may be dropped.
- What happens if `AX_LOKI_URL` changes mid-process (e.g., via a signal
  triggering reload)? Out of scope for v1: the env var is read once at
  logger construction time.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The addon MUST be implemented in a separate source file (`loki.go`) such that when `loki.go` is excluded from the build, no Loki client symbols appear in the compiled binary.
- **FR-002**: The addon MUST check for the `AX_LOKI_URL` environment variable at logger construction time; if absent or empty, the addon MUST be a no-op with zero impact on `logger.go` behavior.
- **FR-003**: When `AX_LOKI_URL` is set, the addon MUST push log lines to the Loki HTTP push endpoint in the expected Loki JSON format (`streams` with `labels` map and `entries` array).
- **FR-004**: All pushes MUST be non-blocking — the caller goroutine writing a log line MUST NOT wait for network I/O to complete.
- **FR-005**: Network failures (connection refused, timeout, non-2xx response) MUST be handled gracefully: drop the batch, optionally retry within a bounded count, and log the failure to stderr at debug level. The CLI's primary exit code MUST NOT be affected.
- **FR-006**: The push buffer MUST have a bounded capacity; when the buffer is full, new entries MUST be dropped (not block the caller).
- **FR-007**: The push transport MUST use TLS by default with no skip-verify override; the implementation MUST follow the `ax.HTTPClient` / `ax.GRPCDial` secure defaults per Constitution Principle IX.
- **FR-008**: When `AX_LOKI_AUTH_TOKEN` is set, the push request MUST include an `Authorization: Bearer <token>` header.
- **FR-009**: Only the five permitted label fields (`environment`, `application`, `level`, `host`, `version`) MAY appear as Loki stream labels; `trace_id`, `span_id`, `user_id`, and all other payload fields MUST appear only in the JSON log line body.
- **FR-010**: The addon MUST be safe for concurrent use across multiple goroutines (must pass `go test -race`).
- **FR-011**: On CLI shutdown, the addon MUST attempt a best-effort flush of any buffered log entries within a bounded deadline (suggested: 2 seconds); entries not flushed within the deadline MAY be dropped.
- **FR-012**: `logger.go`'s import graph MUST NOT include any Loki client package; this MUST be enforced by a build or test-time check (e.g., an `import graph` test or a build tag separation).

### Key Entities

- **LokiAddon**: The addon component that wires a non-blocking Loki `io.Writer` sink into the logger when `AX_LOKI_URL` is set. Created alongside the Logger at construction time; not exposed as a user-facing type beyond a `Flush(ctx)` / `Close()` lifecycle method.
- **LokiStream**: The Loki push API request shape — a `streams` array where each stream has a label map and an array of timestamped log entries.
- **LokiLabels**: The subset of `ax.Labels` (environment, application, host, version) plus the zerolog level, used as Loki stream labels. Distinct from `ax.Labels` to enforce the cardinality rule at the type level.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can deploy an ax-go–based CLI on any non-containerized host, set `AX_LOKI_URL`, and see log lines from that CLI appear in their Loki instance within 5 seconds of the CLI running, without any code changes to the CLI itself.
- **SC-002**: A CLI that uses ax-go but does NOT set `AX_LOKI_URL` has zero additional symbols, no network connections, and no measurable startup overhead attributable to the Loki addon.
- **SC-003**: Under a sustained burst of 1,000 log lines per second from concurrent goroutines, no goroutine blocks waiting for Loki I/O, and `go test -race` reports no data races.
- **SC-004**: 100% of log pushes that encounter a network error or non-2xx response are silently dropped (or retried within a bounded count), and the CLI exits with the same exit code it would have without the addon.
- **SC-005**: A Loki stream label audit of any push request shows exactly the five permitted low-cardinality fields; `trace_id` and `span_id` appear only in the log line payload.

## Assumptions

- Source inputs: GitHub issue #7 and governing ADR `docs/adr/0006-loki-integration.md`. ADR-0006's decisions are absorbed into `research.md` during planning and the ADR is retired as the feature's final task.
- The Loki push API target is `/loki/api/v1/push` (Loki HTTP API v1); the push format is the standard JSON body (`{"streams": [...]}`).
- `AX_LOKI_URL` is read once at logger construction time (no hot-reload). A process restart is required to change the target.
- The optional `AX_LOKI_AUTH_TOKEN` bearer-token header is the only authentication mechanism in scope for v1; mTLS client certificates are out of scope.
- The addon is a pure Go `io.Writer` implementation; no CGo, no generated code.
- Batch size and flush interval have reasonable defaults (e.g., max 100 entries or 1-second interval, whichever comes first); these defaults may be tunable via additional env vars in a later iteration.
- The existing `ax.Labels` struct and `WithLoggerWriter` functional option are stable surfaces that the addon can compose against without changes to `logger.go`.
- HTTP/1.1 is sufficient for the Loki push transport in v1 (HTTP/2 is not required).
- The build tag or file-separation strategy for import isolation is a planning concern; the spec constrains the outcome (no Loki symbols when unused) but not the mechanism.
