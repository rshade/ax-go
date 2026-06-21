package contract

import (
	"bytes"
	"context"
	"testing"
)

func TestNewEnvelopeUsesContextMetadata(t *testing.T) {
	ctx := context.Background()
	ctx = WithMetadata(ctx, Metadata{
		TraceID: "0102030405060708090a0b0c0d0e0f10",
		SpanID:  "0102030405060708",
	})
	ctx = WithDryRun(ctx, true)
	ctx = WithIdempotencyKey(ctx, "key-1")

	got := NewEnvelope(ctx, map[string]string{"ok": "true"})
	if got.Meta.TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Fatalf("TraceID = %q, want provided trace", got.Meta.TraceID)
	}
	if got.Meta.SpanID != "0102030405060708" {
		t.Fatalf("SpanID = %q, want provided span", got.Meta.SpanID)
	}
	if got.Meta.IdempotencyKey != "key-1" {
		t.Fatalf("IdempotencyKey = %q, want key-1", got.Meta.IdempotencyKey)
	}
	if !got.Meta.DryRun {
		t.Fatal("DryRun = false, want true")
	}
}

func TestNewEnvelopeDefaultsToZeroTraceMetadata(t *testing.T) {
	got := NewEnvelope(context.Background(), "ok")
	if got.Meta.TraceID != ZeroTraceID {
		t.Fatalf("TraceID = %q, want %q", got.Meta.TraceID, ZeroTraceID)
	}
	if got.Meta.SpanID != ZeroSpanID {
		t.Fatalf("SpanID = %q, want %q", got.Meta.SpanID, ZeroSpanID)
	}
}

func TestWriteJSONAndLineEmitStrictSingleLine(t *testing.T) {
	payload := Envelope[string]{
		Data: "hello",
		Meta: Metadata{TraceID: ZeroTraceID},
	}

	var jsonOut bytes.Buffer
	if err := WriteJSON(&jsonOut, payload); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	if got := jsonOut.String(); got != `{"data":"hello","meta":{"trace_id":"00000000000000000000000000000000"}}`+"\n" {
		t.Fatalf("WriteJSON = %q", got)
	}

	var lineOut bytes.Buffer
	if err := WriteJSONLine(&lineOut, payload); err != nil {
		t.Fatalf("WriteJSONLine returned error: %v", err)
	}
	if !bytes.Equal(lineOut.Bytes(), jsonOut.Bytes()) {
		t.Fatalf("WriteJSONLine = %q, want %q", lineOut.String(), jsonOut.String())
	}
}
