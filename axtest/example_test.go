package axtest_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/axtest"
)

// ExampleRun runs the toy confirmation-gated "greet" command through the real
// ax.Execute lifecycle and inspects the returned exit code and decoded
// payload. A fresh *testing.T stands in for the caller's real test object:
// Run only ever calls t.Helper() on this success path, which a zero-value
// *testing.T supports safely.
func ExampleRun() {
	tb := &testing.T{}
	root := newGreetCommand(tb)

	result := axtest.Run(context.Background(), tb, root, []string{"greet", "--format=json", "--yes"})

	var envelope ax.Envelope[greetResult]
	if err := json.Unmarshal(result.Stdout, &envelope); err != nil {
		fmt.Println("decode error:", err)
		return
	}
	fmt.Printf("exit code %d, approved %v\n", result.ExitCode, envelope.Data.Approved)
	// Output:
	// exit code 0, approved true
}

// ExampleDecode unwraps a literal envelope's data field into a plain
// caller-defined type, with no wrapper struct declared to reach it.
func ExampleDecode() {
	stdout := []byte(`{"data":{"greeting":"hi"},"meta":{"trace_id":"00000000000000000000000000000000"}}`)

	data := axtest.Decode[greetingFixtureResult](&testing.T{}, stdout)

	fmt.Println(data.Greeting)
	// Output:
	// hi
}

// ExampleRunAndDecode composes Run and Decode in one call for the
// success-path common case: no wrapper struct, and no separate Decode call.
func ExampleRunAndDecode() {
	tb := &testing.T{}
	root := newGreetCommand(tb)

	data, exitCode := axtest.RunAndDecode[greetResult](
		context.Background(), tb, root, []string{"greet", "--format=json", "--yes"},
	)

	fmt.Printf("exit code %d, approved %v\n", exitCode, data.Approved)
	// Output:
	// exit code 0, approved true
}
