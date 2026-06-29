package ax_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	ax "github.com/rshade/ax-go"
)

var errSentinel = errors.New("sentinel")

// taggedError is a typed error used to prove the wrap chain survives a helper
// (errors.As must still recover the concrete type).
type taggedError struct{ tag string }

func (e *taggedError) Error() string { return "tagged: " + e.tag }

func dryRunCtx() context.Context { return ax.WithDryRun(context.Background(), true) }
func realCtx() context.Context   { return ax.WithDryRun(context.Background(), false) }

// recordingFn returns a callback that records whether it ran and returns err.
func recordingFn(err error) (func(context.Context) error, *bool) {
	r := new(bool)
	return func(context.Context) error {
		*r = true
		return err
	}, r
}

// captureStderr swaps os.Stderr for a pipe, runs fn, restores os.Stderr, and
// returns everything written to stderr during fn. It mutates the global
// os.Stderr, so callers MUST NOT be parallel. The suppression line is small and
// fits the pipe buffer, so writing then reading after Close cannot deadlock.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w //nolint:reassign // redirect process stderr to capture the canonical logger's suppression line
	defer func() {
		os.Stderr = orig //nolint:reassign // restore process stderr after capture
	}()
	fn()
	if cerr := w.Close(); cerr != nil {
		t.Fatalf("close pipe writer: %v", cerr)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

func assertSuppressionLine(t *testing.T, out, wantHelper string) {
	t.Helper()
	line := strings.TrimSpace(out)
	if line == "" {
		t.Fatalf("no suppression line emitted")
	}
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("want exactly one suppression line, got:\n%s", line)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		t.Fatalf("suppression line not JSON: %v (line=%q)", err, line)
	}
	if fields["dry_run"] != true {
		t.Errorf("dry_run field = %v, want true", fields["dry_run"])
	}
	if fields["ax_helper"] != wantHelper {
		t.Errorf("ax_helper = %v, want %q", fields["ax_helper"], wantHelper)
	}
	if msg, _ := fields["message"].(string); !strings.Contains(msg, "side effect suppressed") {
		t.Errorf("message = %q, want suppression text", msg)
	}
}

func TestGuard(t *testing.T) {
	tests := []struct {
		name         string
		ctx          context.Context
		effectNil    bool
		effectErr    error
		wantExecuted bool
		wantRan      bool
		wantErr      error
	}{
		{"real run executes effect", realCtx(), false, nil, true, true, nil},
		{"real run propagates error", realCtx(), false, errSentinel, true, true, errSentinel},
		{"dry-run skips effect", dryRunCtx(), false, nil, false, false, nil},
		{"real run nil effect is noop", realCtx(), true, nil, false, false, nil},
		{"dry-run nil effect is noop", dryRunCtx(), true, nil, false, false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var effect func(context.Context) error
			var ran *bool
			if !tc.effectNil {
				effect, ran = recordingFn(tc.effectErr)
			}
			var executed bool
			var err error
			// Capture stderr so the dry-run suppression line does not pollute test output.
			_ = captureStderr(t, func() {
				executed, err = ax.Guard(tc.ctx, effect)
			})
			if executed != tc.wantExecuted {
				t.Errorf("executed = %v, want %v", executed, tc.wantExecuted)
			}
			if ran != nil && *ran != tc.wantRan {
				t.Errorf("effect ran = %v, want %v", *ran, tc.wantRan)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestGuardSuppressionLogged(t *testing.T) {
	effect, _ := recordingFn(nil)
	out := captureStderr(t, func() {
		_, _ = ax.Guard(dryRunCtx(), effect)
	})
	assertSuppressionLine(t, out, "Guard")

	effect2, _ := recordingFn(nil)
	out = captureStderr(t, func() {
		_, _ = ax.Guard(realCtx(), effect2)
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("real run emitted a suppression line: %q", out)
	}

	out = captureStderr(t, func() {
		_, _ = ax.Guard(dryRunCtx(), nil)
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("dry-run with nil effect emitted a line: %q", out)
	}
}

func TestPerform(t *testing.T) {
	tests := []struct {
		name            string
		ctx             context.Context
		rehearseNil     bool
		rehearseErr     error
		commitNil       bool
		commitErr       error
		wantRehearseRan bool
		wantCommitRan   bool
		wantErr         error
		wantLog         bool
	}{
		{"real runs commit", realCtx(), false, nil, false, nil, false, true, nil, false},
		{"real propagates commit error", realCtx(), false, nil, false, errSentinel, false, true, errSentinel, false},
		{"real nil commit is noop", realCtx(), false, nil, true, nil, false, false, nil, false},
		{"dry runs rehearse skips commit", dryRunCtx(), false, nil, false, nil, true, false, nil, true},
		{"dry rehearse error no log", dryRunCtx(), false, errSentinel, false, nil, true, false, errSentinel, false},
		{"dry rehearse ok nil commit no log", dryRunCtx(), false, nil, true, nil, true, false, nil, false},
		{"dry nil rehearse pure skip logs", dryRunCtx(), true, nil, false, nil, false, false, nil, true},
		{"dry nil rehearse nil commit noop", dryRunCtx(), true, nil, true, nil, false, false, nil, false},
		{"dry rehearse err nil commit", dryRunCtx(), false, errSentinel, true, nil, true, false, errSentinel, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rehearse, commit func(context.Context) error
			var rehearseRan, commitRan *bool
			if !tc.rehearseNil {
				rehearse, rehearseRan = recordingFn(tc.rehearseErr)
			}
			if !tc.commitNil {
				commit, commitRan = recordingFn(tc.commitErr)
			}
			var err error
			out := captureStderr(t, func() {
				err = ax.Perform(tc.ctx, rehearse, commit)
			})
			if rehearseRan != nil && *rehearseRan != tc.wantRehearseRan {
				t.Errorf("rehearse ran = %v, want %v", *rehearseRan, tc.wantRehearseRan)
			}
			if commitRan != nil && *commitRan != tc.wantCommitRan {
				t.Errorf("commit ran = %v, want %v", *commitRan, tc.wantCommitRan)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
			logged := strings.TrimSpace(out) != ""
			if logged != tc.wantLog {
				t.Errorf("suppression logged = %v, want %v (out=%q)", logged, tc.wantLog, out)
			}
		})
	}
}

func TestPerformRehearsalParity(t *testing.T) {
	// A single validator used as both the dry-run rehearse and the real commit.
	// For bad input it rejects identically in both modes; commit never runs under dry-run.
	validate := func(context.Context) error { return errSentinel }

	commit, committed := recordingFn(errSentinel)
	var dryErr error
	_ = captureStderr(t, func() {
		dryErr = ax.Perform(dryRunCtx(), validate, commit)
	})
	if !errors.Is(dryErr, errSentinel) {
		t.Errorf("dry-run err = %v, want sentinel", dryErr)
	}
	if *committed {
		t.Error("commit ran under dry-run; want never")
	}

	realErr := ax.Perform(realCtx(), validate, func(context.Context) error { return errSentinel })
	if !errors.Is(realErr, errSentinel) {
		t.Errorf("real err = %v, want sentinel", realErr)
	}
}

func TestPerformSuppressionLogged(t *testing.T) {
	rehearse, _ := recordingFn(nil)
	commit, _ := recordingFn(nil)
	out := captureStderr(t, func() {
		_ = ax.Perform(dryRunCtx(), rehearse, commit)
	})
	assertSuppressionLine(t, out, "Perform")

	// nil rehearse under dry-run still logs (the commit was suppressed).
	commit2, _ := recordingFn(nil)
	out = captureStderr(t, func() {
		_ = ax.Perform(dryRunCtx(), nil, commit2)
	})
	assertSuppressionLine(t, out, "Perform")

	// real run never logs.
	rehearse3, _ := recordingFn(nil)
	commit3, _ := recordingFn(nil)
	out = captureStderr(t, func() {
		_ = ax.Perform(realCtx(), rehearse3, commit3)
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("real run emitted a suppression line: %q", out)
	}
}

func TestEnvelopeDeterministicUnderDryRun(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	build := func(ctx context.Context) []byte {
		// Route a no-op through both helpers first, proving that exercising a
		// helper never perturbs the envelope (FR-009).
		_ = captureStderr(t, func() {
			_, _ = ax.Guard(ctx, func(context.Context) error { return nil })
			_ = ax.Perform(ctx, nil, func(context.Context) error { return nil })
		})
		b, err := json.Marshal(ax.NewEnvelope(ctx, payload{Name: "ax"}))
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		return b
	}

	realJSON := build(realCtx())
	dryJSON := build(dryRunCtx())

	if bytes.Equal(realJSON, dryJSON) {
		t.Fatalf("dry-run envelope must differ by dry_run; both = %s", realJSON)
	}
	if !bytes.Contains(dryJSON, []byte(`"dry_run":true`)) {
		t.Fatalf("dry-run envelope missing dry_run:true; got %s", dryJSON)
	}
	// Byte-identical modulo the documented dry_run field (SC-004).
	stripped := bytes.Replace(dryJSON, []byte(`,"dry_run":true`), nil, 1)
	if !bytes.Equal(stripped, realJSON) {
		t.Errorf("envelopes differ beyond dry_run:\n stripped=%s\n real=    %s", stripped, realJSON)
	}
}

func TestGuardPerformNilContextNoPanic(t *testing.T) {
	// A nil context MUST NOT panic; the helpers treat dry-run as inactive and run
	// the real path (spec Edge Cases / FR-011).
	var nilCtx context.Context

	effect, ran := recordingFn(nil)
	executed, err := ax.Guard(nilCtx, effect)
	if err != nil || !executed || ran == nil || !*ran {
		t.Fatalf("Guard(nil): executed=%v err=%v; want real path with no panic", executed, err)
	}

	commit, committed := recordingFn(nil)
	if err := ax.Perform(nilCtx, nil, commit); err != nil || committed == nil || !*committed {
		t.Fatalf("Perform(nil): err=%v; want commit to run with no panic", err)
	}
}

func TestGuardPerformPreserveWrapChain(t *testing.T) {
	// The helpers return the callback's error verbatim, so errors.Is AND errors.As
	// must keep working through a %w wrap (FR-003 / FR-005). A flattening helper
	// (e.g. fmt.Errorf("%v", err)) would fail errors.As here.
	tagged := &taggedError{tag: "boom"}
	wrapped := fmt.Errorf("op: %w", tagged)
	returnsWrapped := func(context.Context) error { return wrapped }

	check := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, tagged) {
			t.Errorf("errors.Is(err, tagged) = false; wrap chain not preserved (err=%v)", err)
		}
		var te *taggedError
		if !errors.As(err, &te) {
			t.Errorf("errors.As failed; wrap chain not preserved (err=%v)", err)
		}
	}

	_, gErr := ax.Guard(realCtx(), returnsWrapped)
	check(t, gErr)

	check(t, ax.Perform(realCtx(), nil, returnsWrapped))

	_ = captureStderr(t, func() {
		check(t, ax.Perform(dryRunCtx(), returnsWrapped, func(context.Context) error { return nil }))
	})
}
