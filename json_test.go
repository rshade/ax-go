package ax

import (
	"bytes"
	"context"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"
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

// pinnedEnvelopeContext builds a context with fixed, non-zero trace/span IDs and
// a fixed idempotency key so envelope serialization is byte-deterministic
// through existing seams only (specs/003 research D4; FR-008 forbids harness
// normalization). Non-zero values are required: span_id and idempotency_key are
// omitempty and would vanish from a zero-context fixture.
func pinnedEnvelopeContext(t *testing.T, idempotencyKey string) context.Context {
	t.Helper()

	traceID, err := oteltrace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		t.Fatalf("TraceIDFromHex returned error: %v", err)
	}
	spanID, err := oteltrace.SpanIDFromHex("0102030405060708")
	if err != nil {
		t.Fatalf("SpanIDFromHex returned error: %v", err)
	}
	ctx := oteltrace.ContextWithSpanContext(
		context.Background(),
		oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
			TraceID: traceID,
			SpanID:  spanID,
		}),
	)
	return WithIdempotencyKey(ctx, idempotencyKey)
}

func TestEnvelopeGolden(t *testing.T) {
	ctx := pinnedEnvelopeContext(t, "00000000-0000-4000-8000-000000000001")

	data := struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{Name: "example", Count: 1}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, NewEnvelope(ctx, data)); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/success_envelope.golden.json", buf.Bytes())
}

func TestWriteJSONLineGolden(t *testing.T) {
	ctx := pinnedEnvelopeContext(t, "00000000-0000-4000-8000-000000000002")

	data := struct {
		Item string `json:"item"`
		Seq  int    `json:"seq"`
	}{Item: "stream-record", Seq: 42}

	var buf bytes.Buffer
	if err := WriteJSONLine(&buf, NewEnvelope(ctx, data)); err != nil {
		t.Fatalf("WriteJSONLine returned error: %v", err)
	}
	assertGolden(t, "testdata/ndjson_line.golden.json", buf.Bytes())
}
