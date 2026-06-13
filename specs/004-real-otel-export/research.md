# Phase 0 Research: Real OTel Export & Span Lifecycle

**Feature**: `004-real-otel-export` | **Date**: 2026-06-11

This document resolves the feature's open decisions and **absorbs governing
ADR-0005** (retired as the feature's final task) and **ADR-0004** (absorbed for
the record, file retained — see D10) so the Constitution §Governance gate is
satisfied. No `NEEDS CLARIFICATION` markers remain.

## Current-state diagnosis (the defect being fixed)

`internal/telemetry/telemetry.go::Start()` installs the W3C propagator, extracts
`TRACEPARENT`/`TRACESTATE` into the context, then builds
`sdktrace.NewTracerProvider()` — **with no span processor, no exporter, and the
SDK-default `ParentBased(AlwaysSample())` sampler** — and sets it global.
`Execute()` calls this, defers `Telemetry.Shutdown` (2 s), and runs
`root.ExecuteContext(ctx)` **without ever opening a span**. Consequences:

1. **No active local span.** With no inbound `TRACEPARENT`, the context has no
   span context at all → the `tracingHook` in `logger.go` reads the zero
   `SpanContext` → every log line carries `ZeroTraceID`/`ZeroSpanID`. With an
   inbound `TRACEPARENT`, logs borrow the *remote parent's* IDs, but no local
   recording span exists.
2. **Nothing is ever exported.** A provider with no processor/exporter discards
   every span on `End()` — but nothing even calls `End()`.
3. **Fail-closed setup.** `Execute()` maps any telemetry error to `ExitInternal`,
   which FR-008 forbids.

The three fixes are D1/D2 (root span + always-sample), D3–D6 (synchronous
bounded export with env-selected exporters), and D7 (fail-open).

## Decisions

### D1 — A root span wraps command execution, opened in `Execute()`

- **Decision**: After `StartTelemetry` returns the (TRACEPARENT-extracted)
  context, `Execute()` obtains a tracer (`otel.Tracer("github.com/rshade/ax-go")`,
  the SDK global it just set) and opens a root span around
  `root.ExecuteContext(ctx)`: `ctx, span := tracer.Start(ctx, name)`. The span is
  `End()`-ed in a `defer` that is registered **after** the `Shutdown` defer, so
  LIFO ordering runs `span.End()` *before* `Telemetry.Shutdown()` — the span is
  finished (and, under D3, already exported) before the flush. The span context
  flows into `cmd.Context()` (Cobra propagates the `ExecuteContext` context), so
  `ax.NewLogger(...).Info(cmd.Context())` correlates automatically (FR-001,
  FR-003, SC-001).
- **Span name & attributes**: opened with the binary name (`root.Name()`) and
  refined to the resolved subcommand path via
  `trace.SpanFromContext(cmd.Context()).SetName(cmd.CommandPath())` inside the
  wrapped `PersistentPreRunE` (where the subcommand is known). Attributes are
  limited to low-cardinality, non-PII values (`service.name`/`service.version`
  live on the *resource*, D8); the span carries no user input, secrets, or
  resource IDs (Principle IX, spec Assumption on span attributes).
- **Command outcome on the span**: on a failing run `Execute()` sets
  `span.SetStatus(codes.Error, …)`; on success it leaves the default/`Ok`. The
  raw error *message* is **not** copied into a span attribute (it may carry
  user-controlled content; it is already on `stderr` as the `ax.Error` envelope) —
  status code only, so operators see failed vs. ok without new PII exposure.
- **Rationale**: A valid, recording span context active for the whole command is
  the structural precondition for all three user stories; without it correlation
  is impossible and there is nothing to export. Owning the lifecycle in
  `Execute()` keeps the `End()`-before-`Shutdown()` ordering and the
  command-outcome status in one auditable place, next to the existing shutdown
  defer.
- **Alternatives considered**: (a) Open the span inside `StartTelemetry` and end
  it in `Telemetry.Shutdown` — rejected: hides the ordering, couples span
  lifetime into the provider object, and loses access to `cmd.CommandPath()`. (b)
  Open the span in `PersistentPreRunE` — rejected: it would not wrap
  `PersistentPostRun`/teardown and could not be ended from `Execute`'s defer
  without extra plumbing.

### D2 — Force recording with an explicit `AlwaysSample()` sampler

- **Decision**: Configure the provider with
  `sdktrace.WithSampler(sdktrace.AlwaysSample())`, **overriding** the SDK default.
- **Why it matters (verified)**: the SDK default is `ParentBased(AlwaysSample())`
  (`sdk/trace/provider.go:501`). Under `ParentBased`, an inbound `TRACEPARENT`
  with `flags=00` (not-sampled) makes the root span **not record** → nothing is
  exported and (for a non-recording span) correlation degrades. The spec
  clarification (2026-06-10) is explicit: *always sample and export, never defer
  to the inbound sampled flag*. `AlwaysSample()` ignores parent flags, so every
  root span records and exports while still inheriting the inbound `trace_id`
  (sampling is independent of context propagation — FR-002 continuity holds).
- **Rationale**: A short-lived CLI's own span is the unit an operator most wants;
  dropping it because an upstream marked the trace unsampled would silently
  violate the zero-loss promise for exactly the cross-process case ADR-0005
  exists to serve.
- **Alternatives considered**: (a) Keep the default `ParentBased(AlwaysSample())`
  — rejected: defers to the inbound flag, contradicting the clarification and
  FR-002. (b) `TraceIDRatioBased`/configurable sampling — rejected: per-run
  sampling tuning is explicitly out of scope (spec Assumptions); the sampler is
  fixed to always-sample, with tuning a possible future addition.

### D3 — `SimpleSpanProcessor` (synchronous), not `BatchSpanProcessor`

- **Decision**: Register the exporter via
  `sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter))`
  (equivalently `WithSyncer`). `NewSimpleSpanProcessor` synchronously exports each
  span at `End()` (verified: `sdk/trace/simple_span_processor.go` — "synchronously
  sends all completed spans"). For a sub-second CLI this yields
  export-before-exit by construction: by the time the root span's `End()` returns,
  the export has been attempted; `Shutdown`/`ForceFlush` is then belt-and-braces.
- **Rationale**: FR-007 mandates a "synchronous, short-lived-process-correct
  export path — NOT an asynchronous batch path that can silently drop spans when
  the process exits before the batch is sent." `SimpleSpanProcessor` is that path.
- **Alternatives considered**: `BatchSpanProcessor` + `ForceFlush` on shutdown —
  rejected: it queues spans and exports on a background goroutine/timer; even with
  a shutdown flush, a tight exit budget can drop the tail, it adds a concurrency
  surface that complicates `-race`, and the spec forbids the async path by name.
  The synchronous processor is the more direct guarantee for CLI lifetimes.

### D4 — Bound the exporter so a stuck collector cannot hang the CLI

- **Decision**: Construct the OTLP exporter with a **per-attempt timeout derived
  from the shutdown budget** (`otlptracehttp.WithTimeout(budget)`) and **retry
  disabled** (`otlptracehttp.WithRetry(RetryConfig{Enabled: false})`). The
  shutdown budget (`WithTelemetryShutdownTimeout`, default 2 s) is threaded into
  telemetry construction via a new `WithTelemetryShutdownBudget` option.
- **Why it matters**: under D3 the export runs *inside* `span.End()` (in
  `Execute`'s defer), on the **exporter's own** timeout/retry — **not** the
  shutdown `context`. otlptracehttp defaults to a 10 s timeout *with* retry (max
  elapsed tens of seconds). Left unbounded, a stuck/slow collector would block
  `End()` long before the 2 s shutdown budget ever applies. Bounding the exporter
  makes a stuck collector degrade to a single bounded attempt (a partial/dropped
  export plus a `stderr` diagnostic), satisfying the edge case "flush cannot
  complete within the shutdown budget MUST NOT hang the process; a partial export
  is acceptable, a hung CLI is not" (FR-007).
- **Rationale**: ties the two timeouts together so the *one* budget the operator
  controls bounds both the synchronous export and the shutdown flush.
- **Alternatives considered**: (a) leave exporter defaults and rely only on the
  shutdown timeout — rejected: the shutdown timeout does not bound the
  `End()`-time export. (b) move export off the `End()` path with a batcher to
  regain ctx control — rejected: that *is* the async path D3 rejects.

### D5 — Env-gated exporter selection; honor the endpoint's URI scheme

- **Decision**: Resolve once at startup from the environment (no global mutable
  state — FR-012):
  - `OTEL_EXPORTER_OTLP_ENDPOINT` non-empty → build an **OTLP HTTP** exporter
    (`otlptracehttp.New`). Delegate endpoint/scheme/path semantics to the
    canonical exporter, which maps a `http://` endpoint to plaintext transport and
    an `https://` endpoint to **verified** TLS by default; we pass only the
    bounding options (D4) and the resource (D8). We **never** pass
    `WithInsecure()` blindly nor any `tls.Config{InsecureSkipVerify: true}` —
    plaintext (no TLS) is permitted, *unverified* TLS is not (FR-004, spec
    clarification 2026-06-10).
  - `AX_OTEL_DEBUG` truthy → build a **stderr debug** exporter (D6).
  - Both set → register **both** processors; both destinations receive the run's
    spans, neither suppresses the other (edge case).
  - Neither set → **no-op**: the provider gets no processor/exporter; the root
    span still provides a valid context for correlation, but nothing is exported
    and `stdout`/exit code are byte-for-byte unaffected (FR-005, SC-003).
- **Scope boundary**: only `OTEL_EXPORTER_OTLP_ENDPOINT` (gate) + `AX_OTEL_DEBUG`
  are this feature's contract. The broader standard OTel env surface
  (`…_HEADERS`, `…_TIMEOUT`, `…_TRACES_ENDPOINT`, per-signal/protocol selection,
  sampling) is **out of scope** (spec Assumptions); whatever otlptracehttp honors
  natively is incidental — neither tested nor forbidden — and a possible future
  addition.
- **Rationale**: matches ADR-0005's exporter contract (no-op default, OTLP HTTP
  on the standard endpoint var, opt-in stderr debug) and delegates the fiddly
  scheme/path/TLS rules to the package built for them, keeping our surface tiny.
- **Alternatives considered**: (a) OTLP **gRPC** default — rejected: ADR-0005
  picks HTTP (firewall-friendlier, simplest collector setup); gRPC is a future
  addition. (b) parse the endpoint ourselves and choose `WithInsecure` vs
  `WithTLSClientConfig` by scheme — rejected: re-implements standard semantics the
  exporter already encodes and risks divergence; the gate-on-env + delegate
  approach is smaller and equally deterministic under test.

### D6 — Debug exporter writes human-readable spans to the configured `stderr`, race-safely

- **Decision**: When `AX_OTEL_DEBUG` is set, build
  `stdouttrace.New(stdouttrace.WithWriter(w))` where `w` is the **configured
  `stderr`** (threaded in via a new `WithTelemetryStderr` option that `Execute`
  wires from `cfg.stderr`), never `os.Stdout` and never the package default. The
  writer is wrapped in a small mutex-synchronized `io.Writer` so concurrent debug
  exports and `zerolog` lines (which share the same `stderr`) cannot interleave
  bytes or trip the race detector when the sink is a shared buffer in tests.
- **Rationale**: FR-006 (human-readable span data to `stderr` only, strictly
  opt-in), FR-009 (never `stdout`), FR-013/SC-007 (race-clean). In production both
  the logger and the debug exporter write to `os.Stderr` (concurrent
  `*os.File.Write` is safe); the synchronized wrapper makes the *test* sink (a
  shared `bytes.Buffer`) safe too, so the `-race` guarantee holds end to end.
- **Alternatives considered**: (a) write debug spans straight to `os.Stderr` —
  rejected: untestable for stream separation and unsafe when tests share a buffer
  with the logger. (b) a JSON-to-`stderr` custom stringifier — rejected:
  `stdouttrace` is the canonical, pretty-printing option (D9).

### D7 — Telemetry is fail-open; remove the fail-closed `ExitInternal` branch

- **Decision**: Telemetry setup never fails the command. Exporter-construction
  failure, a malformed endpoint, or any setup error degrades to the **no-op path**
  with a single `stderr` diagnostic (the existing `ax: …` diagnostic style, e.g.
  `ax: otel exporter disabled: <reason>`), and execution continues with a
  recording root span (correlation still works, export is off). `StartTelemetry`
  is reshaped to return a usable `*Telemetry` (possibly no-op) and to emit its
  diagnostic to the configured `stderr`; `Execute()` **drops** the current
  `if telemetryErr != nil { WriteError(...); return ExitInternal }` block. No
  telemetry diagnostic is an `ax.Error` on `stdout`; the command's `stdout` and
  exit code are untouched (FR-008, SC-006; edge case "malformed telemetry
  configuration").
- **Rationale**: "Telemetry is instrumentation on the brake — it must never break
  the engine" (spec). The current behavior turns a mistyped endpoint into a failed
  command, the opposite of fail-open.
- **Alternatives considered**: keep returning a fatal error and have `Execute`
  swallow it — rejected: the fail-open contract belongs in the telemetry
  constructor so any caller (not just `Execute`) inherits it; surfacing then
  swallowing invites a future caller to re-introduce fail-closed.

### D8 — Minimal resource identity (`service.name`, `service.version`)

- **Decision**: Attach a `resource.Resource` built with
  `semconv.ServiceName(name)` + `semconv.ServiceVersion(version)`, where `name`
  is the CLI name (`root.Name()`) and `version` is the build-injected
  `WithVersion` value, both threaded into `StartTelemetry` via new options
  (`WithTelemetryServiceName`, `WithTelemetryServiceVersion`). `resource` and
  `semconv` are subpackages of the already-present `otel/sdk` + `otel` modules —
  **no new dependency**.
- **Rationale**: spans without `service.name` are nearly useless to a collector;
  these two attributes are low-cardinality and PII-free, satisfying the spec's
  "no PII/secrets/high-cardinality resource IDs as labels" constraint while making
  the export meaningful. Build-injected version (spec-002) flows through to the
  trace.
- **Alternatives considered**: no resource (default `unknown_service`) — rejected:
  loses the single most useful span attribute for free; richer auto-detected
  resource attributes — rejected: out of scope and risks host/PID cardinality.

### D9 — Two new OTel-canonical exporter dependencies (FR-014)

- **Decision**: Add `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
  v1.44.0 (pulling transitive `…/otlptrace`) and
  `go.opentelemetry.io/otel/exporters/stdout/stdouttrace` v1.44.0, **version-locked
  in lockstep** with the existing `go.opentelemetry.io/otel/sdk` v1.44.0 (both
  publish a matching v1.44.0; verified available).
- **Rationale (dependency-minimalism gate, Principle X / FR-014)**: stdlib does
  not speak OTLP; the existing dep set (sdk, otelhttp, otelgrpc) provides the
  pipeline but no *exporter*. Hand-writing an OTLP/protobuf-over-HTTP encoder or a
  span pretty-printer is non-trivial, exactly what these canonical packages exist
  to provide, and the spec Assumptions anticipate "the OTel-canonical exporter
  package(s)." Recorded here as a **governed addition** (no new ADR).
- **Alternatives considered**: (a) OTLP gRPC exporter (`otlptracegrpc`) instead of
  HTTP — rejected per D5/ADR-0005. (b) a bespoke `stderr` span encoder to avoid
  `stdouttrace` — rejected: more code, less standard, no benefit.

### D10 — ADR-0004 is absorbed for the record but its file is **retained**

- **Decision**: Transcribe ADR-0004's decision/alternatives/consequences into
  §Decision Records Absorbed (below), but **do not delete** `docs/adr/0004-trace-id-format.md`
  in this feature. Only ADR-0005 is retired here.
- **Rationale**: the spec leaves ADR-0004's retirement as a planning
  determination ("retired only if it solely governs this feature"). ADR-0004 does
  **not** solely govern this feature: its W3C-format decision is already elevated
  to constitution Principle VIII, and it governs ADR-0002 (error-envelope
  `trace_id`), ADR-0007 (ID strategy), and `stdout` metadata trace fields — all
  beyond this feature's scope. Deleting it here would force edits across unrelated
  surfaces and overreach the feature. Recording it satisfies the Governance intent
  (the decision is preserved) without premature deletion.
- **Alternatives considered**: delete ADR-0004 now — rejected: it is cross-cutting
  and not yet fully superseded by a single feature; its eventual retirement
  belongs to a feature that owns the error-envelope/ID surfaces it also governs.

### D11 — No new ax-go package-level mutable state

- **Decision**: All telemetry configuration enters through the `StartTelemetry`
  constructor and its functional options; nothing is stored at ax-go package
  scope and no `init()` is added. The OTel SDK *global* provider/propagator are
  set once from inside `StartTelemetry` (`otel.SetTracerProvider`,
  `otel.SetTextMapPropagator`) — the SDK's documented integration model and
  already the existing scaffold's behavior.
- **Rationale**: Principle X forbids ax-go mutable package-level state and
  config-reading `init()`. Setting the SDK's own global from a constructor with
  env-derived config is neither: it is a one-time wiring call on a third-party
  global, consistent with how every OTel program installs its provider.
- **Alternatives considered**: pass the `TracerProvider` explicitly to every
  consumer instead of setting the global — rejected: breaks `otel.Tracer(...)` and
  the `otelhttp`/`otelgrpc` helpers that read the global propagator, and diverges
  from the established scaffold for no isolation benefit in a single-process CLI.

## Decision Records Absorbed

> Constitution §Governance requires every governing ADR's decision, considered
> alternatives, and consequences to be transcribed here **before** the ADR file is
> deleted. ADR-0005 is deleted by `tasks.md`'s final task; ADR-0004 is recorded
> here but **retained** (D10).

### ADR-0005 — OpenTelemetry SDK Integration (ACCEPTED 2026-05-28) — RETIRED by this feature

**Context**: ax-go CLIs are short-lived processes. The standard OTel SDK
lifecycle (`BatchSpanProcessor` + async exporter) was designed for long-running
servers and silently drops telemetry if the process exits before the batch flush
completes. The base package must orchestrate four things: (1) trace-context
extraction from the inbound environment; (2) trace-ID correlation into ZeroLog
output; (3) forced flush of pending spans before process exit; (4)
auto-propagation of trace context on outbound HTTP/gRPC calls.

**Decision drivers**: zero telemetry loss for short-lived processes; every log
line correlates to its trace/span via stable fields; outbound calls inherit the
trace context without per-call wiring; the exporter target is configurable but
has a sensible default.

**Architecture (per ADR-0004 W3C)**:

1. *Context extraction* — at startup, parse `TRACEPARENT` from the environment
   with OTel's W3C propagator; absent it, the SDK `IdGenerator` creates a new root
   context. *(Implemented in `internal/telemetry.Start`; this feature adds the
   recording root span on top — D1.)*
2. *ZeroLog correlation hook* — a `tracingHook` reads `SpanContextFromContext`
   off the event context and stamps `trace_id`/`span_id` when a span is active.
   *(Implemented in `logger.go`; this feature makes those IDs non-zero by ensuring
   a span is always active — D1/D2.)*
3. *Flush-on-exit* — `ax.Execute()` defers a forced `TracerProvider.Shutdown`
   with a short timeout. *(Implemented; this feature makes the flush export real
   spans via `SimpleSpanProcessor` and bounds the exporter by the same budget —
   D3/D4.)*
4. *Outbound propagation* — `ax.HTTPClient()` (otelhttp transport) and
   `ax.GRPCDial()` (otelgrpc handler). *(Implemented in `http.go`; the root span
   makes the propagated context meaningful — FR-011.)*

**Considered options for the default exporter**:

- **A. OTLP HTTP** (default `http://localhost:4318`) — simplest setup; works with
  most collectors and SaaS backends; HTTP overhead per flush.
- **B. OTLP gRPC** (default `localhost:4317`) — lower overhead, OTel-native;
  needs gRPC tooling, firewall-unfriendly in some networks.
- **C. No-op unless configured** (env-gated A) — zero footprint by default,
  opt-in; risk of a silent gap if an operator assumes telemetry is on.
- **D. Stdout exporter (debug only)** — emits spans to `stderr` as JSON; for
  development, not production.

**Decision**: default exporter is **Option C** (no-op) unless
`OTEL_EXPORTER_OTLP_ENDPOINT` is set, in which case the base auto-configures
**Option A** (OTLP HTTP) targeting that endpoint. **Option D** is available via
`AX_OTEL_DEBUG=1` for local development. Rationale: zero footprint by default;
opt-in by setting the standard OTel env var that orchestrators/collectors already
use.

**Consequences**: ax-go takes dependencies on `otel/sdk`, `otelhttp`, `otelgrpc`,
and the chosen exporter package(s) *(this feature adds `otlptracehttp` +
`stdouttrace` — D9)*; the flush-on-exit pattern must be in the canonical example
so consumers do not lose spans; the ZeroLog hook is part of `ax.NewLogger()`
(consumers should not construct loggers directly); `ax.HTTPClient()` /
`ax.GRPCDial()` are the supported way to make outbound calls.

**This feature's deltas to ADR-0005's decisions** (recorded so the retired ADR's
intent stays traceable): (i) the exporter is wired through a **synchronous**
`SimpleSpanProcessor`, not the batch processor the ADR's Context warns against —
making the "zero loss" driver concrete (D3); (ii) the sampler is pinned to
`AlwaysSample()` so export does not defer to an inbound `flags=00` (D2, per the
2026-06-10 clarification); (iii) transport follows the endpoint scheme —
plaintext `http://` permitted, `https://` verified, never unverified TLS (D5, per
the 2026-06-10 clarification); (iv) setup is fail-open (D7).

**Retirement note**: `tasks.md`'s final task deletes
`docs/adr/0005-otel-integration.md` and updates its **7 references**:
`ROADMAP.md:40` (#2 line — mark done, drop/redirect the `(ADR-0005)` tag) and
`ROADMAP.md:165` (the "no-op scaffold (ADR-0005, partial)" line — resolve to
done); and the sibling-ADR cross-references in `docs/adr/0004-trace-id-format.md:61`,
`docs/adr/0007-id-strategy.md:35`, `docs/adr/0008-cli-framework-cobra.md:70`, and
`docs/adr/0009-logger-zerolog.md:11,64`. Per the precedent set retiring ADR-0010
(feature 001), editing a sibling frozen ADR **solely to remove/redirect a dangling
cross-reference** to a deleted ADR is reference hygiene, not a substantive ADR
edit — redirect each to constitution §VIII or `specs/004-real-otel-export/`. **No
Go source file references ADR-0005 by number**, so no doc-comment edits are
required for retirement.

### ADR-0004 — Trace ID Format (ACCEPTED 2026-05-28) — absorbed, file RETAINED (D10)

**Context**: distributed tracing needs unique IDs that interop with the broader
ecosystem (Tempo, Jaeger, Honeycomb, Datadog). An orchestrator running an ax-go
CLI may already have a trace open and pass it via the W3C `TRACEPARENT` env var.
The format choice is constrained by what backends accept and what the OTel SDK
generates.

**Decision drivers**: W3C Trace Context compatibility; zero-friction integration
with OTel-compatible backends; avoid implementing ID generation when a
battle-tested library exists; ASCII-safe, greppable IDs.

**Considered options**: **A.** W3C Trace Context via the OTel SDK (16-byte
`trace_id`, 8-byte `span_id`, hex) — zero implementation cost, direct interop.
**B.** UUID (v4/v7) with hyphens stripped to 16-byte hex — familiar but needs
conversion at the OTel boundary, no backend benefit. **C.** ULID — time-sortable
but needs Base32→hex conversion, no backend benefit.

**Decision**: adopt **Option A** — W3C format generated by OTel's native
`IdGenerator`; `trace_id` is 32 lowercase hex chars, `span_id` is 16. UUIDs/ULIDs
remain valid for **non-trace** identifiers (resource IDs, idempotency keys) — see
ADR-0007.

**Consequences**: direct dependency on `otel/sdk`; `TRACEPARENT` is the canonical
inbound trace-context channel (full integration in ADR-0005); all log lines,
error envelopes, and `stdout` metadata carry `trace_id` (and `span_id` where
appropriate) from the active OTel span context; when no span is active, fields
carry the zero-value valid hex strings (`ZeroTraceID`/`ZeroSpanID`) so parsers do
not branch on absence — the documented fallback this feature preserves (spec edge
case "No active span (defensive)").

**Why retained (D10)**: this decision is already constitution Principle VIII and
governs ADR-0002/0007 and `stdout` metadata beyond this feature; it is recorded
here for traceability but its file is not deleted by `004`. Its zero-value
fallback (`ZeroTraceID`/`ZeroSpanID` in `trace.go`) is explicitly preserved.

## Resolved unknowns (from the plan's Technical Context)

| Unknown | Resolution |
|---|---|
| Span processor mechanism | `SimpleSpanProcessor` (synchronous) — D3 |
| Sampler | explicit `AlwaysSample()` (override the `ParentBased` default) — D2 |
| OTLP exporter package | `otlptracehttp` v1.44.0, HTTP, scheme-honoring TLS — D5/D9 |
| Debug exporter package | `stdouttrace` v1.44.0 → synchronized `stderr` writer — D6/D9 |
| Endpoint env var (gate) | `OTEL_EXPORTER_OTLP_ENDPOINT`; debug var `AX_OTEL_DEBUG` — D5 |
| Flush-budget binding | exporter timeout derived from the shutdown budget; retry off — D4 |
| Setup-failure policy | fail-open, `stderr` diagnostic, no-op fallback — D7 |
| New dependencies | two OTel-canonical exporters, lockstep v1.44.0 — D9 (FR-014) |
| Span attributes | `service.name` + `service.version` resource only — D8 |
| ADR retirement | ADR-0005 deleted (final task); ADR-0004 retained — D10 |
