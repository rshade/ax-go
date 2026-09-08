package ax

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"

	"github.com/rshade/ax-go/contract"
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

// TestWithTraceMetadataNilContextFallsBackToBackground exercises the nil-context
// guard in withTraceMetadata. A nil context.Context (supplied via a typed
// variable rather than the nil literal, so it is the real interface-nil the
// guard checks for) must not panic: the helper substitutes context.Background()
// and still returns a usable context carrying the zero-value trace and span IDs.
func TestWithTraceMetadataNilContextFallsBackToBackground(t *testing.T) {
	var nilCtx context.Context

	got := withTraceMetadata(nilCtx)
	if got == nil {
		t.Fatal("withTraceMetadata(nil) returned a nil context")
	}

	meta := contract.MetadataFromContext(got)
	if meta.TraceID != ZeroTraceID {
		t.Fatalf("TraceID = %q, want ZeroTraceID %q", meta.TraceID, ZeroTraceID)
	}
	if meta.SpanID != ZeroSpanID {
		t.Fatalf("SpanID = %q, want ZeroSpanID %q", meta.SpanID, ZeroSpanID)
	}
}

// TestMetadataFromContextInsideExecuteMatchesNewEnvelope is the regression
// test for issue #212: MetadataFromContext must be byte-for-byte the same
// composition NewEnvelope already uses, so calling it from root ax code (as
// opposed to contract.MetadataFromContext, which cannot see the active span)
// returns live trace/span IDs identical to what the command's own envelope
// would carry.
func TestMetadataFromContextInsideExecuteMatchesNewEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var viaMetadata, viaEnvelope Metadata

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			viaMetadata = MetadataFromContext(cmd.Context())
			viaEnvelope = NewEnvelope(cmd.Context(), struct{}{}).Meta
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}
	root.SetArgs([]string{"--dry-run"})

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)
	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}

	if viaMetadata != viaEnvelope {
		t.Fatalf("MetadataFromContext() = %+v, want %+v (NewEnvelope's Meta)", viaMetadata, viaEnvelope)
	}
	if viaMetadata.TraceID == ZeroTraceID {
		t.Fatal("MetadataFromContext().TraceID = ZeroTraceID, want non-zero under Execute's active root span")
	}
	if viaMetadata.SpanID == ZeroSpanID {
		t.Fatal("MetadataFromContext().SpanID = ZeroSpanID, want non-zero under Execute's active root span")
	}
	if !viaMetadata.DryRun {
		t.Fatal("MetadataFromContext().DryRun = false, want true (--dry-run was set)")
	}
	if viaMetadata.IdempotencyKey == "" {
		t.Fatal("MetadataFromContext().IdempotencyKey is empty, want an auto-generated key")
	}
}

func TestMetadataFromContextWithNoSpanReturnsZeroIDs(t *testing.T) {
	got := MetadataFromContext(context.Background())
	if got.TraceID != ZeroTraceID {
		t.Fatalf("TraceID = %q, want ZeroTraceID %q", got.TraceID, ZeroTraceID)
	}
	if got.SpanID != ZeroSpanID {
		t.Fatalf("SpanID = %q, want ZeroSpanID %q", got.SpanID, ZeroSpanID)
	}
}

// TestMetadataFromContextNilContextFallsBackToBackground mirrors
// TestWithTraceMetadataNilContextFallsBackToBackground: MetadataFromContext
// delegates to withTraceMetadata, so it inherits the same nil-context guard.
func TestMetadataFromContextNilContextFallsBackToBackground(t *testing.T) {
	var nilCtx context.Context

	got := MetadataFromContext(nilCtx)
	if got.TraceID != ZeroTraceID {
		t.Fatalf("TraceID = %q, want ZeroTraceID %q", got.TraceID, ZeroTraceID)
	}
	if got.SpanID != ZeroSpanID {
		t.Fatalf("SpanID = %q, want ZeroSpanID %q", got.SpanID, ZeroSpanID)
	}
}

// TestMetadataFromContextLiveSpanSupersedesExplicitMetadata is the
// regression test for FR-006: when a context carries both an explicitly
// stored trace/span ID (via contract.WithMetadata) and an actively executing
// span, the live span's IDs must win, exactly as NewEnvelope/NewError already
// behave.
func TestMetadataFromContextLiveSpanSupersedesExplicitMetadata(t *testing.T) {
	const explicitTraceID = "explicit-trace-id"
	const explicitSpanID = "explicit-span-id"

	ctx, tel, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(string) string { return "" }),
		WithTelemetryServiceName("metadata-precedence-test"),
	)
	if err != nil {
		t.Fatalf("StartTelemetry: %v", err)
	}
	t.Cleanup(func() {
		if err := tel.Shutdown(context.Background()); err != nil {
			t.Fatalf("Telemetry.Shutdown: %v", err)
		}
	})

	ctx = contract.WithMetadata(ctx, contract.Metadata{
		TraceID: explicitTraceID,
		SpanID:  explicitSpanID,
	})

	ctx, span := otel.Tracer("github.com/rshade/ax-go/test").Start(ctx, "precedence-op")
	defer span.End()

	got := MetadataFromContext(ctx)
	if got.TraceID == explicitTraceID {
		t.Fatal("MetadataFromContext().TraceID = explicitly stored value, want the live span's trace ID")
	}
	if got.SpanID == explicitSpanID {
		t.Fatal("MetadataFromContext().SpanID = explicitly stored value, want the live span's span ID")
	}
	if got.TraceID == ZeroTraceID {
		t.Fatal("MetadataFromContext().TraceID = ZeroTraceID, want the live span's non-zero trace ID")
	}
	if got.SpanID == ZeroSpanID {
		t.Fatal("MetadataFromContext().SpanID = ZeroSpanID, want the live span's non-zero span ID")
	}
}
