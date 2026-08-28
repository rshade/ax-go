package ax_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"

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
// os.Stderr, so callers MUST NOT be parallel. Captured output is small and
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

func assertAuditLines(t *testing.T, out, wantHelper, wantDescription string, wantErr error) {
	t.Helper()
	trimmed := strings.TrimSuffix(out, "\n")
	if trimmed == "" {
		t.Fatal("no audit lines emitted")
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) != 2 {
		t.Fatalf("want exactly two audit lines, got %d:\n%s", len(lines), out)
	}

	wantMessages := []string{"ax: about to run effect", "ax: effect succeeded"}
	wantLevels := []string{"info", "info"}
	if wantErr != nil {
		wantMessages[1] = "ax: effect failed"
		wantLevels[1] = "error"
	}
	for i, line := range lines {
		fields := mustJSONLine(t, line)
		if fields["level"] != wantLevels[i] {
			t.Errorf("line %d level = %v, want %q", i, fields["level"], wantLevels[i])
		}
		if fields["message"] != wantMessages[i] {
			t.Errorf("line %d message = %v, want %q", i, fields["message"], wantMessages[i])
		}
		if fields["ax_helper"] != wantHelper {
			t.Errorf("line %d ax_helper = %v, want %q", i, fields["ax_helper"], wantHelper)
		}
		if fields["description"] != wantDescription {
			t.Errorf("line %d description = %v, want %q", i, fields["description"], wantDescription)
		}
		// Verify trace_id and span_id are present and non-empty
		traceID, ok := fields["trace_id"].(string)
		if !ok || traceID == "" {
			t.Errorf("line %d trace_id missing or empty: %v", i, fields["trace_id"])
		}
		spanID, ok := fields["span_id"].(string)
		if !ok || spanID == "" {
			t.Errorf("line %d span_id missing or empty: %v", i, fields["span_id"])
		}
		if i != 1 {
			continue
		}
		if wantErr == nil {
			if _, ok := fields["error"]; ok {
				t.Errorf("successful outcome unexpectedly carries error field: %s", line)
			}
			continue
		}
		if got := fields["error"]; got != wantErr.Error() {
			t.Errorf("failed outcome error = %v, want %q", got, wantErr.Error())
		}
	}
}

func mustJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		t.Fatalf("log line is not JSON: %v (line=%q)", err, line)
	}
	return fields
}

func withAuditSetting(ctx context.Context, enabled *bool) context.Context {
	if enabled == nil {
		return ctx
	}
	return ax.WithAudit(ctx, *enabled)
}

func boolPointer(value bool) *bool { return &value }

func TestAuditEnabledFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"no prior WithAudit call defaults to true", context.Background(), true},
		{"WithAudit(ctx, false)", ax.WithAudit(context.Background(), false), false},
		{"WithAudit(ctx, true)", ax.WithAudit(context.Background(), true), true},
		{
			"nested WithAudit overrides ancestor",
			ax.WithAudit(ax.WithAudit(context.Background(), false), true),
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ax.AuditEnabledFromContext(tc.ctx); got != tc.want {
				t.Errorf("AuditEnabledFromContext() = %v, want %v", got, tc.want)
			}
		})
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
	assertAuditLines(t, out, "Guard", "", nil)

	out = captureStderr(t, func() {
		_, _ = ax.Guard(dryRunCtx(), nil)
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("dry-run with nil effect emitted a line: %q", out)
	}
}

func TestGuardAuditTruthTable(t *testing.T) {
	tagged := &taggedError{tag: "guard"}
	wrapped := fmt.Errorf("guard effect: %w", tagged)
	tests := []struct {
		name         string
		dryRun       bool
		auditEnabled *bool
		effectNil    bool
		effectErr    error
		wantExecuted bool
		wantRan      bool
		wantLog      string
	}{
		{"real success audits by default", false, nil, false, nil, true, true, "audit-success"},
		{"real failure audits explicit enabled", false, boolPointer(true), false, wrapped, true, true, "audit-failure"},
		{"real success opt-out is silent", false, boolPointer(false), false, nil, true, true, "none"},
		{"real nil effect is silent", false, nil, true, nil, false, false, "none"},
		{"dry-run effect suppresses with audit default", true, nil, false, nil, false, false, "suppression"},
		{
			"dry-run effect suppresses with audit opt-out",
			true, boolPointer(false), false, nil, false, false, "suppression",
		},
		{"dry-run nil effect is silent", true, boolPointer(true), true, nil, false, false, "none"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := withAuditSetting(ax.WithDryRun(context.Background(), tc.dryRun), tc.auditEnabled)
			var effect func(context.Context) error
			var ran *bool
			if !tc.effectNil {
				effect, ran = recordingFn(tc.effectErr)
			}
			var executed bool
			var err error
			out := captureStderr(t, func() {
				executed, err = ax.Guard(ctx, effect)
			})
			if executed != tc.wantExecuted {
				t.Errorf("executed = %v, want %v", executed, tc.wantExecuted)
			}
			if ran != nil && *ran != tc.wantRan {
				t.Errorf("effect ran = %v, want %v", *ran, tc.wantRan)
			}
			if tc.effectErr == nil {
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tagged) {
					t.Errorf("errors.Is(err, tagged) = false; err=%v", err)
				}
				var gotTagged *taggedError
				if !errors.As(err, &gotTagged) {
					t.Errorf("errors.As(err, *taggedError) = false; err=%v", err)
				}
			}
			switch tc.wantLog {
			case "audit-success":
				assertAuditLines(t, out, "Guard", "", nil)
			case "audit-failure":
				assertAuditLines(t, out, "Guard", "", wrapped)
			case "suppression":
				assertSuppressionLine(t, out, "Guard")
			case "none":
				if strings.TrimSpace(out) != "" {
					t.Errorf("unexpected log output: %q", out)
				}
			}
		})
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
		{"real runs commit", realCtx(), false, nil, false, nil, false, true, nil, true},
		{"real propagates commit error", realCtx(), false, nil, false, errSentinel, false, true, errSentinel, true},
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

	// Real run emits the default audit pair.
	rehearse3, _ := recordingFn(nil)
	commit3, _ := recordingFn(nil)
	out = captureStderr(t, func() {
		_ = ax.Perform(realCtx(), rehearse3, commit3)
	})
	assertAuditLines(t, out, "Perform", "", nil)
}

func TestPerformAuditTruthTable(t *testing.T) {
	tagged := &taggedError{tag: "perform"}
	wrapped := fmt.Errorf("perform commit: %w", tagged)
	tests := []struct {
		name            string
		dryRun          bool
		auditEnabled    *bool
		rehearseNil     bool
		rehearseErr     error
		commitNil       bool
		commitErr       error
		wantRehearseRan bool
		wantCommitRan   bool
		wantErr         error
		wantLog         string
	}{
		{
			name: "real success audits by default", wantCommitRan: true,
			wantLog: "audit-success",
		},
		{
			name: "real failure audits explicit enabled", auditEnabled: boolPointer(true),
			commitErr: wrapped, wantCommitRan: true, wantErr: tagged, wantLog: "audit-failure",
		},
		{
			name: "real success opt-out is silent", auditEnabled: boolPointer(false),
			wantCommitRan: true, wantLog: "none",
		},
		{name: "real nil commit is silent", commitNil: true, wantLog: "none"},
		{
			name: "dry nil rehearse suppresses commit", dryRun: true, rehearseNil: true,
			wantLog: "suppression",
		},
		{
			name: "dry nil callbacks are silent", dryRun: true, auditEnabled: boolPointer(false),
			rehearseNil: true, commitNil: true, wantLog: "none",
		},
		{
			name: "dry successful rehearse suppresses commit", dryRun: true,
			auditEnabled: boolPointer(false), wantRehearseRan: true, wantLog: "suppression",
		},
		{
			name: "dry successful rehearse with nil commit is silent", dryRun: true,
			commitNil: true, wantRehearseRan: true, wantLog: "none",
		},
		{
			name: "dry failed rehearse does not suppress commit", dryRun: true,
			auditEnabled: boolPointer(true), rehearseErr: errSentinel,
			wantRehearseRan: true, wantErr: errSentinel, wantLog: "none",
		},
		{
			name: "dry failed rehearse with nil commit is silent", dryRun: true,
			rehearseErr: errSentinel, commitNil: true, wantRehearseRan: true,
			wantErr: errSentinel, wantLog: "none",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := withAuditSetting(ax.WithDryRun(context.Background(), tc.dryRun), tc.auditEnabled)
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
				err = ax.Perform(ctx, rehearse, commit)
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
			if tc.commitErr != nil {
				var gotTagged *taggedError
				if !errors.As(err, &gotTagged) {
					t.Errorf("errors.As(err, *taggedError) = false; err=%v", err)
				}
			}
			switch tc.wantLog {
			case "audit-success":
				assertAuditLines(t, out, "Perform", "", nil)
			case "audit-failure":
				assertAuditLines(t, out, "Perform", "", wrapped)
			case "suppression":
				assertSuppressionLine(t, out, "Perform")
			case "none":
				if strings.TrimSpace(out) != "" {
					t.Errorf("unexpected log output: %q", out)
				}
			}
		})
	}
}

func TestGuardWithAuditDescription(t *testing.T) {
	const description = "destroy stack prod-east"
	effect, _ := recordingFn(nil)
	var executed bool
	var err error
	realOutput := captureStderr(t, func() {
		executed, err = ax.GuardWithAudit(realCtx(), description, effect)
	})
	if !executed || err != nil {
		t.Fatalf("GuardWithAudit real run: executed=%v err=%v", executed, err)
	}
	assertAuditLines(t, realOutput, "Guard", description, nil)

	dryEffect, _ := recordingFn(nil)
	describedDryOutput := captureStderr(t, func() {
		_, _ = ax.GuardWithAudit(dryRunCtx(), description, dryEffect)
	})
	plainDryOutput := captureStderr(t, func() {
		_, _ = ax.Guard(dryRunCtx(), dryEffect)
	})
	if describedDryOutput != plainDryOutput {
		t.Errorf(
			"GuardWithAudit dry-run output differs from Guard:\n described=%q\n plain=    %q",
			describedDryOutput,
			plainDryOutput,
		)
	}
}

func TestPerformWithAuditDescription(t *testing.T) {
	const description = "reconcile stack prod-east"
	tagged := &taggedError{tag: "commit"}
	wrapped := fmt.Errorf("apply plan: %w", tagged)
	commit, committed := recordingFn(wrapped)
	var err error
	out := captureStderr(t, func() {
		err = ax.PerformWithAudit(realCtx(), description, nil, commit)
	})
	if !*committed {
		t.Fatal("PerformWithAudit did not run commit")
	}
	if !errors.Is(err, tagged) {
		t.Errorf("errors.Is(err, tagged) = false; err=%v", err)
	}
	var gotTagged *taggedError
	if !errors.As(err, &gotTagged) {
		t.Errorf("errors.As(err, *taggedError) = false; err=%v", err)
	}
	assertAuditLines(t, out, "Perform", description, wrapped)
}

func TestDryRunAuditParity(t *testing.T) {
	type invokeFn func(context.Context) (bool, error)
	tests := []struct {
		name       string
		wantHelper string
		invoke     invokeFn
	}{
		{
			name: "Guard", wantHelper: "Guard",
			invoke: func(ctx context.Context) (bool, error) {
				ran := false
				_, err := ax.Guard(ctx, func(context.Context) error { ran = true; return nil })
				return ran, err
			},
		},
		{
			name: "GuardWithAudit", wantHelper: "Guard",
			invoke: func(ctx context.Context) (bool, error) {
				ran := false
				_, err := ax.GuardWithAudit(
					ctx,
					"destroy stack prod-east",
					func(context.Context) error { ran = true; return nil },
				)
				return ran, err
			},
		},
		{
			name: "Perform", wantHelper: "Perform",
			invoke: func(ctx context.Context) (bool, error) {
				ran := false
				err := ax.Perform(ctx, nil, func(context.Context) error { ran = true; return nil })
				return ran, err
			},
		},
		{
			name: "PerformWithAudit", wantHelper: "Perform",
			invoke: func(ctx context.Context) (bool, error) {
				ran := false
				err := ax.PerformWithAudit(
					ctx,
					"apply stack prod-east",
					nil,
					func(context.Context) error { ran = true; return nil },
				)
				return ran, err
			},
		},
	}

	plainOutputs := make(map[string]string)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var defaultRan bool
			var defaultErr error
			defaultOutput := captureStderr(t, func() {
				defaultRan, defaultErr = tc.invoke(dryRunCtx())
			})
			if defaultRan || defaultErr != nil {
				t.Errorf("default audit state: ran=%v err=%v; want false, nil", defaultRan, defaultErr)
			}
			assertSuppressionLine(t, defaultOutput, tc.wantHelper)

			var optedOutRan bool
			var optedOutErr error
			optedOutOutput := captureStderr(t, func() {
				optedOutRan, optedOutErr = tc.invoke(ax.WithAudit(dryRunCtx(), false))
			})
			if optedOutRan || optedOutErr != nil {
				t.Errorf("audit opt-out: ran=%v err=%v; want false, nil", optedOutRan, optedOutErr)
			}
			if optedOutOutput != defaultOutput {
				t.Errorf(
					"audit opt-out changed dry-run output:\n default=%q\n opt-out=%q",
					defaultOutput,
					optedOutOutput,
				)
			}

			if tc.name == tc.wantHelper {
				plainOutputs[tc.wantHelper] = defaultOutput
				return
			}
			plain, ok := plainOutputs[tc.wantHelper]
			if !ok {
				t.Skipf("plain %s variant did not run in this test invocation", tc.wantHelper)
			}
			if defaultOutput != plain {
				t.Errorf(
					"named variant changed dry-run output:\n plain=%q\n named=%q",
					plain,
					defaultOutput,
				)
			}
		})
	}
}

func TestPerformDryRunRehearsalFailureHasNoAudit(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(context.Context) error
	}{
		{
			name: "Perform",
			invoke: func(ctx context.Context) error {
				return ax.Perform(
					ctx,
					func(context.Context) error { return errSentinel },
					func(context.Context) error { return nil },
				)
			},
		},
		{
			name: "PerformWithAudit",
			invoke: func(ctx context.Context) error {
				return ax.PerformWithAudit(
					ctx,
					"apply plan",
					func(context.Context) error { return errSentinel },
					func(context.Context) error { return nil },
				)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, auditEnabled := range []*bool{nil, boolPointer(false)} {
				ctx := withAuditSetting(dryRunCtx(), auditEnabled)
				var err error
				out := captureStderr(t, func() { err = tc.invoke(ctx) })
				if !errors.Is(err, errSentinel) {
					t.Errorf("err = %v, want sentinel", err)
				}
				if strings.TrimSpace(out) != "" {
					t.Errorf("failed rehearsal emitted log output: %q", out)
				}
			}
		})
	}
}

func TestAuditDescriptionCannotForgeLogLines(t *testing.T) {
	const description = "destroy stack\n{\"level\":\"fatal\"}\t\x1b[31m"
	commitErr := errors.New("commit failed\nwith forged-looking text")
	out := captureStderr(t, func() {
		_, _ = ax.GuardWithAudit(realCtx(), description, func(context.Context) error { return nil })
		_ = ax.PerformWithAudit(realCtx(), description, nil, func(context.Context) error {
			return commitErr
		})
	})

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("audit output split into %d lines, want exactly 4:\n%s", len(lines), out)
	}
	wantHelpers := []string{"Guard", "Guard", "Perform", "Perform"}
	wantMessages := []string{
		"ax: about to run effect",
		"ax: effect succeeded",
		"ax: about to run effect",
		"ax: effect failed",
	}
	for i, line := range lines {
		fields := mustJSONLine(t, line)
		if fields["ax_helper"] != wantHelpers[i] {
			t.Errorf("line %d ax_helper = %v, want %q", i, fields["ax_helper"], wantHelpers[i])
		}
		if fields["description"] != description {
			t.Errorf("line %d description = %q, want raw adversarial value", i, fields["description"])
		}
		if fields["message"] != wantMessages[i] {
			t.Errorf("line %d message = %v, want constant %q", i, fields["message"], wantMessages[i])
		}
		for key, value := range fields {
			if key != "description" && value == description {
				t.Errorf("line %d copies description into %q field", i, key)
			}
		}
	}
	if got := mustJSONLine(t, lines[3])["error"]; got != commitErr.Error() {
		t.Errorf("failed line error = %v, want %q", got, commitErr.Error())
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
			_, _ = ax.GuardWithAudit(ctx, "write report", func(context.Context) error { return nil })
			_ = ax.PerformWithAudit(ctx, "apply plan", nil, func(context.Context) error { return nil })
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
	_ = captureStderr(t, func() {
		var executed bool
		var err error
		executed, err = ax.Guard(nilCtx, effect)
		if err != nil || !executed || ran == nil || !*ran {
			t.Fatalf("Guard(nil): executed=%v err=%v; want real path with no panic", executed, err)
		}
	})

	commit, committed := recordingFn(nil)
	_ = captureStderr(t, func() {
		var err error
		err = ax.Perform(nilCtx, nil, commit)
		if err != nil || committed == nil || !*committed {
			t.Fatalf("Perform(nil): err=%v; want commit to run with no panic", err)
		}
	})
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

	var gErr error
	_ = captureStderr(t, func() {
		_, gErr = ax.Guard(realCtx(), returnsWrapped)
	})
	check(t, gErr)

	var pErr error
	_ = captureStderr(t, func() {
		pErr = ax.Perform(realCtx(), nil, returnsWrapped)
	})
	check(t, pErr)

	_ = captureStderr(t, func() {
		check(t, ax.Perform(dryRunCtx(), returnsWrapped, func(context.Context) error { return nil }))
	})
}

func TestGuardPanicAuditAbnormal(t *testing.T) {
	// When the effect panics, the helper must emit exactly 2 audit lines:
	// one "about to run" and one "did not return normally" (abnormal termination).
	// The panic is recovered at the call site (simulating dispatch.go behavior).
	panicEffect := func(context.Context) error {
		panic("boom")
	}

	out := captureStderr(t, func() {
		defer func() {
			recover() // recover the panic, simulating what dispatch.go does
		}()
		_, _ = ax.Guard(realCtx(), panicEffect)
	})

	trimmed := strings.TrimSuffix(out, "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) != 2 {
		t.Fatalf("want exactly 2 audit lines for panic, got %d:\n%s", len(lines), out)
	}

	firstLine := mustJSONLine(t, lines[0])
	if firstLine["message"] != "ax: about to run effect" {
		t.Errorf("first line message = %q, want 'ax: about to run effect'", firstLine["message"])
	}

	secondLine := mustJSONLine(t, lines[1])
	if secondLine["message"] != "ax: effect did not return normally" {
		t.Errorf("second line message = %q, want 'ax: effect did not return normally'", secondLine["message"])
	}
	if secondLine["level"] != "error" {
		t.Errorf("second line level = %q, want 'error'", secondLine["level"])
	}
}

func TestPerformPanicAuditAbnormal(t *testing.T) {
	// Perform should also emit abnormal-termination audit line on panic.
	panicCommit := func(context.Context) error {
		panic("commit boom")
	}

	out := captureStderr(t, func() {
		defer func() {
			recover()
		}()
		_ = ax.Perform(realCtx(), nil, panicCommit)
	})

	trimmed := strings.TrimSuffix(out, "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) != 2 {
		t.Fatalf("want exactly 2 audit lines for panic, got %d:\n%s", len(lines), out)
	}

	secondLine := mustJSONLine(t, lines[1])
	if secondLine["message"] != "ax: effect did not return normally" {
		t.Errorf("second line message = %q, want 'ax: effect did not return normally'", secondLine["message"])
	}
}

func TestURLErrorRedaction(t *testing.T) {
	// URL errors with query-string secrets should be redacted in audit logs,
	// but the wrap chain should survive intact for the caller.
	urlErr := &url.Error{
		Op:  "Get",
		URL: "https://api.example.com/v1?api_key=SECRET123",
		Err: errors.New("connection refused"),
	}
	urlWrapped := fmt.Errorf("request failed: %w", urlErr)

	out := captureStderr(t, func() {
		_, returnedErr := ax.Guard(realCtx(), func(context.Context) error { return urlWrapped })

		// Verify the returned error chain is intact (not altered by logging)
		if !errors.Is(returnedErr, urlErr) {
			t.Errorf("errors.Is(returnedErr, urlErr) = false; wrap chain altered in return value")
		}
	})

	// Verify the logged error does NOT contain the URL or secret
	if strings.Contains(out, "SECRET123") {
		t.Errorf("audit log leaked secret: %s", out)
	}
	if strings.Contains(out, "api.example.com") {
		t.Errorf("audit log leaked URL: %s", out)
	}
	// Verify the underlying error message is still logged
	if !strings.Contains(out, "connection refused") {
		t.Errorf("audit log should contain underlying error 'connection refused', got: %s", out)
	}
}

func TestAuditLineTracingCorrelation(t *testing.T) {
	// Create a context with a real OTel span so we can verify trace_id and span_id
	// are correctly populated in audit lines from the active span context.
	ctx, tel, err := ax.StartTelemetry(
		context.Background(),
		ax.WithTelemetryEnv(func(string) string { return "" }),
		ax.WithTelemetryServiceName("audit-trace-test"),
	)
	if err != nil {
		t.Fatalf("StartTelemetry: %v", err)
	}
	t.Cleanup(func() {
		if err := tel.Shutdown(context.Background()); err != nil {
			t.Fatalf("Telemetry.Shutdown: %v", err)
		}
	})

	// Create a real span in the context.
	spanCtx, span := otel.Tracer("github.com/rshade/ax-go/test").Start(ctx, "audit-test-op")
	defer span.End()

	// Get the expected trace_id and span_id from the span context.
	sc := span.SpanContext()
	expectedTraceID := sc.TraceID().String()
	expectedSpanID := sc.SpanID().String()

	// Call GuardWithAudit with the span-bearing context and capture stderr.
	out := captureStderr(t, func() {
		_, _ = ax.GuardWithAudit(spanCtx, "test operation", func(context.Context) error { return nil })
	})

	// Parse the two audit lines and verify trace_id and span_id match the span context.
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 audit lines, got %d", len(lines))
	}

	for i, line := range lines {
		fields := mustJSONLine(t, line)
		if gotTraceID, ok := fields["trace_id"].(string); !ok || gotTraceID != expectedTraceID {
			t.Errorf("line %d trace_id = %q, want %q", i, gotTraceID, expectedTraceID)
		}
		if gotSpanID, ok := fields["span_id"].(string); !ok || gotSpanID != expectedSpanID {
			t.Errorf("line %d span_id = %q, want %q", i, gotSpanID, expectedSpanID)
		}
	}
}
