package ax

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
)

// FuzzIdempotencyKey verifies that an arbitrary user-supplied idempotency key
// survives the context+envelope round-trip without mutation and that
// auto-generated keys remain valid UUID v4. There is no
// key-validation API; the fuzzed surface is the context store/retrieve plus
// envelope-marshal round-trip.
//
// Contract: an empty key is treated as ABSENT — IdempotencyKeyFromContext
// returns ok=false for "" (see context.go), matching execute.go's
// "generate a key when none was supplied" behavior. A non-empty key is returned
// byte-identically and surfaces unchanged in the envelope.
//
// Note: encoding/json coerces invalid UTF-8 to the replacement rune U+FFFD
// (documented stdlib behavior), so the byte-exact guarantee through a JSON
// round-trip holds only for valid-UTF-8 keys. The context store/retrieve is
// byte-exact for any string, including invalid UTF-8.
func FuzzIdempotencyKey(f *testing.F) {
	f.Add("550e8400-e29b-41d4-a716-446655440000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("🦀-unicode-ключ")
	f.Add(strings.Repeat("a", 4096))
	f.Add("\xf2")       // invalid UTF-8 — JSON-coerced to U+FFFD; context round-trip exact
	f.Add("\x01\n\x1b") // control chars - valid UTF-8; JSON-escaped on marshal

	f.Fuzz(func(t *testing.T, key string) {
		ctx := WithIdempotencyKey(context.Background(), key)
		got, ok := IdempotencyKeyFromContext(ctx)
		switch {
		case key == "":
			if ok || got != "" {
				t.Fatalf("empty key must be reported absent, got %q ok=%v", got, ok)
			}
		case !ok || got != key:
			t.Fatalf("non-empty key must round-trip: got %q ok=%v, want %q", got, ok, key)
		}

		// got is the canonical retrieved value ("" when absent); the envelope
		// must reflect exactly that, before and after a JSON round-trip.
		env := NewEnvelope(ctx, struct{}{})
		if env.Meta.IdempotencyKey != got {
			t.Fatalf("envelope key=%q, want %q", env.Meta.IdempotencyKey, got)
		}

		b, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		var rt Envelope[struct{}]
		if err := json.Unmarshal(b, &rt); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		// JSON coerces invalid UTF-8 to U+FFFD, so a byte-exact JSON round-trip
		// holds only for valid-UTF-8 keys; the context round-trip above is exact
		// for any string.
		if utf8.ValidString(got) && rt.Meta.IdempotencyKey != got {
			t.Fatalf("post-unmarshal key=%q, want %q", rt.Meta.IdempotencyKey, got)
		}

		gen := NewIdempotencyKey()
		parsed, err := uuid.Parse(gen)
		if err != nil {
			t.Fatalf("NewIdempotencyKey %q not a UUID: %v", gen, err)
		}
		if parsed.Version() != 4 {
			t.Fatalf("NewIdempotencyKey version=%d, want 4", parsed.Version())
		}
		if parsed.Variant() != uuid.RFC4122 {
			t.Fatalf("NewIdempotencyKey variant=%v, want RFC4122", parsed.Variant())
		}
	})
}
