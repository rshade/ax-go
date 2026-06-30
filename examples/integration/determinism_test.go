package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/internal/testutil"
)

// tbSpy embeds *testing.T to satisfy testing.TB (including unexported methods)
// and overrides Errorf to record whether a failure was reported without
// propagating it to the parent test. Used to verify that the harness itself
// detects divergences correctly.
type tbSpy struct {
	*testing.T

	errorfCalled bool
}

const deterministicEntityID = "019744d2-1a5f-7000-8000-000000000001"

func (s *tbSpy) Errorf(format string, args ...any) {
	s.errorfCalled = true
	s.T.Logf("spy caught Errorf: "+format, args...)
}

// runWithArgs executes the integration command with the given args and returns
// the stdout bytes. Root-command runs use a fixed entity ID so determinism tests
// compare payload data instead of masking resource identifiers. The test fails
// immediately if the command exits with a non-success code.
func runWithArgs(t *testing.T, args []string) []byte {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithEntityID(
		context.Background(),
		args,
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
		ax.ResolveVersion(version),
		func() (string, error) { return deterministicEntityID, nil },
	)
	if code != ax.ExitSuccess {
		t.Fatalf("run %v failed with code %d; stderr=%s", args, code, stderr.String())
	}
	return stdout.Bytes()
}

// runDefault executes the integration command with the default args for a
// determinism test: pinned idempotency key and JSON format.
func runDefault(t *testing.T) []byte {
	return runWithArgs(t, []string{"--format=json", "--idempotency-key=test-key"})
}

// TestDeterminismSuccessPath runs the default integration command twice with
// identical arguments and asserts byte-identical stdout after masking.
func TestDeterminismSuccessPath(t *testing.T) {
	out1 := runDefault(t)
	out2 := runDefault(t)
	testutil.CompareOutputs(t, out1, out2, testutil.ModeBoundedJSON)
}

// TestDeterminismSuccessPathBreakDetection verifies that CompareOutputs detects
// a deliberately injected divergence. The spy intercepts the expected t.Errorf
// call and asserts it was made.
func TestDeterminismSuccessPathBreakDetection(t *testing.T) {
	out1 := runDefault(t)
	out2 := runDefault(t)

	// Inject a divergence into the second run's bytes to simulate a
	// non-deterministic payload that masking cannot cover.
	out2Patched := bytes.ReplaceAll(out2, []byte(`"greeting":"hello"`), []byte(`"greeting":"world"`))

	spy := &tbSpy{T: t}
	testutil.CompareOutputs(spy, out1, out2Patched, testutil.ModeBoundedJSON)
	if !spy.errorfCalled {
		t.Error("CompareOutputs did not call t.Errorf on a deliberate divergence")
	}
}

// TestDeterminismStreamPath runs the stream subcommand twice with a pinned
// idempotency key and asserts byte-identical NDJSON output after masking.
func TestDeterminismStreamPath(t *testing.T) {
	out1 := runStream(t)
	out2 := runStream(t)
	testutil.CompareOutputs(t, out1, out2, testutil.ModeNDJSON)
}

// TestDeterminismStreamLineCountMismatch verifies that CompareOutputs detects a
// line-count mismatch in NDJSON output.
func TestDeterminismStreamLineCountMismatch(t *testing.T) {
	out1 := runStream(t)
	out2 := runStream(t)

	// Trim one NDJSON line from the second run's bytes to simulate a mismatch.
	lines := strings.SplitN(string(out2), "\n", 2)
	out2Trimmed := []byte(lines[0] + "\n")

	spy := &tbSpy{T: t}
	testutil.CompareOutputs(spy, out1, out2Trimmed, testutil.ModeNDJSON)
	if !spy.errorfCalled {
		t.Error("CompareOutputs did not call t.Errorf on a line-count mismatch")
	}
}

// TestDeterminismTimestampValidation runs the default command and passes the
// stdout bytes to ValidateTimestamps. The current integration payload carries
// no timestamp fields so this must pass silently (SC-003).
func TestDeterminismTimestampValidation(t *testing.T) {
	out := runDefault(t)
	testutil.ValidateTimestamps(t, out)
}

// TestDeterminismFullyTypedEnvelope runs the default command and asserts the
// stdout can be unmarshalled cleanly into the strongly-typed envelope.
func TestDeterminismFullyTypedEnvelope(t *testing.T) {
	out := runDefault(t)
	testutil.AssertFullyTyped[ax.Envelope[helloPayload]](t, out)
}

// runStream executes the stream subcommand with a pinned idempotency key and
// returns the stdout bytes.
func runStream(t *testing.T) []byte {
	return runWithArgs(t, []string{"stream", "--format=json", "--idempotency-key=test-key", "--count=3"})
}
