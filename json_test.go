package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"sort"
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

// pinnedEnvelopeContext builds a context with fixed, non-zero trace/span IDs and
// a fixed idempotency key so envelope serialization is byte-deterministic
// through existing seams only (specs/003 research D4; FR-008 forbids harness
// normalization). Non-zero values are required: span_id and idempotency_key are
// omitempty and would vanish from a zero-context fixture.
func pinnedEnvelopeContext(t *testing.T, idempotencyKey string) context.Context {
	t.Helper()
	ctx := newTraceContext(t, "0102030405060708090a0b0c0d0e0f10", "0102030405060708")
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

// TestSuccessEnvelopeUnaffectedByApproval pins FR-019: approval state is an
// error-path concern only and must never reach the success envelope. The
// existing envelope golden cannot catch a field that appears solely when
// approval is granted, because it is rendered from a context that never carries
// one, so the key set is asserted here under both approval states.
func TestSuccessEnvelopeUnaffectedByApproval(t *testing.T) {
	base := pinnedEnvelopeContext(t, "00000000-0000-4000-8000-000000000003")
	data := struct {
		Name string `json:"name"`
	}{Name: "example"}

	render := func(t *testing.T, ctx context.Context) []byte {
		t.Helper()
		var buf bytes.Buffer
		if err := WriteJSON(&buf, NewEnvelope(ctx, data)); err != nil {
			t.Fatalf("WriteJSON returned error: %v", err)
		}
		return buf.Bytes()
	}

	withoutApproval := render(t, base)
	withApproval := render(t, WithApproval(base, true))
	if !bytes.Equal(withoutApproval, withApproval) {
		t.Fatalf("approval changed the success envelope:\n without = %s\n with    = %s",
			withoutApproval, withApproval)
	}

	var decoded struct {
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(withApproval, &decoded); err != nil {
		t.Fatalf("envelope did not decode: %v", err)
	}
	gotKeys := make([]string, 0, len(decoded.Meta))
	for key := range decoded.Meta {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)

	wantKeys := []string{"idempotency_key", "span_id", "trace_id"}
	if !slices.Equal(gotKeys, wantKeys) {
		t.Fatalf("success envelope meta keys = %v, want %v", gotKeys, wantKeys)
	}
}
