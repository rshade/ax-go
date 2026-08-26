package axtest_test

import (
	"context"
	"testing"

	"github.com/rshade/ax-go/axtest"
)

// TestRunAndDecodeMatchesRunThenDecode covers spec.md's User Story 3
// Acceptance Scenario: RunAndDecode must return the same typed value and exit
// code as calling Run then Decode separately, since it composes the two with
// no independent logic that could drift from either.
func TestRunAndDecodeMatchesRunThenDecode(t *testing.T) {
	ctx := context.Background()
	args := []string{"greet", "--format=json", "--yes"}

	composedData, composedExitCode := axtest.RunAndDecode[greetResult](ctx, t, newGreetCommand(t), args)

	result := axtest.Run(ctx, t, newGreetCommand(t), args)
	manualData := axtest.Decode[greetResult](t, result.Stdout)

	if composedExitCode != result.ExitCode {
		t.Fatalf("RunAndDecode exit code = %d, want %d (Run then Decode)", composedExitCode, result.ExitCode)
	}
	if composedData != manualData {
		t.Fatalf("RunAndDecode data = %+v, want %+v (Run then Decode)", composedData, manualData)
	}
}
