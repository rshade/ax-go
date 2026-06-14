package ax

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// FuzzPatchConfig verifies that PatchConfig never panics for arbitrary input.
//
// It seeds the fuzzer with valid, comment-bearing, and structurally varied
// Hujson inputs so the engine explores past the outermost parser layer into
// the patch-application paths. All return paths (success, parse error, and
// patch error) are valid outcomes — only panics are failures.
func FuzzPatchConfig(f *testing.F) {
	seeds := []struct {
		existing string
		patch    string
	}{
		{
			existing: `{"name":"ax","replicas":3}`,
			patch:    `[{"op":"replace","path":"/replicas","value":5}]`,
		},
		{
			existing: `{
	// comment preserved
	"host": "localhost",
	"port": 8080,
}`,
			patch: `[{"op":"replace","path":"/port","value":9090}]`,
		},
		{
			existing: `{"a":{"b":{"c":1}}}`,
			patch:    `[{"op":"add","path":"/a/b/d","value":"new"}]`,
		},
		{
			existing: `{"items":[1,2,3]}`,
			patch:    `[{"op":"remove","path":"/items/1"}]`,
		},
		{
			existing: `{}`,
			patch:    `[]`,
		},
		{
			existing: `{"x":1}`,
			patch:    `[{"op":"test","path":"/x","value":1},{"op":"replace","path":"/x","value":2}]`,
		},
		// Invalid Hujson — parse errors are valid outcomes, not panics.
		{existing: `{`, patch: `[]`},
		{existing: ``, patch: `[]`},
		// Invalid patch documents.
		{existing: `{"a":1}`, patch: `not json`},
		{existing: `{"a":1}`, patch: `{}`},
	}

	for _, s := range seeds {
		f.Add(s.existing, s.patch)
	}

	f.Fuzz(func(t *testing.T, existing, patch string) {
		// Must not panic regardless of input; error outcome is irrelevant.
		_, _ = PatchConfig(context.Background(), strings.NewReader(existing), []byte(patch))
	})
}

// FuzzParseConfig verifies ParseConfig never panics and always classifies its
// outcome under arbitrary byte input and arbitrary read caps, with particular
// attention to the bounded-reader boundary (Principle V). Success and any
// validation error are acceptable outcomes; only a panic, an unclassified
// error, or a boundary misclassification is a failure.
func FuzzParseConfig(f *testing.F) {
	seeds := []struct {
		data     []byte
		maxBytes int64
	}{
		{[]byte(`{"a":1}`), DefaultMaxConfigBytes},
		{[]byte("{}"), 2},
		{[]byte("{}"), 1},
		{[]byte("{}"), 3},
		{[]byte(""), 0},
		{[]byte("not json"), DefaultMaxConfigBytes},
		{[]byte(`{"a":1}`), -1},
		{[]byte(`{"a":1}`), MaxConfigBytesCeiling + 1},
	}
	for _, s := range seeds {
		f.Add(s.data, s.maxBytes)
	}

	f.Fuzz(func(t *testing.T, data []byte, maxBytes int64) {
		var dst map[string]any
		err := ParseConfig(context.Background(), bytes.NewReader(data), &dst, WithMaxConfigBytes(maxBytes))
		if err == nil {
			return
		}

		var axErr *Error
		if !errors.As(err, &axErr) {
			t.Fatalf("ParseConfig returned non-*Error: %T (%v)", err, err)
		}
		// The free-function and method exit codes must agree — a cross-API check
		// assertConfigError does not make (it only asserts the free-function form).
		if got, want := ErrorExitCode(err), axErr.ExitCode(); got != want {
			t.Fatalf("ErrorExitCode=%d disagrees with envelope ExitCode=%d", got, want)
		}

		// Classify the outcome against the bounded-reader boundary, delegating the
		// code + ExitValidation assertions to the shared suite helpers.
		validCap := maxBytes >= 0 && maxBytes <= MaxConfigBytesCeiling
		switch {
		case !validCap:
			assertConfigError(t, err, "config_max_bytes_invalid")
		case int64(len(data)) > maxBytes:
			assertConfigError(t, err, "config_too_large")
		default:
			assertNotConfigTooLarge(t, err)
			if axErr.ExitCode() != ExitValidation {
				t.Fatalf("error %q exit=%d, want ExitValidation(%d)", axErr.ErrorCode, axErr.ExitCode(), ExitValidation)
			}
		}
	})
}
