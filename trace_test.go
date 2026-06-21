package ax

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestTraceIDFromContextWithNoSpanReturnsZeroTraceID(t *testing.T) {
	got := TraceIDFromContext(context.Background())
	if got != ZeroTraceID {
		t.Fatalf("TraceIDFromContext with no active span = %q, want ZeroTraceID %q", got, ZeroTraceID)
	}
}

func TestSpanIDFromContextWithNoSpanReturnsZeroSpanID(t *testing.T) {
	got := SpanIDFromContext(context.Background())
	if got != ZeroSpanID {
		t.Fatalf("SpanIDFromContext with no active span = %q, want ZeroSpanID %q", got, ZeroSpanID)
	}
}

func TestTraceIDFromContextWithActiveSpanIsNonZero(t *testing.T) {
	ctx, tel, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(string) string { return "" }),
		WithTelemetryServiceName("trace-id-test"),
	)
	if err != nil {
		t.Fatalf("StartTelemetry: %v", err)
	}
	t.Cleanup(func() {
		if err := tel.Shutdown(context.Background()); err != nil {
			t.Fatalf("Telemetry.Shutdown: %v", err)
		}
	})

	ctx, span := otel.Tracer("github.com/rshade/ax-go/test").Start(ctx, "trace-id-op")
	defer span.End()

	got := TraceIDFromContext(ctx)
	if got == ZeroTraceID {
		t.Fatalf("TraceIDFromContext with active span = ZeroTraceID; want non-zero trace ID")
	}
}

func TestSpanIDFromContextWithActiveSpanIsNonZero(t *testing.T) {
	ctx, tel, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(string) string { return "" }),
		WithTelemetryServiceName("span-id-test"),
	)
	if err != nil {
		t.Fatalf("StartTelemetry: %v", err)
	}
	t.Cleanup(func() {
		if err := tel.Shutdown(context.Background()); err != nil {
			t.Fatalf("Telemetry.Shutdown: %v", err)
		}
	})

	ctx, span := otel.Tracer("github.com/rshade/ax-go/test").Start(ctx, "span-id-op")
	defer span.End()

	got := SpanIDFromContext(ctx)
	if got == ZeroSpanID {
		t.Fatalf("SpanIDFromContext with active span = ZeroSpanID; want non-zero span ID")
	}
}

func TestZeroIDConstantFormats(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		wantLen  int
	}{
		{name: "ZeroTraceID is 32 hex digits (W3C trace-id)", constant: ZeroTraceID, wantLen: 32},
		{name: "ZeroSpanID is 16 hex digits (W3C parent-id)", constant: ZeroSpanID, wantLen: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.constant) != tt.wantLen {
				t.Fatalf("%s length = %d, want %d", tt.name, len(tt.constant), tt.wantLen)
			}
			for i, c := range tt.constant {
				if c != '0' {
					t.Fatalf("%s[%d] = %q, want '0'", tt.name, i, c)
				}
			}
		})
	}
}
