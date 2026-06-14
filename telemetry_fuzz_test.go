package ax

import (
	"context"
	"strings"
	"testing"
)

// FuzzTraceparentExtraction verifies trace/span-ID extraction never panics and
// always yields canonical-length IDs for arbitrary TRACEPARENT and TRACESTATE
// headers (W3C Trace Context, ADR-0004).
func FuzzTraceparentExtraction(f *testing.F) {
	f.Add("", "")
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "vendor=value")
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00", "")
	f.Add("invalid", "also-invalid")
	f.Add("00-00000000000000000000000000000000-0000000000000000-01", "k=v,k2=v2")
	// bad-hex: structurally valid length but a non-hex digit in the trace-id.
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e473g-00f067aa0ba902b7-01", "vendor=value")
	// oversized tracestate: well past the W3C 512-char guidance.
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "k="+strings.Repeat("v", 600))

	f.Fuzz(func(t *testing.T, traceparent, tracestate string) {
		ctx, telemetry, err := StartTelemetry(
			context.Background(),
			WithTelemetryEnv(func(key string) string {
				switch key {
				case "TRACEPARENT":
					return traceparent
				case "TRACESTATE":
					return tracestate
				default:
					return ""
				}
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
