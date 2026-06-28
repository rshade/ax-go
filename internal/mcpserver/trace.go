package mcpserver

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/rshade/ax-go/contract"
)

const (
	tracerName       = "github.com/rshade/ax-go/mcp"
	traceParentKey   = "traceparent"
	traceStateKey    = "tracestate"
	callSpanNameRoot = "tools/call "
)

// startCallSpan begins one span per tools/call. It continues the caller's W3C
// trace when the request carries traceparent metadata and otherwise starts a
// fresh root trace, then seeds the context with trace/span IDs so log lines and
// the result envelope emitted while serving the call carry trace_id/span_id
// (FR-025, C-18, D7).
func startCallSpan(ctx context.Context, req *sdk.CallToolRequest, toolName string) (context.Context, trace.Span) {
	ctx = extractTraceContext(ctx, req)
	ctx, span := otel.Tracer(tracerName).Start(ctx, callSpanNameRoot+toolName)

	traceID, spanID := contract.ZeroTraceID, contract.ZeroSpanID
	sc := span.SpanContext()
	if sc.HasTraceID() {
		traceID = sc.TraceID().String()
	}
	if sc.HasSpanID() {
		spanID = sc.SpanID().String()
	}
	ctx = contract.WithMetadata(ctx, contract.Metadata{TraceID: traceID, SpanID: spanID})
	return ctx, span
}

// extractTraceContext extracts W3C trace context from the request's _meta when
// present, returning ctx unchanged when no usable traceparent is supplied.
func extractTraceContext(ctx context.Context, req *sdk.CallToolRequest) context.Context {
	if req == nil || req.Params == nil {
		return ctx
	}
	meta := req.Params.Meta
	if len(meta) == 0 {
		return ctx
	}

	carrier := propagation.MapCarrier{}
	if value, ok := stringFromMeta(meta, traceParentKey); ok {
		carrier[traceParentKey] = value
	}
	if value, ok := stringFromMeta(meta, traceStateKey); ok {
		carrier[traceStateKey] = value
	}
	if len(carrier) == 0 {
		return ctx
	}
	return propagation.TraceContext{}.Extract(ctx, carrier)
}

// stringFromMeta returns a non-empty string value for key from the request
// metadata.
func stringFromMeta(meta sdk.Meta, key string) (string, bool) {
	raw, ok := meta[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}
