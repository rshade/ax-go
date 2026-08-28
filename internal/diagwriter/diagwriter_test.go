package diagwriter

import (
	"bytes"
	"context"
	"io"
	"testing"
)

// TestWithWriterRoundTrip covers the basic carry contract: a writer set via
// WithWriter is the one FromContext returns.
func TestWithWriterRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext ok = false, want true")
	}
	if got != io.Writer(&buf) {
		t.Fatalf("FromContext writer = %v, want %v", got, &buf)
	}
}

// TestFromContextAbsent covers the no-writer-carried case: a plain context
// returns ok=false rather than a zero-value writer a caller might mistake for
// a usable one.
func TestFromContextAbsent(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("FromContext ok = true for a context with no carried writer, want false")
	}
}

// TestFromContextNil covers the defensive nil-context path: FromContext must
// not panic on a nil ctx, mirroring the nil-context tolerance the rest of this
// codebase's context helpers observe.
func TestFromContextNil(t *testing.T) {
	if _, ok := FromContext(nil); ok {
		t.Fatal("FromContext ok = true for a nil context, want false")
	}
}
