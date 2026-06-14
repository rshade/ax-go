package ax

import (
	"context"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func TestTelemetryDebugEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		// falsy: empty, explicit false values, whitespace variants.
		// "false" (lowercase) is the canonical switch-case literal; "False" and
		// "FALSE" verify that strings.ToLower normalises before the comparison.
		{"", false},
		{"0", false},
		{"False", false},
		{"FALSE", false},
		{"No", false},
		{"Off", false},
		{"  off  ", false},
		{"  0  ", false},
		// truthy: anything that isn't the falsy set
		{"1", true},
		{"true", true},
		{"True", true},
		{"yes", true},
		{"on", true},
		{"enabled", true},
		{"anything", true},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.value), func(t *testing.T) {
			if got := telemetryDebugEnabled(tc.value); got != tc.want {
				t.Fatalf("telemetryDebugEnabled(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestStartTelemetryExtractsTraceState(t *testing.T) {
	const (
		traceID    = "4bf92f3577b34da6a3ce929d0e0e4736"
		traceState = "rojo=00f067aa0ba902b7"
	)

	ctx, telemetry, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(key string) string {
			switch key {
			case "TRACEPARENT":
				return "00-" + traceID + "-00f067aa0ba902b7-01"
			case "TRACESTATE":
				return traceState
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

	sc := trace.SpanContextFromContext(ctx)
	if got := sc.TraceState().String(); got != traceState {
		t.Fatalf("TraceState = %q, want %q — TRACESTATE env var not wired through StartTelemetry", got, traceState)
	}
}

func TestStartTelemetryCreatesRecordingSpanWithoutCollector(t *testing.T) {
	ctx, telemetry, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(string) string { return "" }),
	)
	if err != nil {
		t.Fatalf("StartTelemetry returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := telemetry.Shutdown(context.Background()); err != nil {
			t.Fatalf("Telemetry.Shutdown returned error: %v", err)
		}
	})

	ctx, span := otel.Tracer("github.com/rshade/ax-go/test").Start(ctx, "app")
	defer span.End()

	if !span.IsRecording() {
		t.Fatal("span is not recording")
	}
	traceID := TraceIDFromContext(ctx)
	spanID := SpanIDFromContext(ctx)
	if traceID == ZeroTraceID {
		t.Fatalf("TraceIDFromContext = %q, want non-zero", traceID)
	}
	if spanID == ZeroSpanID {
		t.Fatalf("SpanIDFromContext = %q, want non-zero", spanID)
	}
	if got := TraceIDFromContext(ctx); got != traceID {
		t.Fatalf("TraceIDFromContext changed from %q to %q", traceID, got)
	}
	if got := SpanIDFromContext(ctx); got != spanID {
		t.Fatalf("SpanIDFromContext changed from %q to %q", spanID, got)
	}
}

func TestStartTelemetryAlwaysSamplesUnsampledInboundTraceparent(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	ctx, telemetry, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(key string) string {
			if key == "TRACEPARENT" {
				return "00-" + traceID + "-00f067aa0ba902b7-00"
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

	ctx, span := otel.Tracer("github.com/rshade/ax-go/test").Start(ctx, "app")
	defer span.End()

	if got := TraceIDFromContext(ctx); got != traceID {
		t.Fatalf("TraceIDFromContext = %q, want %q", got, traceID)
	}
	if !span.IsRecording() {
		t.Fatal("span is not recording for unsampled inbound TRACEPARENT")
	}
	if !trace.SpanContextFromContext(ctx).IsSampled() {
		t.Fatal("span context is not sampled for unsampled inbound TRACEPARENT")
	}
}

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
