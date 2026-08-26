package axtest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/rshade/ax-go/axtest"
)

// greetingFixtureResult is a plain caller-defined type decoded from a literal
// envelope fixture — deliberately not axtest.Run's output, so this story
// stays independently testable of User Story 1.
type greetingFixtureResult struct {
	Greeting string `json:"greeting"`
}

// fakeTB records Helper() and Fatalf() calls instead of exiting the goroutine,
// the standard technique for testing a helper that calls t.Fatalf: a real
// *testing.T's Fatalf cannot be observed without killing the test.
type fakeTB struct {
	testing.TB

	helperCalled  bool
	fatalfCalled  bool
	fatalfMessage string
}

func (f *fakeTB) Helper() {
	f.helperCalled = true
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatalfCalled = true
	f.fatalfMessage = fmt.Sprintf(format, args...)
}

// TestDecodeSuccess covers spec.md's User Story 2 Acceptance Scenario 1: a
// well-formed envelope decodes to the expected typed value with no wrapper
// struct declared in this test.
func TestDecodeSuccess(t *testing.T) {
	stdout := []byte(`{"data":{"greeting":"hi"},"meta":{"trace_id":"00000000000000000000000000000000"}}`)

	got := axtest.Decode[greetingFixtureResult](t, stdout)

	if got.Greeting != "hi" {
		t.Fatalf("Greeting = %q, want %q", got.Greeting, "hi")
	}
}

// TestDecodeFailsImmediatelyOnMalformedInput covers spec.md's User Story 2
// Acceptance Scenario 2 (FR-006): malformed JSON and a shape mismatch must
// both fail the calling test immediately, naming the cause.
func TestDecodeFailsImmediatelyOnMalformedInput(t *testing.T) {
	tests := []struct {
		name   string
		stdout []byte
	}{
		{name: "malformed JSON", stdout: []byte(`{not valid json`)},
		{
			name:   "shape mismatch",
			stdout: []byte(`{"data":"not-an-object","meta":{"trace_id":"00000000000000000000000000000000"}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &fakeTB{TB: t}

			axtest.Decode[greetingFixtureResult](spy, tt.stdout)

			if !spy.helperCalled {
				t.Error("Decode did not call t.Helper()")
			}
			if !spy.fatalfCalled {
				t.Fatal("Decode did not call t.Fatalf() on malformed input")
			}
			if spy.fatalfMessage == "" {
				t.Error("Decode's Fatalf message is empty, want a cause naming the parse or shape-mismatch error")
			}
		})
	}
}

// TestDecodeComposesWithRun is an explicit cross-story integration case: it
// decodes the real Result.Stdout produced by axtest.Run (User Story 1) with
// axtest.Decode (User Story 2), validating the two already-independent
// stories integrate correctly rather than depending on each other's tests.
func TestDecodeComposesWithRun(t *testing.T) {
	result := axtest.Run(context.Background(), t, newGreetCommand(t), []string{"greet", "--format=json", "--yes"})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%s", result.ExitCode, result.Stderr)
	}

	got := axtest.Decode[greetResult](t, result.Stdout)

	if !got.Approved {
		t.Error("Approved = false, want true")
	}
}
