# Contract: Telemetry & Span Lifecycle API

**Feature**: `004-real-otel-export` | **Date**: 2026-06-11

The public contract this feature exposes/changes in package `ax`, plus the
externally-observable behavioral contract agents and operators depend on.
Renaming a frozen identifier is a breaking change (Constitution Principle VI).

## Exported surface (package `ax`, module root)

```go
// Telemetry owns the OTel provider lifecycle for a short-lived CLI process.
// Shape unchanged. Shutdown flushes and shuts down the provider, bounded by the
// caller's ctx (the existing shutdown budget); it is a no-op on a nil/no-op
// Telemetry.
type Telemetry struct {
    TracerProvider *sdktrace.TracerProvider
}

// TelemetryOption configures StartTelemetry. Functional options; no global state.
type TelemetryOption func(*telemetryConfig)

// WithTelemetryEnv sets the environment lookup used for config + trace extraction.
// (existing)
func WithTelemetryEnv(env func(string) string) TelemetryOption

// WithTelemetryStderr sets the writer for the AX_OTEL_DEBUG span exporter and for
// fail-open telemetry diagnostics. Defaults to os.Stderr. NEW. Never stdout.
func WithTelemetryStderr(w io.Writer) TelemetryOption

// WithTelemetryServiceName sets the OTel resource service.name (low-cardinality,
// no PII). Typically the CLI binary name. NEW.
func WithTelemetryServiceName(name string) TelemetryOption

// WithTelemetryServiceVersion sets the OTel resource service.version, typically
// the build-injected version. NEW.
func WithTelemetryServiceVersion(version string) TelemetryOption

// WithTelemetryShutdownBudget sets the budget from which the OTLP exporter's
// per-attempt timeout is derived, bounding the synchronous export so a stuck
// collector cannot hang the process. Defaults to the Execute shutdown timeout
// (2s). NEW.
//
// This is the StartTelemetry-layer counterpart of the existing Execute-layer
// option WithTelemetryShutdownTimeout: Execute wires this budget from its
// cfg.shutdownTimeout value (see T005), so the two names describe the same
// budget at different layers, not competing timeouts.
func WithTelemetryShutdownBudget(d time.Duration) TelemetryOption

// StartTelemetry installs W3C trace propagation, extracts an inbound TRACEPARENT,
// and configures the tracer provider's exporter(s) from the environment:
//
//   - OTEL_EXPORTER_OTLP_ENDPOINT set → OTLP HTTP exporter targeting it, using a
//     synchronous SimpleSpanProcessor and an AlwaysSample sampler. The endpoint's
//     URI scheme is honored: a plaintext http:// endpoint is permitted; an
//     https:// endpoint uses verified TLS. TLS verification is never disabled.
//   - AX_OTEL_DEBUG truthy → a human-readable span exporter writing to the
//     configured stderr (never stdout); may run alongside the OTLP exporter.
//   - neither set → a no-op: a recording root span still provides a valid context
//     for log correlation, but nothing is exported and the command's stdout, exit
//     code, and observable behavior are byte-for-byte unaffected.
//
// StartTelemetry is FAIL-OPEN: a malformed endpoint or a failed exporter
// construction degrades to the no-op path with a single stderr diagnostic and
// never fails the command. It returns a usable *Telemetry (possibly no-op) and a
// context carrying any extracted inbound trace context.
//
// The trailing error return is retained for signature stability (StartTelemetry
// is a frozen identifier; dropping it would be a breaking change) and is always
// nil under the fail-open contract. Callers such as Execute discard it
// (ctx, t, _ := StartTelemetry(...)).
func StartTelemetry(ctx context.Context, opts ...TelemetryOption) (context.Context, *Telemetry, error)

// Shutdown flushes pending spans and shuts the provider down, bounded by ctx.
func (t *Telemetry) Shutdown(ctx context.Context) error

// (unchanged, referenced by this contract)
const ZeroTraceID = "00000000000000000000000000000000"
const ZeroSpanID  = "0000000000000000"
func TraceIDFromContext(ctx context.Context) string
func SpanIDFromContext(ctx context.Context) string
func HTTPClient() *http.Client
func GRPCDial(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)
```

**Behavior change (fail-open)**: `StartTelemetry` no longer surfaces exporter/
endpoint problems as fatal errors, and `Execute()` **removes** its prior
`telemetryErr != nil → ExitInternal` branch. A telemetry misconfiguration is a
`stderr` diagnostic, not a failed command (FR-008). This is a governed
runtime-behavior change recorded in `research.md` (D7), not a new ADR.

**Environment contract (externally observable)**:

| Variable | Effect | Notes |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | enables OTLP HTTP export to the endpoint | scheme-honoring TLS; the *only* exporter-selection var in scope |
| `AX_OTEL_DEBUG` | enables human-readable span output on `stderr` | strictly opt-in; absent → no span data anywhere |
| `TRACEPARENT` / `TRACESTATE` | continues an inbound W3C trace | existing W3C extraction; root span inherits `trace_id` |

Out of scope (neither guaranteed nor forbidden): the broader standard OTel env
surface — `OTEL_EXPORTER_OTLP_HEADERS`, `…_TIMEOUT`, `…_TRACES_ENDPOINT`,
protocol/per-signal selection, and sampling vars (spec Assumptions, research D5).

## Behavioral contract (maps to FR / SC)

| ID | Guarantee | Verified by |
|----|-----------|-------------|
| FR-001 / SC-001 | A recording root span is active for the whole command; logs carry non-zero `trace_id`/`span_id` even with no inbound `TRACEPARENT`. | correlation test: run with empty env, assert `stderr` log line IDs are non-zero and equal across lines and to the active span |
| FR-002 / SC-008 | With an inbound `TRACEPARENT` the root span continues that trace (`trace_id` equals inbound); it records & exports even when `flags=00`. | continuity test + unsampled-parent test (assert export happens despite `flags=00`) |
| FR-003 | Every log line emitted while the span is active carries that span's IDs. | logger test over `cmd.Context()` |
| FR-004 | OTLP exporter auto-configured when the endpoint var is set; `http://` plaintext permitted, `https://` verified TLS; verification never disabled. | export test to an `http://` httptest receiver; assert no `InsecureSkipVerify` path exists |
| FR-005 / SC-003 | No endpoint configured → no exporter footprint; `stdout` byte-identical, exit code unchanged. | re-run existing `stdout` goldens with empty env; no-op assertion |
| FR-006 / SC-005 | `AX_OTEL_DEBUG` → human-readable span data on `stderr` only; strictly opt-in. | debug test: span text on `stderr`, nothing on `stdout`; absent-var test asserts silence |
| FR-007 / SC-002 | Pending spans flushed before exit via a synchronous path bounded by the shutdown budget; stuck collector never hangs. | receiver observes the span before exit; stuck-collector test returns within the budget |
| FR-008 / SC-006 | Export/setup failures never change exit code or `stdout`; degrade to a `stderr` diagnostic. | malformed-endpoint test; unreachable-collector test (exit code + `stdout` unchanged) |
| FR-009 / SC-004 | No telemetry bytes ever reach `stdout` in any mode. | stream-separation assertion across no-op/OTLP/debug |
| FR-010 / SC-003 | The root span does not make any `stdout` payload non-deterministic. | existing envelope/schema goldens unchanged |
| FR-011 | Outbound calls via `HTTPClient`/`GRPCDial` propagate the active root span. | otelhttp round-trip test asserting the active `trace_id` in outbound headers |
| FR-012 | Telemetry config derived from env at startup; no mutable package-level state. | construction via options; `go vet`/review |
| FR-013 / SC-007 | Exporter + concurrent logging + flush are race-clean. | `go test -race` over concurrent log/export/shutdown |
| FR-014 | New deps justified + recorded as a governed addition. | `research.md` D9; `go.mod` review |

## Frozen vs. non-frozen

- **Frozen identifiers**: `Telemetry`, `StartTelemetry`, `TelemetryOption`,
  `WithTelemetryEnv`, the new `WithTelemetry*` options, `ZeroTraceID`,
  `ZeroSpanID`, `TraceIDFromContext`, `SpanIDFromContext`, `HTTPClient`,
  `GRPCDial`. Renaming any is a breaking change.
- **Frozen env contract**: `OTEL_EXPORTER_OTLP_ENDPOINT` (gate) and
  `AX_OTEL_DEBUG` (debug) names and effects.
- **Non-frozen / non-deterministic**: `trace_id`/`span_id` values (documented
  non-deterministic — Principle II); the exact wording of `stderr` diagnostics;
  the root span's name/attribute details; debug-exporter formatting.

## Out of scope (delegated / deferred)

- OTLP **gRPC** exporter, alternate protocols, per-signal exporters — future
  addition (research D5, ADR-0005 absorbed).
- **Sampling configuration** — fixed `AlwaysSample()`; per-run tuning is a future
  addition (research D2).
- **Metrics / logs signals** — traces only this feature.
- The broader standard OTel env surface — see the environment-contract note above.
