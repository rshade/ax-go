package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/rshade/ax-go/contract"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRunGolden(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		exit        int
	}{
		{"pass", "exported.json", contract.ExitSuccess},
		{"fail", "internal.json", contract.ExitValidation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var calls []string
			analyze := func(_ context.Context, tags string) ([]byte, error) {
				calls = append(calls, tags)
				return fixture(t, tc.input), nil
			}
			got := run(t.Context(), analyze, nil, &stdout, &stderr)
			if got != tc.exit {
				t.Fatalf("exit %d, want %d: %s", got, tc.exit, &stderr)
			}
			wantTags := []string{"", "ax_no_grpc", "ax_no_otlp", "ax_no_grpc,ax_no_otlp"}
			if !reflect.DeepEqual(calls, wantTags) {
				t.Errorf("configurations = %q", calls)
			}
			if want := fixture(t, tc.name+".stdout.golden"); !bytes.Equal(stdout.Bytes(), want) {
				t.Errorf("stdout = %s, want %s", &stdout, want)
			}
			if want := fixture(t, tc.name+".stderr.golden"); !bytes.Equal(stderr.Bytes(), want) {
				t.Errorf("stderr = %s, want %s", &stderr, want)
			}
		})
	}
}

func TestReachableInOneConfiguration(t *testing.T) {
	for _, liveTags := range []string{"", "ax_no_grpc", "ax_no_otlp", "ax_no_grpc,ax_no_otlp"} {
		t.Run("live_"+liveTags, func(t *testing.T) {
			analyze := func(_ context.Context, tags string) ([]byte, error) {
				if tags == liveTags {
					return fixture(t, "exported.json"), nil
				}
				return fixture(t, "internal.json"), nil
			}
			var stdout, stderr bytes.Buffer
			if exit := run(t.Context(), analyze, nil, &stdout, &stderr); exit != 0 || stderr.Len() != 0 {
				t.Fatalf("exit %d: %s", exit, &stderr)
			}
			if !bytes.Equal(stdout.Bytes(), fixture(t, "pass.stdout.golden")) {
				t.Fatalf("unexpected payload: %s", &stdout)
			}
		})
	}
}

func TestPolicy(t *testing.T) {
	for _, tc := range []struct {
		pkg, name string
		want      bool
	}{
		{"github.com/rshade/ax-go", "ParseMode", false},
		{"github.com/rshade/ax-go/contract", "SpanIDFromContext", false},
		{"github.com/rshade/ax-go/mcp", "WithAllowNonLoopback", false},
		{"github.com/rshade/ax-go", "unused", true},
		{"github.com/rshade/ax-go_test", "unusedTestHelper", true},
		{"github.com/rshade/ax-go/contract_test", "unusedTestHelper", true},
		{"github.com/rshade/ax-go/internal/cli", "Unused", true},
		{"github.com/rshade/ax-go/internal", "Unused", true},
		{"github.com/rshade/ax-go/internalized", "Unused", false},
		{"github.com/rshade/ax-go", "Public.private", true},
		{"github.com/rshade/ax-go", "private.Public", false},
		{"github.com/rshade/ax-go", "Éxported", false},
		{"github.com/elsewhere/internal/helper", "Unused", false},
		{"github.com/rshade/ax-go-extra/internal", "Unused", false},
	} {
		t.Run(tc.pkg+"/"+tc.name, func(t *testing.T) {
			if got := inScope(tc.pkg, tc.name); got != tc.want {
				t.Errorf("inScope = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		data string
		err  error
		exit int
		code string
	}{
		{name: "flag", args: []string{"--bogus"}, exit: 2, code: "invalid_deadcode_artifact"},
		{name: "argument", args: []string{"extra"}, exit: 2, code: "invalid_deadcode_artifact"},
		{name: "missing_tool", err: os.ErrNotExist, exit: 2, code: "deadcode_analysis_failed"},
		{name: "permission", err: fs.ErrPermission, exit: 4, code: "deadcode_permission"},
		{name: "build", err: errors.New("does not compile"), exit: 2, code: "deadcode_analysis_failed"},
		{name: "cancel", err: context.Canceled, exit: 1, code: "deadcode_internal"},
		{name: "timeout", err: context.DeadlineExceeded, exit: 3, code: "deadcode_timeout"},
		{name: "malformed", data: `[{`, exit: 2, code: "invalid_deadcode_artifact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			calls := 0
			analyze := func(_ context.Context, _ string) ([]byte, error) { calls++; return []byte(tc.data), tc.err }
			got := run(t.Context(), analyze, tc.args, &stdout, &stderr)
			if got != tc.exit || stdout.Len() != 0 {
				t.Fatalf("exit %d stdout %q stderr %s", got, &stdout, &stderr)
			}
			var env contract.Error
			if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if env.ErrorCode != tc.code {
				t.Errorf("code = %s, want %s", env.ErrorCode, tc.code)
			}
			if bytes.Count(stderr.Bytes(), []byte("\n")) != 1 {
				t.Errorf("not one line: %q", &stderr)
			}
			if len(tc.args) != 0 && calls != 0 {
				t.Error("invalid arguments invoked analyzer")
			}
		})
	}
}

func TestParseReport(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"null", "null", true}, {"empty", "[]", true},
		{"missing", "", false}, {"object", "{}", false},
		{"trailing", "[] []", false}, {"unknown", `[{"Other":1}]`, false},
		{"missing_fields", `[{"Path":"github.com/rshade/ax-go"}]`, false},
		{"missing_position", `[{"Name":"ax","Path":"github.com/rshade/ax-go","Funcs":[{"Name":"unused"}]}]`, false},
		{"oversized", strings.Repeat(" ", maxReportBytes+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseReport([]byte(tc.input))
			if (err == nil) != tc.valid {
				t.Errorf("error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

func FuzzParseReport(f *testing.F) {
	f.Add([]byte("null"))
	f.Add([]byte("[]"))
	f.Add(
		[]byte(
			`[{"Name":"ax","Path":"github.com/rshade/ax-go","Funcs":[{"Name":"unused","Position":{"File":"mode.go","Line":1,"Col":1},"Generated":false,"Marker":false}]}]`,
		),
	)
	f.Fuzz(func(_ *testing.T, input []byte) { _, _ = parseReport(input) })
}

func TestBuildTagMatrix(t *testing.T) {
	data, err := os.ReadFile("../../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if matrix, ok := strings.CutPrefix(line, "BUILD_TAG_MATRIX?="); ok {
			want := strings.Fields(matrix)
			for i := range want {
				if want[i] == "none" {
					want[i] = ""
				}
			}
			if !reflect.DeepEqual(buildTags(), want) {
				t.Fatalf("gate tags %q differ from Makefile %q", buildTags(), want)
			}
			return
		}
	}
	t.Fatal("Makefile has no build-tag matrix")
}

func TestLaterAnalysisFailure(t *testing.T) {
	calls := 0
	analyze := func(_ context.Context, _ string) ([]byte, error) {
		calls++
		if calls == 4 {
			return nil, errors.New("fourth build failed")
		}
		return []byte("null"), nil
	}
	var stdout, stderr bytes.Buffer
	if exit := run(t.Context(), analyze, nil, &stdout, &stderr); exit != contract.ExitValidation || stdout.Len() != 0 {
		t.Fatalf("exit %d stdout %q stderr %s", exit, &stdout, &stderr)
	}
	if calls != 4 || !strings.Contains(stderr.String(), "fourth build failed") {
		t.Fatalf("calls %d stderr %s", calls, &stderr)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestWriteFailure(t *testing.T) {
	analyze := func(_ context.Context, _ string) ([]byte, error) { return []byte("null"), nil }
	var stderr bytes.Buffer
	if exit := run(
		t.Context(),
		analyze,
		nil,
		failingWriter{},
		&stderr,
	); exit != contract.ExitInternal ||
		!strings.Contains(stderr.String(), "could not write result") {
		t.Fatalf("exit %d stderr %s", exit, &stderr)
	}
	if exit := run(
		t.Context(),
		analyze,
		[]string{"--invalid"},
		failingWriter{},
		failingWriter{},
	); exit != contract.ExitInternal {
		t.Fatalf("exit %d", exit)
	}
}
