# Phase 1 Data Model: Real OTel Export & Span Lifecycle

**Feature**: `004-real-otel-export` | **Date**: 2026-06-11

This feature persists no data (Principle VI — no state). The "entities" are the
runtime telemetry values resolved once per process and the spec's Key Entities,
mapped to concrete OTel SDK types and the ax-go option surface.

## Entities

### Telemetry configuration (resolved once at startup)

- **What**: the environment-derived settings that decide which exporter(s) run,
  resolved a single time inside `StartTelemetry` from the injected `env func(string) string`
  (no global mutable state — FR-012, research D11).
- **Fields**:

  | Field | Source | Type | Default |
  |---|---|---|---|
  | OTLP endpoint | `OTEL_EXPORTER_OTLP_ENDPOINT` | `string` | `""` (no-op) |
  | debug enabled | `AX_OTEL_DEBUG` (truthy) | `bool` | `false` |
  | stderr sink | `WithTelemetryStderr` (Execute wires `cfg.stderr`) | `io.Writer` | `os.Stderr` |
  | service name | `WithTelemetryServiceName` (`root.Name()`) | `string` | `""` |
  | service version | `WithTelemetryServiceVersion` (`cfg.version`) | `string` | `""` |
  | shutdown budget | `WithTelemetryShutdownBudget` (`cfg.shutdownTimeout`) | `time.Duration` | 2 s |
  | inbound `TRACEPARENT`/`TRACESTATE` | env | `string` | `""` |

- **Rules**:
  - Endpoint set → OTLP HTTP exporter; debug set → `stderr` debug exporter; both →
    both; neither → no-op (no processor/exporter). (research D5)
  - The OTLP exporter's per-attempt timeout is derived from the shutdown budget
    and retry is disabled (research D4).
  - Resolution is **fail-open**: any construction error degrades to no-op with a
    `stderr` diagnostic; it never errors the command (research D7).

### Root span

- **What**: the recording span created around the whole command in `Execute()`;
  parent of all command work and the source of the `trace_id`/`span_id` on every
  log line (spec Key Entity "Root span").
- **Type**: `go.opentelemetry.io/otel/trace.Span`, started via
  `tracer.Start(ctx, name)` on the `TRACEPARENT`-extracted context.
- **Lifecycle**: `Start` (in `Execute`, before `root.ExecuteContext`) → name
  refined to `cmd.CommandPath()` in `PersistentPreRunE` → `SetStatus` from the
  command outcome → `End()` (deferred so it runs **before** `Shutdown`).
- **Rules**:
  - Always recording (sampler `AlwaysSample()`), regardless of inbound `flags`
    (FR-002, research D2).
  - Continues the inbound remote trace when present (same `trace_id`, fresh
    `span_id`); starts a fresh trace otherwise (FR-001/FR-002).
  - Attributes are low-cardinality and PII-free; identity lives on the resource
    (`service.name`/`service.version`), not as span attributes (research D8).
  - On no-op config it still provides a valid context for correlation but its
    `End()` exports nothing (FR-005).

### Span exporter (three states)

- **What**: the configured destination for finished spans (spec Key Entity).
- **States / types**:

  | State | Trigger | Type | Destination |
  |---|---|---|---|
  | none / no-op | neither env var | *(no processor registered)* | discarded |
  | OTLP HTTP | `OTEL_EXPORTER_OTLP_ENDPOINT` | `otlptracehttp` exporter | the endpoint (scheme-honoring TLS) |
  | stderr debug | `AX_OTEL_DEBUG` | `stdouttrace` exporter | synchronized `stderr` writer |

- **Rules**: the OTLP and debug states may both be active. Neither ever writes to
  `stdout` (FR-009). TLS verification is never disabled (FR-004).

### Span processor (flush path)

- **What**: the mechanism handing finished spans to the exporter, force-flushed
  at exit (spec Key Entity).
- **Type**: `sdktrace.SimpleSpanProcessor` (one per active exporter), registered
  via `WithSpanProcessor`.
- **Rules**: exports **synchronously at `span.End()`** (research D3), so a
  completed root span is exported before `Telemetry.Shutdown` runs; the shutdown
  flush is bounded by the shutdown budget and the exporter is bounded by a derived
  timeout (research D4) so a stuck collector cannot hang the process.

### Telemetry lifecycle handle (`ax.Telemetry`)

- **What**: the public handle returned by `StartTelemetry`, owning the provider
  lifecycle for the process.
- **Type**: `ax.Telemetry{ TracerProvider *sdktrace.TracerProvider }` (shape
  unchanged); `Shutdown(ctx)` flushes + shuts down (no-op when nil/no-op).

## Public option surface (additions)

```text
WithTelemetryEnv(env func(string) string)        // existing
WithTelemetryStderr(w io.Writer)                 // NEW — debug-exporter + diagnostic sink
WithTelemetryServiceName(name string)            // NEW — resource service.name (D8)
WithTelemetryServiceVersion(version string)      // NEW — resource service.version (D8)
WithTelemetryShutdownBudget(d time.Duration)     // NEW — derives exporter timeout (D4)
```

All are functional options on `StartTelemetry` (Principle X); none introduces
package-level state. `Execute` wires them from its `executeConfig`
(`cfg.stderr`, `root.Name()`, `cfg.version`, `cfg.shutdownTimeout`).

## State transitions

```text
StartTelemetry(ctx, opts…)
   │  extract TRACEPARENT/TRACESTATE → ctx                         (existing)
   │  resolve config from env (endpoint?, debug?, budget, ids)     (D5/D11)
   │
   ├─ build exporter(s):
   │     endpoint set ─► otlptracehttp.New(timeout=budget, retry=off)  ─┐  (D4/D5)
   │     debug set    ─► stdouttrace.New(WithWriter(sync(stderr)))      ─┤  (D6)
   │     (construction error) ─► stderr diagnostic, treat as absent ────┤  FAIL-OPEN (D7)
   │     neither       ─► no exporter                                   │  (D5/FR-005)
   │                                                                    ▼
   │  provider = NewTracerProvider(WithSampler(AlwaysSample),           (D2)
   │              WithResource(service.name/version),                   (D8)
   │              WithSpanProcessor(SimpleSpanProcessor(exp))… )        (D3)
   │  otel.SetTracerProvider(provider); SetTextMapPropagator(W3C)       (D11)
   ▼
Execute:  tracer.Start(ctx, root.Name())  ──► ctx carries root span     (D1)
   │      defer Shutdown(budget)   [registered first → runs last]
   │      defer span.End()         [registered second → runs first]     (D1 ordering)
   │      PersistentPreRunE: span.SetName(cmd.CommandPath())            (D1)
   │      root.ExecuteContext(ctx)
   │         └─ logs via cmd.Context() ► tracingHook ► non-zero IDs     (FR-003/SC-001)
   │      on error: span.SetStatus(Error)                               (D1)
   ▼
span.End()  ─► SimpleSpanProcessor exports synchronously (if any)        (D3)
   │             stuck collector ► bounded by exporter timeout ► partial (D4)
   ▼
Telemetry.Shutdown(budget)  ─► ForceFlush + exporter.Shutdown (bounded)  (FR-007)
   │  collector outage at flush ► fail-open "ax: otel export failed",
   │     shared once with ExportSpans (no double-report)                 (FR-008/FR-009)
   │  other shutdown error ► sanitized "ax: otel shutdown failed" stderr  (FR-008/FR-009)
   ▼
stdout: byte-identical to pre-feature in no-op mode; zero telemetry bytes any mode (SC-003/SC-004)
```

## Non-entities (explicitly not modeled)

- **No persisted state, cache, or file** (Principle VI).
- **No per-signal/metrics/log exporters** — traces only, this feature (spec
  Assumptions).
- **No sampling configuration** — fixed `AlwaysSample()` (research D2).
- **No broader standard OTel env contract** (`…_HEADERS`, `…_TIMEOUT`,
  `…_TRACES_ENDPOINT`, protocol selection) — out of scope (research D5).
