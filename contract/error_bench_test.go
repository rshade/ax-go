package contract

import (
	"context"
	"io"
	"testing"
)

// benchError builds a representative *Error: a validation failure with a
// populated Context (a few fields) and Suggestions (a few entries) — the
// realistic worst case for the marshal path, not an empty envelope
// (research.md Decision 4).
func benchError() error {
	return NewError(context.Background(), "validation_error", "config field \"timeout\" must be a positive duration",
		WithErrorContext(map[string]any{
			"field":   "timeout",
			"value":   "-5s",
			"file":    "config.json",
			"line":    42,
			"pointer": "/server/timeout",
		}),
		WithSuggestions(
			"set timeout to a positive duration, e.g. \"30s\"",
			"omit the field to use the default timeout",
		),
	)
}

// BenchmarkWriteError measures WriteError's allocation profile on the error
// envelope marshal path — every command that fails serializes one *Error to
// stderr, so this substantiates that hot path's allocation claim tracked by
// the CI performance regression budget (research.md Decision 4). Writing to
// io.Discard isolates the marshal cost from sink I/O.
func BenchmarkWriteError(b *testing.B) {
	err := benchError()

	b.ReportAllocs()
	for b.Loop() {
		_ = WriteError(io.Discard, err)
	}
}
