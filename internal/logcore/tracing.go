package logcore

import (
	"context"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"github.com/rshade/ax-go/contract"
)

// Trace-correlation field names. These are payload fields, never stream labels:
// trace and span IDs are high-cardinality by construction (Constitution
// Principle VIII).
const (
	fieldTraceIDName = "trace_id"
	fieldSpanIDName  = "span_id"
)

// applyLabels attaches the non-empty label fields to a zerolog context. An empty
// field is omitted entirely rather than emitted as an empty string, so a consumer
// can distinguish "not set" from "set to empty".
func applyLabels(ctx zerolog.Context, labels Labels) zerolog.Context {
	if labels.Environment != "" {
		ctx = ctx.Str(labelFieldEnvironment, labels.Environment)
	}
	if labels.Application != "" {
		ctx = ctx.Str(labelFieldApplication, labels.Application)
	}
	if labels.Host != "" {
		ctx = ctx.Str(labelFieldHost, labels.Host)
	}
	if labels.Version != "" {
		ctx = ctx.Str(labelFieldVersion, labels.Version)
	}
	return ctx
}

// traceIDs returns the trace and span IDs from a single span-context lookup, for
// callers that need both and would otherwise extract the context twice.
//
// Absence is represented by the zero-value VALID hex constants rather than by an
// empty string or a missing field, so consumer parsers never branch on absence.
// That is a binding behavioral invariant, not a convenience.
//
// Only the OpenTelemetry trace API is used here, never the SDK: this code READS
// an existing span context and never generates an ID. That distinction is what
// keeps the isolated logging surface free of the SDK's dependency tree.
func traceIDs(ctx context.Context) (string, string) {
	sc := trace.SpanContextFromContext(ctx)
	traceID, spanID := contract.ZeroTraceID, contract.ZeroSpanID
	if sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	if sc.HasSpanID() {
		spanID = sc.SpanID().String()
	}
	return traceID, spanID
}

// tracingHook stamps trace_id and span_id onto every emitted log line so log
// output correlates with traces. It runs once per enabled event at Msg time;
// events filtered out by level never construct, so disabled logs skip the hook
// entirely.
//
// Allocation contract (verified by the logcore and root-package benchmarks, and
// asserted directly by TestNoActiveSpanPathIsAllocationFree; see this feature's
// research.md R8):
//   - No active span: the IDs are the contract package's zero-value constants, so
//     the hot path is allocation-free.
//   - Active span: TraceID.String()/SpanID.String() hex-encode each ID into a
//     fresh string — a fixed, bounded per-line cost independent of the number of
//     labels or structured fields on the line.
type tracingHook struct{}

func (tracingHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	ctx := e.GetCtx()
	if ctx == nil {
		ctx = context.Background()
	}
	traceID, spanID := traceIDs(ctx)
	e.Str(fieldTraceIDName, traceID)
	e.Str(fieldSpanIDName, spanID)
}
