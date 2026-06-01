package ax

import (
	"context"
	"testing"
)

func TestNewEnvelopeUsesContextMetadata(t *testing.T) {
	ctx := context.Background()
	ctx = WithDryRun(ctx, true)
	ctx = WithIdempotencyKey(ctx, "key-1")

	got := NewEnvelope(ctx, map[string]string{"ok": "true"})
	if got.Meta.TraceID != ZeroTraceID {
		t.Fatalf("TraceID = %q, want %q", got.Meta.TraceID, ZeroTraceID)
	}
	if got.Meta.IdempotencyKey != "key-1" {
		t.Fatalf("IdempotencyKey = %q, want key-1", got.Meta.IdempotencyKey)
	}
	if !got.Meta.DryRun {
		t.Fatal("DryRun = false, want true")
	}
}
