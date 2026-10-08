package contract

import (
	"bytes"
	"testing"
)

func TestWithWarningsPreservesOrderAndDropsBlanks(t *testing.T) {
	env := NewEnvelope(t.Context(), map[string]string{"status": "ok"})
	got := WithWarnings(env,
		Warning{Code: " ", Message: "blank code"},
		Warning{Code: "first", Message: "one"},
		Warning{Code: "second", Message: ""},
		Warning{Code: "third", Message: "three"},
	)

	if len(got.Warnings) != 2 {
		t.Fatalf("warnings = %#v, want 2 kept", got.Warnings)
	}
	if got.Warnings[0] != (Warning{Code: "first", Message: "one"}) ||
		got.Warnings[1] != (Warning{Code: "third", Message: "three"}) {
		t.Fatalf("warnings = %#v, want caller order with blanks dropped", got.Warnings)
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("original envelope warnings = %#v, want unchanged", env.Warnings)
	}

	var first, second bytes.Buffer
	if err := WriteJSON(&first, got); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if err := WriteJSON(&second, WithWarnings(NewEnvelope(t.Context(), map[string]string{"status": "ok"}),
		Warning{Code: "first", Message: "one"},
		Warning{Code: "third", Message: "three"},
	)); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("payloads differ\n%s\n%s", first.Bytes(), second.Bytes())
	}

	empty := WithWarnings(NewEnvelope(t.Context(), "x"))
	var buf bytes.Buffer
	if err := WriteJSON(&buf, empty); err != nil {
		t.Fatalf("WriteJSON empty: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("warnings")) {
		t.Fatalf("empty warnings were serialized: %s", buf.Bytes())
	}
}
