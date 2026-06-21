package contract

import (
	"context"
	"testing"
)

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	if _, ok := ModeFromContext(ctx); ok {
		t.Fatal("ModeFromContext ok=true on empty context")
	}
	if DryRunFromContext(ctx) {
		t.Fatal("DryRunFromContext = true on empty context")
	}
	if _, ok := IdempotencyKeyFromContext(ctx); ok {
		t.Fatal("IdempotencyKeyFromContext ok=true on empty context")
	}

	ctx = WithMode(ctx, ModeJSON)
	ctx = WithDryRun(ctx, true)
	ctx = WithIdempotencyKey(ctx, "key-1")

	mode, ok := ModeFromContext(ctx)
	if !ok || mode != ModeJSON {
		t.Fatalf("ModeFromContext = %q, %v; want json, true", mode, ok)
	}
	if !DryRunFromContext(ctx) {
		t.Fatal("DryRunFromContext = false, want true")
	}
	key, ok := IdempotencyKeyFromContext(ctx)
	if !ok || key != "key-1" {
		t.Fatalf("IdempotencyKeyFromContext = %q, %v; want key-1, true", key, ok)
	}
}

func TestMetadataContextHelpers(t *testing.T) {
	ctx := WithMetadata(context.Background(), Metadata{
		TraceID:        "0102030405060708090a0b0c0d0e0f10",
		SpanID:         "0102030405060708",
		IdempotencyKey: "from-metadata",
		DryRun:         true,
	})

	metadata := MetadataFromContext(ctx)
	if metadata.TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Fatalf("TraceID = %q, want provided trace", metadata.TraceID)
	}
	if metadata.SpanID != "0102030405060708" {
		t.Fatalf("SpanID = %q, want provided span", metadata.SpanID)
	}
	if metadata.IdempotencyKey != "from-metadata" {
		t.Fatalf("IdempotencyKey = %q, want provided key", metadata.IdempotencyKey)
	}
	if !metadata.DryRun {
		t.Fatal("DryRun = false, want true")
	}
}
