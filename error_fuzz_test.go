package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzErrorEnvelope verifies the ax.Error build -> marshal -> unmarshal
// round-trip is panic-free, exported fields round-trip, and the WithErrorCause
// chain is reachable in-process but never serialized.
//
// Note: encoding/json coerces invalid UTF-8 to the replacement rune U+FFFD, so
// byte-exact field round-trip is asserted only for valid-UTF-8 inputs.
func FuzzErrorEnvelope(f *testing.F) {
	// causeMarker is a fixed, JSON-safe sentinel prefixed onto the fuzzed cause.
	// It lets the I5 leak check detect the cause in serialized output without the
	// false positives a raw `cause` needle would hit: the only other strings in
	// the envelope are the fuzzed code/message, which can equal or contain cause.
	// The marker has no JSON-escapable characters, so its raw and escaped forms
	// are identical.
	const causeMarker = "fuzz-cause-sentinel-d3adb33f-"

	f.Add("validation_error", "bad input", "underlying cause")
	f.Add("", "", "")
	f.Add("config_invalid", "line 1\nline 2\tctrl", "decode failed")
	f.Add(strings.Repeat("c", 2048), strings.Repeat("m", 2048), strings.Repeat("u", 2048))
	f.Add("0", "\xb2", "0") // invalid UTF-8 in message — JSON-coerced to U+FFFD

	f.Fuzz(func(t *testing.T, code, message, cause string) {
		sentinel := errors.New(causeMarker + cause)
		// Derive recovery fields deterministically from the inputs so the
		// retryable/retry_after_seconds marshal path is fuzzed without changing
		// the corpus signature. retryAfter stays non-negative.
		retryable := len(code)%2 == 0
		retryAfter := int64(len(message))
		e := NewError(context.Background(), code, message,
			WithErrorCause(sentinel), WithErrorExitCode(ExitValidation),
			WithRetryable(retryable), WithRetryAfterSeconds(retryAfter))

		if !errors.Is(e, sentinel) {
			t.Fatalf("cause not reachable via errors.Is before marshal")
		}

		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// I5: the cause text must never be serialized. The marker can appear in
		// the output only via the fuzzed code/message fields; exclude those, and
		// any remaining occurrence is a genuine leak.
		if !strings.Contains(code, causeMarker) && !strings.Contains(message, causeMarker) &&
			bytes.Contains(b, []byte(causeMarker)) {
			t.Fatalf("cause text leaked into serialized JSON: %s", b)
		}
		var got Error
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// JSON coerces invalid UTF-8 to U+FFFD, so byte-exact field round-trip is
		// asserted only when the inputs are valid UTF-8.
		if utf8.ValidString(code) && got.ErrorCode != code {
			t.Fatalf("code round-trip mismatch: %q -> %q", code, got.ErrorCode)
		}
		if utf8.ValidString(message) && got.Message != message {
			t.Fatalf("message round-trip mismatch: %q -> %q", message, got.Message)
		}
		if got.SchemaVersion != e.SchemaVersion {
			t.Fatalf("schema_version %q != %q", got.SchemaVersion, e.SchemaVersion)
		}
		if got.Retryable == nil || *got.Retryable != retryable {
			t.Fatalf("retryable round-trip mismatch: %v -> %v", retryable, got.Retryable)
		}
		if got.RetryAfterSeconds != retryAfter {
			t.Fatalf(
				"retry_after_seconds round-trip mismatch: %d -> %d",
				retryAfter,
				got.RetryAfterSeconds,
			)
		}
		if got.Unwrap() != nil {
			t.Fatalf("cause must not survive serialization, got %v", got.Unwrap())
		}

		var buf bytes.Buffer
		if err := WriteError(&buf, e); err != nil {
			t.Fatalf("WriteError: %v", err)
		}
		out := buf.Bytes()
		if len(out) == 0 || out[len(out)-1] != '\n' || bytes.Count(out, []byte("\n")) != 1 {
			t.Fatalf("WriteError must emit exactly one trailing newline, got %q", out)
		}

		var nilBuf bytes.Buffer
		if err := WriteError(&nilBuf, nil); err != nil || nilBuf.Len() != 0 {
			t.Fatalf("WriteError(nil) must be a no-op, wrote %d bytes err=%v", nilBuf.Len(), err)
		}
		var typedNilBuf bytes.Buffer
		_ = WriteError(&typedNilBuf, (*Error)(nil)) // must not panic
	})
}

// FuzzErrorEnvelopeUnmarshal verifies ax.Error's json.Unmarshal path never
// panics on arbitrary bytes and that any value which unmarshals cleanly
// re-serializes to a byte-for-byte stable fixpoint (Principle II).
// A byte-level fixpoint (not struct DeepEqual) is used deliberately: omitempty
// on Suggestions/Context makes empty-vs-nil indistinguishable at the JSON level.
func FuzzErrorEnvelopeUnmarshal(f *testing.F) {
	f.Add([]byte(`{"error_code":"x","message":"m","context":{"k":1},"suggestions":["a"]}`))
	f.Add([]byte("{}"))
	f.Add([]byte(""))
	f.Add([]byte("   "))
	f.Add([]byte("}{"))
	f.Add([]byte(`{"context":{"nested":{"deep":[1,2,3]}}}`))
	f.Add([]byte(`{"error_code":1}`))                            // wrong field type — string field given a number
	f.Add([]byte(`{"error_code":"x","mess`))                     // truncated mid-key
	f.Add([]byte(`{"retryable":true,"retry_after_seconds":30}`)) // recovery fields populated
	f.Add([]byte(`{"retryable":false}`))                         // explicit non-retryable, no backoff
	f.Add([]byte(`{"retry_after_seconds":-5}`))                  // negative backoff survives the unmarshal fixpoint
	f.Add([]byte(`{"retryable":"yes"}`))                         // wrong type — bool field given a string

	f.Fuzz(func(t *testing.T, data []byte) {
		var e Error
		if err := json.Unmarshal(data, &e); err != nil {
			return
		}
		b1, err := json.Marshal(&e)
		if err != nil {
			t.Fatalf("re-marshal of cleanly unmarshalled value failed: %v", err)
		}
		var e2 Error
		if err := json.Unmarshal(b1, &e2); err != nil {
			t.Fatalf("re-unmarshal failed: %v", err)
		}
		b2, err := json.Marshal(&e2)
		if err != nil {
			t.Fatalf("second marshal failed: %v", err)
		}
		if !bytes.Equal(b1, b2) {
			t.Fatalf("serialization not idempotent:\n b1=%s\n b2=%s", b1, b2)
		}
	})
}
