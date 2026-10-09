package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/unreadfield"
)

// updateGolden, when set via `go test -update`, rewrites the golden files.
// Review every changed byte against contracts/gate-cli.md before committing.
//
//nolint:gochecknoglobals // test-only golden-file update flag must be package-scoped for the flag package
var updateGolden = flag.Bool("update", false, "update golden files in testdata/")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s: %v (run `go test -update` to create it)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s mismatch:\n got  %s\n want %s", name, got, want)
	}
}

// checkEnvelope checks the failure streams: an empty stdout and one
// envelope on stderr carrying code.
func checkEnvelope(t *testing.T, stdout, stderr *bytes.Buffer, code string) {
	t.Helper()
	if stdout.Len() != 0 {
		t.Errorf("stdout not empty on a failure: %s", stdout)
	}
	var envelope struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not one JSON envelope: %v: %s", err, stderr)
	}
	if envelope.ErrorCode != code {
		t.Errorf("error_code = %q, want %q", envelope.ErrorCode, code)
	}
}

// brokenStub pins the analysis-failure golden without the compiler's own
// wording, which changes between Go releases. TestRunBrokenModule covers the
// real failure.
func brokenStub(
	context.Context, string, []unreadfield.Configuration, time.Duration,
) (unreadfield.Report, error) {
	return unreadfield.Report{}, &unreadfield.AnalysisError{
		Configuration: "default",
		Err:           errors.New("go list: exit status 1: broken.go:3:27: type error"),
	}
}

func TestRunBrokenModule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run(t.Context(), unreadfield.Run, []string{"-dir", "testdata/broken"}, &stdout, &stderr); got !=
		contract.ExitValidation {
		t.Errorf("exit %d, want %d: %s", got, contract.ExitValidation, &stderr)
	}
	checkEnvelope(t, &stdout, &stderr, codeAnalysis)
}

func TestRunUnreadableSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.go")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module unreadable\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("package unreadable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(src); err == nil {
		t.Skip("a mode-0 file is still readable here (root, or no POSIX permissions)")
	}
	var stdout, stderr bytes.Buffer
	if got := run(t.Context(), unreadfield.Run, []string{"-dir", dir}, &stdout, &stderr); got != contract.ExitAuth {
		t.Errorf("exit %d, want %d: %s", got, contract.ExitAuth, &stderr)
	}
	checkEnvelope(t, &stdout, &stderr, codePermission)
}

func TestRunGolden(t *testing.T) {
	for _, tc := range []struct {
		name, dir, stdoutGolden, stderrGolden string
		exit                                  int
		analyze                               runner
	}{
		{name: "pass", dir: "testdata/clean", stdoutGolden: "pass.stdout.golden", exit: contract.ExitSuccess},
		{name: "fail", dir: "testdata/failing", stderrGolden: "fail.stderr.golden", exit: contract.ExitValidation},
		{
			name: "analysis", dir: "testdata/broken", stderrGolden: "analysis.stderr.golden",
			exit: contract.ExitValidation, analyze: brokenStub,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			analyze := tc.analyze
			if analyze == nil {
				analyze = unreadfield.Run
			}
			var stdout, stderr bytes.Buffer
			got := run(t.Context(), analyze, []string{"-dir", tc.dir}, &stdout, &stderr)
			if got != tc.exit {
				t.Fatalf("exit %d, want %d: %s", got, tc.exit, &stderr)
			}
			if tc.stdoutGolden != "" {
				golden(t, tc.stdoutGolden, stdout.Bytes())
				if stderr.Len() != 0 {
					t.Errorf("stderr not empty on a pass: %s", &stderr)
				}
			}
			if tc.stderrGolden != "" {
				golden(t, tc.stderrGolden, stderr.Bytes())
				if stdout.Len() != 0 {
					t.Errorf("stdout not empty on a failure: %s", &stdout)
				}
			}
		})
	}
}

func TestRunIsDeterministic(t *testing.T) {
	var first, second bytes.Buffer
	run(t.Context(), unreadfield.Run, []string{"-dir", "testdata/failing"}, &bytes.Buffer{}, &first)
	run(t.Context(), unreadfield.Run, []string{"-dir", "testdata/failing"}, &bytes.Buffer{}, &second)
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Errorf("two runs differ:\n%s\n%s", &first, &second)
	}
}

func TestRunPassesPolicy(t *testing.T) {
	var gotDir string
	var gotConfigs []unreadfield.Configuration
	var gotTimeout time.Duration
	stub := func(
		_ context.Context, dir string, configs []unreadfield.Configuration, timeout time.Duration,
	) (unreadfield.Report, error) {
		gotDir, gotConfigs, gotTimeout = dir, configs, timeout
		return unreadfield.Report{Configurations: len(configs)}, nil
	}
	if exit := run(t.Context(), stub, []string{"-dir", "testdata/clean"}, &bytes.Buffer{}, &bytes.Buffer{}); exit != 0 {
		t.Fatalf("exit %d", exit)
	}
	if gotDir != "testdata/clean" || gotTimeout != analysisTimeout {
		t.Errorf("dir %q timeout %v", gotDir, gotTimeout)
	}
	if !reflect.DeepEqual(gotConfigs, unreadfield.BuildConfigurations()) {
		t.Errorf("configurations = %v, want BuildConfigurations()", gotConfigs)
	}
}

func TestRunFailureClassification(t *testing.T) {
	analysis := func(err error) error { return &unreadfield.AnalysisError{Configuration: "default", Err: err} }
	for _, tc := range []struct {
		name string
		err  error
		code string
		exit int
	}{
		{name: "timeout", err: context.DeadlineExceeded, code: codeTimeout, exit: contract.ExitNetwork},
		{name: "canceled", err: context.Canceled, code: codeInternal, exit: contract.ExitInternal},
		{name: "permission", err: fs.ErrPermission, code: codePermission, exit: contract.ExitAuth},
		{name: "analysis", err: analysis(errors.New("type error")), code: codeAnalysis, exit: contract.ExitValidation},
		{name: "internal", err: errors.New("boom"), code: codeInternal, exit: contract.ExitInternal},
		{
			name: "analysis wrapping timeout", err: analysis(fmt.Errorf("go list: %w", context.DeadlineExceeded)),
			code: codeTimeout, exit: contract.ExitNetwork,
		},
		{
			name: "analysis wrapping cancel", err: analysis(fmt.Errorf("go list: %w", context.Canceled)),
			code: codeInternal, exit: contract.ExitInternal,
		},
		{
			name: "analysis wrapping permission", err: analysis(fmt.Errorf("open: %w", fs.ErrPermission)),
			code: codePermission, exit: contract.ExitAuth,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := func(
				context.Context, string, []unreadfield.Configuration, time.Duration,
			) (unreadfield.Report, error) {
				return unreadfield.Report{}, tc.err
			}
			var stdout, stderr bytes.Buffer
			if got := run(t.Context(), stub, []string{"-dir", "testdata/clean"}, &stdout, &stderr); got != tc.exit {
				t.Errorf("exit %d, want %d: %s", got, tc.exit, &stderr)
			}
			checkEnvelope(t, &stdout, &stderr, tc.code)
		})
	}
}

func TestRunRejectsInvalidInvocation(t *testing.T) {
	never := func(
		context.Context, string, []unreadfield.Configuration, time.Duration,
	) (unreadfield.Report, error) {
		t.Fatal("runner called for an invalid invocation")
		return unreadfield.Report{}, nil
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"-bogus"}},
		{name: "positional argument", args: []string{"./..."}},
		{name: "not a module root", args: []string{"-dir", "testdata"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(t.Context(), never, tc.args, &stdout, &stderr); got != contract.ExitValidation {
				t.Errorf("exit %d, want %d: %s", got, contract.ExitValidation, &stderr)
			}
			checkEnvelope(t, &stdout, &stderr, codeArtifact)
		})
	}
}

func TestRunWriteFailure(t *testing.T) {
	stub := func(
		context.Context, string, []unreadfield.Configuration, time.Duration,
	) (unreadfield.Report, error) {
		return unreadfield.Report{}, nil
	}
	var stderr bytes.Buffer
	if got := run(t.Context(), stub, []string{"-dir", "testdata/clean"}, failingWriter{}, &stderr); got !=
		contract.ExitInternal {
		t.Errorf("exit %d, want %d", got, contract.ExitInternal)
	}
	checkEnvelope(t, &bytes.Buffer{}, &stderr, codeInternal)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }
