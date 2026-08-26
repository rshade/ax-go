package axtest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

	ax "github.com/rshade/ax-go"
)

// Decode unmarshals stdout as an ax.Envelope[T] and returns its Data field.
// stdout need not have come from Run: Decode accepts any []byte in the
// envelope shape, which is what lets a caller decode output captured by other
// means without a second helper.
//
// Decode calls t.Helper() then t.Fatalf — never returning an error alongside
// T — when stdout is empty, is not valid JSON, or does not conform to the
// envelope shape (FR-006): the calling test fails immediately, with a message
// naming the parse or shape-mismatch cause, rather than receiving a
// zero-valued T that could be silently misread as a real (if empty) result.
func Decode[T any](t testing.TB, stdout []byte) T {
	t.Helper()

	var envelope ax.Envelope[T]
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("axtest.Decode: %v", err)
	}
	return envelope.Data
}

// RunAndDecode composes Run and Decode for the success-path common case: it
// runs root and decodes its stdout into T in one call, with no independent
// logic beyond the t.Helper() call below — its behavior can never drift from
// Run's or Decode's, because it is built from nothing else.
//
// The t.Helper() call is required, not cosmetic: without it, a Decode failure
// raised through this composition would attribute its t.Fatalf location to
// the line inside RunAndDecode that calls Decode, rather than to the caller's
// test line. testing.Helper() walks the call stack upward from the failure
// site, skipping only consecutively marked frames, and stops at the first
// frame that never called t.Helper() — so every intermediate wrapper in a
// call chain must mark itself, not just the innermost one.
//
// RunAndDecode is intended for the success path: a caller expecting a
// non-zero exit code should use Run directly, since Decode's t.Fatalf on a
// failure-shaped payload would obscure the exit code the caller actually
// wanted to assert on.
func RunAndDecode[T any](
	ctx context.Context,
	t testing.TB,
	root *cobra.Command,
	args []string,
	opts ...ax.ExecuteOption,
) (T, int) {
	t.Helper()

	result := Run(ctx, t, root, args, opts...)
	return Decode[T](t, result.Stdout), result.ExitCode
}
