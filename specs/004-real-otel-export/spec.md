# Feature Specification: Real OTel Export & Span Lifecycle

**Feature Branch**: `004-real-otel-export`

**Created**: 2026-06-10

**Status**: Draft

**Input**: User description: "Real OTel export + span lifecycle" (GitHub issue #2)

## User Scenarios & Testing *(mandatory)*

The consumers of this feature are: (a) **LLM agents** — which read every log
line's `trace_id`/`span_id` to correlate a run's logs with its trace, attribute
failures, and stitch a multi-process workflow into one trace; (b) **operators**
— who run a collector (Grafana Tempo/Jaeger/Honeycomb via OTLP) and need the
spans a short-lived CLI emits to actually arrive before the process exits; and
(c) **adopters** — Go developers building CLIs on `ax-go` who want correct
telemetry with zero per-command wiring, and a footprint of exactly zero when no
collector is configured.

Today the tracer provider is installed with no span processor and no exporter,
and nothing wraps command execution in a span. Two consequences follow: spans
are created and immediately discarded (nothing is ever exported), and — because
no span is active for a normal invocation — every log line carries the all-zeros
`trace_id`/`span_id` unless an external `TRACEPARENT` happened to be injected.
The central promise of zero telemetry loss for short-lived CLI processes is
unmet.

### User Story 1 - Every command's logs are trace-correlated, with no collector required (Priority: P1)

An agent runs a CLI command the ordinary way — no inbound trace context, no
collector configured. Each log line the command emits to `stderr` now carries a
real, non-zero `trace_id` and `span_id` that identify the command's execution,
because a root span is active for the whole command. The agent can group all of
a run's logs by `trace_id` and tie a specific log line to a specific span.

**Why this priority**: This is the defect the issue names first and the value
most consumers feel immediately — it requires no infrastructure at all. Without
an active span, correlation is structurally impossible and every log line lies
with all-zeros IDs. Every other story builds on a root span existing.

**Independent Test**: Run a command with no `TRACEPARENT` and no collector
endpoint set, capture `stderr`, and confirm the log lines carry a non-zero
`trace_id`/`span_id` that match the active span for the run (and are identical
across lines of the same run). Delivers the core value — log correlation for
every invocation — on its own.

**Acceptance Scenarios**:

1. **Given** a command invoked with no inbound `TRACEPARENT` and no exporter
   configured, **When** it logs while running, **Then** every log line carries a
   non-zero `trace_id` and `span_id` belonging to the run's root span.
2. **Given** the same command, **When** it is invoked with an inbound
   `TRACEPARENT`, **Then** the root span continues that remote trace and the
   logs carry the inbound `trace_id` (cross-process trace continuity is
   preserved).
3. **Given** any command, **When** it completes, **Then** the `trace_id`/`span_id`
   on its logs do not appear in, and do not alter, the `stdout` machine payload.

---

### User Story 2 - Spans actually reach a configured collector, with zero loss on exit (Priority: P2)

An operator points the CLI at their collector by setting the standard OTLP
endpoint environment variable. When a command runs, the spans it produces are
exported to that collector and are guaranteed to have been flushed before the
short-lived process exits — no spans are silently dropped by an asynchronous
batch that never got a chance to send. Transport to the collector uses secure
defaults; certificate verification is never disabled.

**Why this priority**: This is the half of ADR-0005's "zero telemetry loss"
promise that needs real infrastructure, so it is valuable but secondary to
correlation working everywhere. It depends on Story 1's root span existing to
have anything meaningful to export.

**Independent Test**: With an OTLP endpoint configured to a capturing receiver,
run a command, let the process exit normally, and confirm the receiver observed
the run's span(s) — proving export happened before exit rather than being
dropped.

**Acceptance Scenarios**:

1. **Given** an OTLP endpoint is configured, **When** a command runs to
   completion and the process exits, **Then** the run's span(s) were exported to
   that endpoint before exit.
2. **Given** an OTLP endpoint AND an inbound `TRACEPARENT`, **When** the command
   exports, **Then** the exported spans carry the inbound trace ID, so the
   collector stitches this process into the larger trace.
3. **Given** a configured endpoint that is unreachable or fails, **When** the
   command runs, **Then** the command's `stdout` payload and exit code are
   unchanged and the failure surfaces only as a `stderr` diagnostic (telemetry
   never breaks the command).
4. **Given** NO endpoint configured (the default), **When** a command runs,
   **Then** there is no exporter footprint at all and the command behaves
   byte-for-byte as if telemetry export did not exist.

---

### User Story 3 - A developer can see spans locally without running a collector (Priority: P3)

A developer debugging telemetry locally sets a single debug environment
variable. The command then prints its span data, in human-readable form, to
`stderr` — never `stdout` — so the developer can inspect what would be exported
without standing up a collector.

**Why this priority**: This is a developer-ergonomics affordance, not a
production guarantee, so it is the lowest priority. It reuses the same span
lifecycle Stories 1 and 2 establish and only swaps the destination.

**Independent Test**: Set the debug variable, run a command with no collector,
and confirm span data appears on `stderr` and nothing telemetry-related appears
on `stdout`.

**Acceptance Scenarios**:

1. **Given** the debug variable is set, **When** a command runs, **Then** its
   span data is written to `stderr` in a human-readable form.
2. **Given** the debug variable is set, **When** a command runs, **Then** no
   span data is written to `stdout` (stream separation holds).
3. **Given** the debug variable is NOT set, **When** a command runs, **Then** no
   span data is printed anywhere (debug output is strictly opt-in).

---

### Edge Cases

- **Configured collector is unreachable/slow/TLS-failing**: The command MUST
  still complete with its normal `stdout` and exit code; the telemetry failure
  is at most a `stderr` diagnostic. Telemetry is instrumentation on the brake —
  it must never break the engine.
- **Flush cannot complete within the shutdown budget**: The flush-on-exit MUST
  be bounded by the existing shutdown timeout and MUST NOT hang the process; a
  partial export under a genuinely stuck collector is acceptable, a hung CLI is
  not.
- **Malformed telemetry configuration** (e.g., an unparseable endpoint value):
  MUST degrade to the no-op default with a `stderr` diagnostic rather than
  failing the command or corrupting `stdout` (fail-open for telemetry).
- **Both the OTLP endpoint and the debug variable are set**: Both destinations
  receive the run's spans; neither suppresses the other.
- **No active span (defensive)**: If, for any reason, no span is active when a
  log line is emitted, correlation MUST fall back to the documented all-zeros
  IDs rather than panicking — the existing zero-value behavior.
- **Concurrent logging during export/shutdown**: Span export, log emission, and
  flush-on-exit run without data races.

## Clarifications

### Session 2026-06-10

- Q: FR-004 forbids disabling TLS certificate verification but is silent on
  whether a plaintext `http://` OTLP endpoint (the common local-collector case)
  is permitted. Which transport policy should the OTLP exporter enforce? → A:
  Follow the endpoint's URI scheme — a plaintext `http://` endpoint is permitted
  and an `https://` endpoint uses verified TLS; certificate verification is never
  disabled for either.
- Q: When an inbound `TRACEPARENT` is marked not-sampled (flags=00), should the
  command still export its span? → A: Always sample and export — every command
  records and exports its root span regardless of the inbound sampled flag; the
  sampling decision is never deferred to the inbound flag.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A root span MUST wrap the full execution of a command within the
  standard execution path, so a valid (non-zero) span context is active for the
  entire duration of every command — including when no inbound `TRACEPARENT` is
  present.
- **FR-002**: When an inbound `TRACEPARENT` is present, the root span MUST
  continue that remote trace (the run's `trace_id` equals the inbound trace ID),
  preserving cross-process trace continuity per the existing W3C extraction. The
  command MUST record and export its span even when the inbound `TRACEPARENT` is
  marked not-sampled (flags=00): ax-go always samples the root span and does NOT
  defer the export decision to the inbound sampled flag, so a configured
  collector always observes this process's span.
- **FR-003**: Every log line emitted while a span is active MUST carry that
  span's `trace_id` and `span_id` as non-zero values, replacing today's
  all-zeros output for normally-invoked commands.
- **FR-004**: A real span exporter MUST be auto-configured when the standard OTLP
  endpoint environment variable is set, targeting that endpoint, using secure
  transport defaults. The exporter MUST honor the endpoint's URI scheme: a
  plaintext `http://` endpoint (e.g., a local collector) is permitted, and an
  `https://` endpoint MUST use verified TLS. TLS certificate verification MUST
  NEVER be disabled for either scheme (plaintext transport is allowed;
  *unverified* TLS is not).
- **FR-005**: When no exporter endpoint is configured (the default), telemetry
  MUST degrade to a no-op: a root span still provides a valid context for log
  correlation, but nothing is exported, and the command's `stdout`, exit code,
  and observable behavior are byte-for-byte unaffected (zero footprint).
- **FR-006**: A debug span exporter MUST be available, gated on a dedicated debug
  environment variable, emitting human-readable span data to `stderr` only —
  never `stdout`. It MUST be strictly opt-in (absent the variable, no span data
  is printed).
- **FR-007**: Pending spans MUST be flushed before process exit through a
  synchronous, short-lived-process-correct export path — NOT an asynchronous
  batch path that can silently drop spans when the process exits before the
  batch is sent. The flush MUST be driven by the existing flush-on-exit shutdown
  and bounded by its timeout so a stuck collector cannot hang the CLI.
- **FR-008**: Telemetry export failures (unreachable collector, timeout, TLS
  error, exporter construction failure) MUST NOT change the command's exit code
  and MUST NOT corrupt or appear on `stdout`; they degrade to a `stderr`
  diagnostic at most (fail-open).
- **FR-009**: No telemetry data — exported span payloads, debug span output,
  export-error diagnostics, or shutdown-error diagnostics — may ever be written
  to `stdout`. Stream separation holds across every telemetry path.
- **FR-010**: `trace_id` and `span_id` remain documented non-deterministic
  fields. Introducing the root span MUST NOT make any `stdout` payload
  non-deterministic: the root span affects only `stderr` correlation and the
  (separate) trace export, never the machine payload.
- **FR-011**: Outbound calls made through the provided HTTP/gRPC client helpers
  MUST propagate the active root span's context, so a downstream service sees
  this command as the parent of its work (the root span makes the already-wired
  propagation meaningful).
- **FR-012**: Telemetry configuration MUST be derived from the environment at
  startup with no mutable package-level state; configuration enters through the
  existing constructor/option path. The default (nothing configured) is the
  no-op path.
- **FR-013**: The full telemetry path — exporter, concurrent logging, and
  flush-on-exit — MUST be race-clean under `go test -race`.
- **FR-014**: Any new third-party dependency required to export spans MUST be
  justified against dependency-minimalism (stdlib first, existing deps next) and
  recorded in the feature's research, as a governed addition.

### Key Entities *(include if feature involves data)*

- **Root span**: The span created around command execution; the parent of all
  work the command does and the source of the `trace_id`/`span_id` that appear
  on every log line. Continues an inbound remote trace when one is present.
- **Span exporter**: The configured destination for finished spans. Three
  states: none/no-op (default), the OTLP endpoint (when the standard endpoint
  variable is set), and the human-readable `stderr` debug exporter (when the
  debug variable is set); the last two may both be active.
- **Span processor (flush path)**: The mechanism that hands finished spans to the
  exporter and is force-flushed at process exit, guaranteeing export-before-exit
  for a short-lived process within the shutdown budget.
- **Telemetry configuration**: The environment-derived settings (OTLP endpoint,
  debug toggle, and any standard OTLP transport settings honored) resolved once
  at startup; never global mutable state.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of normally-invoked commands (no inbound `TRACEPARENT`, no
  collector) emit log lines whose `trace_id`/`span_id` are non-zero and identical
  to the run's active span — zero all-zeros-on-a-live-command regressions.
- **SC-002**: When a collector endpoint is configured, 100% of spans a command
  creates are received by the collector before the process exits, for runs that
  complete within the shutdown budget (zero loss).
- **SC-003**: With no telemetry configured, a command's `stdout` is byte-identical
  to, and its exit code unchanged from, the pre-feature behavior — no observable
  footprint.
- **SC-004**: Across all telemetry modes (no-op, OTLP, debug), `stdout` carries
  only the documented machine payload — zero telemetry bytes leak to `stdout`.
- **SC-005**: A developer can observe a local run's span data by setting a single
  environment variable, with no collector running.
- **SC-006**: A configured but failing/unreachable collector changes neither the
  command's exit code nor its `stdout` in 100% of runs.
- **SC-007**: The full telemetry path passes the race detector (`go test -race`)
  with zero reported data races.
- **SC-008**: An inbound `TRACEPARENT` is reflected in both the run's log
  correlation and the exported spans, so a multi-process workflow shares one
  `trace_id` end-to-end.

## Assumptions

- Source inputs: GitHub issue #2 and governing ADRs ADR-0005 (OTel SDK
  integration) and ADR-0004 (W3C trace-ID format). Both ADRs' decisions,
  considered alternatives, and consequences are absorbed into this feature's
  `research.md` during planning; ADR-0005 is retired as the feature's final task.
  ADR-0004's standing on trace-ID format is already a constitution principle and
  is captured but its file is retired only if it solely governs this feature
  (a planning determination).
- The exporter-selection contract follows ADR-0005: no-op by default, OTLP HTTP
  auto-enabled by the standard `OTEL_EXPORTER_OTLP_ENDPOINT` variable, and an
  opt-in `stderr` debug exporter behind a dedicated debug variable. Honoring the
  broader standard OTel environment contract (alternate protocols, per-signal
  exporter selection, sampling configuration) beyond endpoint + debug is out of
  scope for this feature and may be a future addition. Because sampling
  configuration is out of scope, the sampler is fixed to always-sample: every
  root span records and exports regardless of any inbound sampled flag (FR-002);
  per-run sampling tuning is a possible future addition.
- Transport security follows the endpoint's URI scheme rather than forbidding
  plaintext: a plaintext `http://` endpoint (the common local-collector case) is
  accepted, an `https://` endpoint uses verified TLS, and certificate
  verification is never disabled (FR-004). "Secure transport defaults" means
  never *weakening* TLS, not mandating TLS for every endpoint.
- Telemetry is fail-open: invalid telemetry configuration or a failing exporter
  degrades to no-op with a `stderr` diagnostic and never changes the command's
  exit code, because ax-go is the brake, not the engine, and telemetry is its
  instrumentation. The exact diagnostic wording is a planning detail.
- The flush-on-exit budget is the existing shutdown timeout already wired into
  the execution path; this feature makes that flush export real spans rather than
  no-ops and does not change the budget's default.
- "Synchronous, short-lived-process-correct export" means finished spans are
  handed to the exporter promptly (not buffered in an asynchronous batch that may
  never flush before exit); the precise span-processor mechanism is a planning
  detail constrained only by the zero-loss-on-exit guarantee.
- The root span's name and attributes are a planning detail; the spec constrains
  only that a valid span context is active for the whole command and that no
  span attribute carries PII, secrets, or high-cardinality resource IDs as
  low-cardinality labels.
- Any new exporter dependency is expected to be the OTel-canonical exporter
  package(s); the exact module path(s) are a planning detail justified in
  `research.md`.
