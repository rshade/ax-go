package ax

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	"github.com/rshade/ax-go/contract"
)

const (
	// ZeroTraceID is a valid zero-value W3C trace ID for no-active-span cases.
	ZeroTraceID = contract.ZeroTraceID
	// ZeroSpanID is a valid zero-value W3C span ID for no-active-span cases.
	ZeroSpanID = contract.ZeroSpanID
)

// TraceIDFromContext returns the active W3C trace ID or ZeroTraceID.
func TraceIDFromContext(ctx context.Context) string {
	traceID, _ := traceIDs(ctx)
	return traceID
}

// SpanIDFromContext returns the active W3C span ID or ZeroSpanID.
func SpanIDFromContext(ctx context.Context) string {
	_, spanID := traceIDs(ctx)
	return spanID
}

// traceIDs returns the trace and span IDs from a single span-context lookup,
// for callers that need both and would otherwise extract the context twice.
// It serves withTraceMetadata, which populates contract.Metadata for the error
// envelope — a root-facade concern that has nothing to do with logging.
//
// internal/logcore carries its own identical copy for the logging hot path. The
// duplication is deliberate, not an oversight: logcore's export set is closed by
// contract (specs/017-import-isolated-logging/contracts/logcore-package.md), so
// deduplicating would mean exporting a TraceIDs helper from a package whose whole
// value is a minimal, reviewed surface — and the surface gate would then carry it
// forever, because logcore.Option makes logcore reachable from a gated public
// package.
//
// The two MUST stay behaviorally identical: same zero-value fallbacks, same
// independent HasTraceID/HasSpanID checks. That is enforced, not merely asked
// for — logging/parity_test.go emits through both surfaces under an active span
// and asserts the lines are byte-identical, so a divergence here fails a test
// rather than silently producing two different correlation stories.
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

// MetadataFromContext returns the envelope metadata ax would emit for ctx:
// the live OpenTelemetry trace and span IDs (ZeroTraceID and ZeroSpanID when
// no span is active) merged with the dry-run state and idempotency key that
// Execute stores. A trace or span ID a caller stored explicitly through
// contract.WithMetadata is superseded by the live span, exactly as NewEnvelope
// and NewError already behave.
//
// contract.MetadataFromContext cannot see the active span — this package is
// import-isolated from the OpenTelemetry SDK — and returns zero IDs for a
// root-ax context. Use this function instead when calling from root ax code.
func MetadataFromContext(ctx context.Context) Metadata {
	return contract.MetadataFromContext(withTraceMetadata(ctx))
}

func withTraceMetadata(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID, spanID := traceIDs(ctx)
	return contract.WithMetadata(ctx, contract.Metadata{
		TraceID: traceID,
		SpanID:  spanID,
	})
}
