package ax

import (
	"context"
	"testing"
)

func TestStartTelemetryExtractsTraceparent(t *testing.T) {
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	spanID := "00f067aa0ba902b7"

	ctx, telemetry, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(key string) string {
			switch key {
			case "TRACEPARENT":
				return "00-" + traceID + "-" + spanID + "-01"
			default:
				return ""
			}
		}),
	)
	if err != nil {
		t.Fatalf("StartTelemetry returned error: %v", err)
	}
	defer telemetry.Shutdown(context.Background())

	if got := TraceIDFromContext(ctx); got != traceID {
		t.Fatalf("TraceIDFromContext = %q, want %q", got, traceID)
	}
	if got := SpanIDFromContext(ctx); got != spanID {
		t.Fatalf("SpanIDFromContext = %q, want %q", got, spanID)
	}
}
