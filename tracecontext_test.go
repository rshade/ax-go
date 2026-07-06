package ax

import (
	"context"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// The canonical W3C traceparent example IDs, reused across the suite wherever a
// test or benchmark needs a populated SpanContext.
const (
	exampleTraceIDHex = "4bf92f3577b34da6a3ce929d0e0e4736"
	exampleSpanIDHex  = "00f067aa0ba902b7"
)

// newTraceContext builds a context carrying a SpanContext with the given hex
// trace/span IDs. The hex decode happens here, so callers (tests and
// benchmarks alike) never re-derive the fixture inside a timed path.
func newTraceContext(tb testing.TB, traceHex, spanHex string) context.Context {
	tb.Helper()
	traceID, err := oteltrace.TraceIDFromHex(traceHex)
	if err != nil {
		tb.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := oteltrace.SpanIDFromHex(spanHex)
	if err != nil {
		tb.Fatalf("SpanIDFromHex: %v", err)
	}
	return oteltrace.ContextWithSpanContext(context.Background(), oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	}))
}

// activeTraceContext returns a context carrying the canonical W3C example
// SpanContext so callers exercise the populated-trace path (hex-formatted
// trace/span IDs) without re-deriving the fixture. Taking testing.TB lets both
// tests and benchmarks share it; the decode happens before any b.Loop(), so
// decode cost stays out of benchmarked paths.
func activeTraceContext(tb testing.TB) context.Context {
	tb.Helper()
	return newTraceContext(tb, exampleTraceIDHex, exampleSpanIDHex)
}
