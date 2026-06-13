package ax

import (
	"context"
	"testing"
)

func FuzzTraceparentExtraction(f *testing.F) {
	f.Add("")
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	f.Add("invalid")
	f.Add("00-00000000000000000000000000000000-0000000000000000-01")

	f.Fuzz(func(t *testing.T, traceparent string) {
		ctx, telemetry, err := StartTelemetry(
			context.Background(),
			WithTelemetryEnv(func(key string) string {
				if key == "TRACEPARENT" {
					return traceparent
				}
				return ""
			}),
		)
		if err != nil {
			t.Fatalf("StartTelemetry returned error: %v", err)
		}
		t.Cleanup(func() {
			if err := telemetry.Shutdown(context.Background()); err != nil {
				t.Fatalf("Telemetry.Shutdown returned error: %v", err)
			}
		})

		if got := TraceIDFromContext(ctx); len(got) != len(ZeroTraceID) {
			t.Fatalf("TraceIDFromContext length = %d, want %d", len(got), len(ZeroTraceID))
		}
		if got := SpanIDFromContext(ctx); len(got) != len(ZeroSpanID) {
			t.Fatalf("SpanIDFromContext length = %d, want %d", len(got), len(ZeroSpanID))
		}
	})
}
