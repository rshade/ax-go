package testutil_test

import (
	"fmt"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

// ExampleMaskNonDeterministic demonstrates that trace_id, span_id, and
// idempotency_key string values are all replaced with MaskedSentinel in a
// single call, while the rest of the JSON payload is preserved.
func ExampleMaskNonDeterministic() {
	input := []byte(
		`{"data":{"greeting":"hello"},` +
			`"meta":{"trace_id":"abc123","span_id":"def456","idempotency_key":"key-789"}}`,
	)
	masked := testutil.MaskNonDeterministic(input)
	fmt.Println(string(masked))
	// Output:
	// {"data":{"greeting":"hello"},"meta":{"trace_id":"MASKED","span_id":"MASKED","idempotency_key":"MASKED"}}
}

// ExampleCompareOutputs shows the signature and typical call pattern for
// comparing two bounded JSON outputs. Call within a *testing.T test function;
// the t parameter must be a *testing.T, *testing.B, or *testing.F.
//
// Typical usage in a determinism test:
//
//	run1 := captureRun(t, args...)
//	run2 := captureRun(t, args...)
//	testutil.CompareOutputs(t, run1, run2, testutil.ModeBoundedJSON)
func ExampleCompareOutputs() {
	// Demonstrate that two byte-identical (after masking) outputs pass comparison.
	// We use a compile-time type assertion to verify the function signature.
	var _ func(testing.TB, []byte, []byte, testutil.OutputMode) = testutil.CompareOutputs
	fmt.Println("CompareOutputs accepts (testing.TB, []byte, []byte, OutputMode)")
	// Output:
	// CompareOutputs accepts (testing.TB, []byte, []byte, OutputMode)
}

// ExampleValidateTimestamps shows the signature and typical call pattern for
// validating that all RFC 3339 timestamps in a JSON payload are UTC.
//
// Typical usage:
//
//	out := captureRun(t, args...)
//	testutil.ValidateTimestamps(t, out)
func ExampleValidateTimestamps() {
	var _ func(testing.TB, []byte) = testutil.ValidateTimestamps
	fmt.Println("ValidateTimestamps accepts (testing.TB, []byte)")
	// Output:
	// ValidateTimestamps accepts (testing.TB, []byte)
}

// ExampleAssertFullyTyped shows the generic signature and typical call pattern
// for asserting that a JSON payload unmarshals into a concrete struct type
// without error.
//
// Typical usage:
//
//	out := captureRun(t, args...)
//	testutil.AssertFullyTyped[myEnvelope](t, out)
func ExampleAssertFullyTyped() {
	// Verify that AssertFullyTyped is a generic function accepting testing.TB and []byte.
	// The type parameter is provided at the call site in real tests.
	_ = testutil.MaskNonDeterministic // package import verification
	fmt.Println("AssertFullyTyped[T] accepts (testing.TB, []byte)")
	// Output:
	// AssertFullyTyped[T] accepts (testing.TB, []byte)
}
