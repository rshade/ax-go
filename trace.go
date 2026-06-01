package ax

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

const (
	// ZeroTraceID is a valid zero-value W3C trace ID for no-active-span cases.
	ZeroTraceID = "00000000000000000000000000000000"
	// ZeroSpanID is a valid zero-value W3C span ID for no-active-span cases.
	ZeroSpanID = "0000000000000000"
)

// TraceIDFromContext returns the active W3C trace ID or ZeroTraceID.
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ZeroTraceID
}

// SpanIDFromContext returns the active W3C span ID or ZeroSpanID.
func SpanIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasSpanID() {
		return sc.SpanID().String()
	}
	return ZeroSpanID
}

// traceIDs returns the trace and span IDs from a single span-context lookup,
// for callers that need both and would otherwise extract the context twice.
func traceIDs(ctx context.Context) (string, string) {
	sc := trace.SpanContextFromContext(ctx)
	traceID, spanID := ZeroTraceID, ZeroSpanID
	if sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	if sc.HasSpanID() {
		spanID = sc.SpanID().String()
	}
	return traceID, spanID
}
