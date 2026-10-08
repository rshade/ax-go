package unreadfield

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildTagMatrix(t *testing.T) {
	data, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, cfg := range BuildConfigurations() {
		got = append(got, strings.Join(cfg.Tags, ","))
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if matrix, ok := strings.CutPrefix(line, "BUILD_TAG_MATRIX?="); ok {
			want := strings.Fields(matrix)
			for i := range want {
				if want[i] == "none" {
					want[i] = ""
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("BuildConfigurations tags %q differ from Makefile %q", got, want)
			}
			return
		}
	}
	t.Fatal("Makefile has no build-tag matrix")
}

func TestRunIntersectsConfigurations(t *testing.T) {
	defaultOnly := []Configuration{{Name: "default"}}
	for _, tc := range []struct {
		name    string
		dir     string
		configs []Configuration
		want    []string
	}{
		{name: "read only in tests", dir: "testdata/testonlyread", configs: BuildConfigurations()},
		{name: "read only under a tag", dir: "testdata/taggedread", configs: BuildConfigurations()},
		{
			name:    "tagged read invisible to default alone",
			dir:     "testdata/taggedread",
			configs: defaultOnly,
			want:    []string{"a.go:4:2: taggedread.flags.grpc (assigned at a.go:7:34)"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Run(t.Context(), tc.dir, tc.configs, time.Minute)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			var got []string
			for _, f := range report.Findings {
				got = append(got, f.String())
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("findings = %q, want %q", got, tc.want)
			}
			if report.Configurations != len(tc.configs) {
				t.Errorf("Configurations = %d, want %d", report.Configurations, len(tc.configs))
			}
		})
	}
}

func TestRunCounts(t *testing.T) {
	report, err := Run(t.Context(), "testdata/testonlyread", BuildConfigurations(), time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Two units: testonlyread and testonlyread_test. One field: hidden.Level is
	// out of scope because export_test.go exposes hidden through an exported
	// alias, leaving opts.debug.
	if report.Packages != 2 || report.Fields != 1 {
		t.Errorf("Packages = %d, Fields = %d, want 2 and 1", report.Packages, report.Fields)
	}
}

func TestRunFailures(t *testing.T) {
	expired, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()
	sleeping := func(ctx context.Context, _ string, _ []string) ([]byte, error) {
		// A real child process killed by the context returns *exec.ExitError
		// ("signal: killed"), not the context error.
		return exec.CommandContext(ctx, "sleep", "30").Output()
	}
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		dir     string
		list    lister
		timeout time.Duration
		target  error
	}{
		{name: "type error", ctx: t.Context(), dir: "testdata/broken", list: goList, timeout: time.Minute},
		{
			name: "expired context", ctx: expired, dir: "testdata/testonlyread", list: goList,
			timeout: time.Minute, target: context.DeadlineExceeded,
		},
		{
			name: "timeout during go list", ctx: t.Context(), dir: "testdata/testonlyread", list: sleeping,
			timeout: 50 * time.Millisecond, target: context.DeadlineExceeded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runWith(tc.ctx, tc.dir, BuildConfigurations(), tc.timeout, tc.list)
			var analysisErr *AnalysisError
			if !errors.As(err, &analysisErr) {
				t.Fatalf("error = %v, want *AnalysisError", err)
			}
			if analysisErr.Configuration != "default" {
				t.Errorf("Configuration = %q, want the first configuration", analysisErr.Configuration)
			}
			if tc.target != nil && !errors.Is(err, tc.target) {
				t.Errorf("error = %v, want it to wrap %v", err, tc.target)
			}
		})
	}
}

func TestRunRejectsMissingDir(t *testing.T) {
	if _, err := Run(t.Context(), "testdata/does-not-exist", BuildConfigurations(), time.Minute); err == nil {
		t.Fatal("Run succeeded on a missing directory")
	}
}
