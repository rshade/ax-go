# ADR-0005: OpenTelemetry SDK Integration

## Status

ACCEPTED — 2026-05-28.

## Context

ax-go CLIs are short-lived processes. The standard OTel SDK lifecycle
(`BatchSpanProcessor`, async exporter) was designed for long-running
servers and silently drops telemetry if the process exits before the
batch flush completes.

The base package must orchestrate four things:

1. Trace context extraction from the inbound environment.
2. Trace ID correlation into ZeroLog output.
3. Forced flush of pending spans before process exit.
4. Auto-propagation of trace context on outbound HTTP / gRPC calls.

## Decision Drivers

- Zero telemetry loss for short-lived CLI processes.
- Every log line correlates to its trace and span via stable fields.
- Outbound calls inherit the trace context without per-call wiring.
- Exporter target is configurable but has a sensible default.

## Architecture (per ADR-0004 W3C)

### 1. Context extraction

At startup, the base parses `TRACEPARENT` from the environment using
OTel's W3C propagator. If absent, OTel's `IdGenerator` creates a new root
context.

### 2. ZeroLog correlation hook

```go
type tracingHook struct{}

func (tracingHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
    sc := trace.SpanContextFromContext(e.GetCtx())
    if sc.HasTraceID() {
        e.Str("trace_id", sc.TraceID().String())
    }
    if sc.HasSpanID() {
        e.Str("span_id", sc.SpanID().String())
    }
}
```

Every log line emitted to `stderr` automatically carries `trace_id` and
`span_id` when a span is active.

### 3. Flush-on-exit

The `ax.Execute()` wrapper around Cobra's `Execute()` defers a forced
flush with a short timeout:

```go
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    if err := tracerProvider.Shutdown(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "ax: otel shutdown failed: %v\n", err)
    }
}()
```

### 4. Outbound propagation

The base package provides `ax.HTTPClient()` returning an `*http.Client`
with `otelhttp.Transport` already wired, and `ax.GRPCDial()` returning a
`*grpc.ClientConn` with `otelgrpc` interceptors. Consumers should use
these helpers rather than constructing clients directly.

## Considered Options for default exporter

### A. OTLP HTTP exporter (default `http://localhost:4318`)

Pros: simplest setup; works with most collectors and SaaS backends.
Cons: HTTP overhead on every flush.

### B. OTLP gRPC exporter (default `localhost:4317`)

Pros: lower overhead; the OTel-native protocol.
Cons: requires gRPC tooling; firewall-unfriendly in some networks.

### C. No-op exporter unless configured (env-gated A)

Pros: zero footprint by default; opt-in to telemetry; works in
environments without a collector.
Cons: silent gap if the operator assumes telemetry is on.

### D. Stdout exporter (debug only)

Emits spans to `stderr` as JSON. For development; not for production.

## Decision

Default exporter is **Option C** (no-op) unless
`OTEL_EXPORTER_OTLP_ENDPOINT` is set in the environment, in which case
the base auto-configures **Option A** (OTLP HTTP) targeting that
endpoint. **Option D** (stdout exporter) is available via the
`AX_OTEL_DEBUG=1` env var for local development.

Rationale: zero footprint by default; opt-in to telemetry by setting
the standard OTel env var (which orchestrators and collectors already
use).

## Consequences

- ax-go takes dependencies on `go.opentelemetry.io/otel/sdk`, `otelhttp`,
  `otelgrpc`, and the chosen exporter package(s).
- The flush-on-exit pattern must be in the canonical example so consumers
  do not lose spans.
- The ZeroLog hook is part of `ax.NewLogger()`; consumers should not
  construct loggers directly.
- The `ax.HTTPClient()` / `ax.GRPCDial()` helpers are the supported way
  to make outbound calls — using `net/http` directly bypasses propagation.
