package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/rshade/ax-go/contract"
)

// fakeBuilder writes a file of the requested size for each probe package instead
// of invoking the Go toolchain, so the three outcomes can be exercised
// deterministically and in milliseconds. The real toolchain path is covered by
// TestRunAgainstTheRealProbes.
func fakeBuilder(sizes map[string]int, failFor map[string]error) builder {
	return func(_ context.Context, _, pkg, out string) error {
		if err, ok := failFor[pkg]; ok {
			return err
		}
		size, ok := sizes[pkg]
		if !ok {
			return errors.New("fakeBuilder: no size configured for " + pkg)
		}
		return os.WriteFile(out, bytes.Repeat([]byte{0}, size), 0o600)
	}
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()

	trimmed := strings.TrimRight(raw, "\n")
	if strings.Contains(trimmed, "\n") {
		t.Fatalf("stderr carried %d lines, want exactly one envelope:\n%s",
			strings.Count(trimmed, "\n")+1, raw)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		t.Fatalf("stderr was not a JSON envelope: %v\ngot: %s", err, raw)
	}
	return env
}

// TestRunOutcomes covers the gate's three distinguishable failures plus the pass
// path. Keeping them distinct is the point of the test: a maintainer resolves a
// build failure, a ceiling breach, and a ratio breach in three completely
// different ways, and a gate that reported one code for all three would send them
// down the wrong path.
func TestRunOutcomes(t *testing.T) {
	const (
		iso  = "./examples/logging"
		root = "./examples/rootlogging"
	)
	buildErr := errors.New("probe does not compile")

	cases := []struct {
		name        string
		sizes       map[string]int
		failFor     map[string]error
		ceiling     int64
		wantExit    int
		wantCode    string
		wantMessage string
	}{
		{
			name:     "pass",
			sizes:    map[string]int{iso: 2_000_000, root: 12_000_000},
			ceiling:  3_000_000,
			wantExit: contract.ExitSuccess,
		},
		{
			name:     "pass_exactly_at_ceiling",
			sizes:    map[string]int{iso: 3_000_000, root: 12_000_000},
			ceiling:  3_000_000,
			wantExit: contract.ExitSuccess,
		},
		{
			name:     "ceiling_breach",
			sizes:    map[string]int{iso: 3_000_001, root: 40_000_000},
			ceiling:  3_000_000,
			wantExit: contract.ExitValidation,
			wantCode: codeCeiling,
			// A ratio of 92% here proves the ceiling is checked independently:
			// this case passes the ratio budget comfortably and must still fail.
			wantMessage: "absolute size ceiling",
		},
		{
			name:     "ratio_breach",
			sizes:    map[string]int{iso: 2_900_000, root: 4_000_000},
			ceiling:  3_000_000,
			wantExit: contract.ExitValidation,
			wantCode: codeRatio,
			// 27.5% reduction, comfortably under the ceiling: the ratio is
			// checked independently in the other direction too.
			wantMessage: "not sufficiently smaller",
		},
		{
			name:        "isolated_probe_build_failure",
			sizes:       map[string]int{root: 12_000_000},
			failFor:     map[string]error{iso: buildErr},
			ceiling:     3_000_000,
			wantExit:    contract.ExitValidation,
			wantCode:    codeBuildFailed,
			wantMessage: "could not build the size probe",
		},
		{
			name:        "root_probe_build_failure",
			sizes:       map[string]int{iso: 2_000_000},
			failFor:     map[string]error{root: buildErr},
			ceiling:     3_000_000,
			wantExit:    contract.ExitValidation,
			wantCode:    codeBuildFailed,
			wantMessage: "could not build the size probe",
		},
		{
			name:        "toolchain_permission_denied",
			sizes:       map[string]int{root: 12_000_000},
			failFor:     map[string]error{iso: fs.ErrPermission},
			ceiling:     3_000_000,
			wantExit:    contract.ExitAuth,
			wantCode:    codeSizePermission,
			wantMessage: "permission denied",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cfg := runConfig{
				Dir:          t.TempDir(),
				Build:        fakeBuilder(tc.sizes, tc.failFor),
				Ceiling:      tc.ceiling,
				MinReduction: 75.0,
			}

			got := run(context.Background(), cfg, nil, &stdout, &stderr)
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stderr: %s)", got, tc.wantExit, stderr.String())
			}

			if tc.wantExit == contract.ExitSuccess {
				assertPassStreams(t, &stdout, &stderr)
				return
			}
			assertFailureStreams(t, &stdout, &stderr, tc.wantCode, tc.wantMessage)
		})
	}
}

// assertPassStreams pins the pass half of the stream contract: exactly one
// minified JSON object on stdout, nothing on stderr.
func assertPassStreams(t *testing.T, stdout, stderr *bytes.Buffer) {
	t.Helper()

	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty on a pass", stderr.String())
	}

	trimmed := strings.TrimRight(stdout.String(), "\n")
	if strings.Contains(trimmed, "\n") {
		t.Fatalf("stdout carried more than one line:\n%s", stdout.String())
	}

	var doc result
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		t.Fatalf("stdout was not a JSON document: %v\ngot: %s", err, stdout.String())
	}
	if doc.Status != statusPass {
		t.Errorf("status = %q, want %q", doc.Status, statusPass)
	}
	// Both measurements and the computed reduction must be reported, so a
	// reviewer reading CI output can see the headroom without rerunning anything.
	if doc.IsolatedBytes <= 0 || doc.RootBytes <= 0 {
		t.Errorf("isolated_bytes = %d, root_bytes = %d; both must be reported",
			doc.IsolatedBytes, doc.RootBytes)
	}
	if doc.ReductionPercent <= 0 {
		t.Errorf("reduction_percent = %v, want the computed reduction", doc.ReductionPercent)
	}
}

// assertFailureStreams pins the failure half: nothing on stdout, exactly one
// ax.Error envelope on stderr carrying the expected code.
func assertFailureStreams(t *testing.T, stdout, stderr *bytes.Buffer, wantCode, wantMessage string) {
	t.Helper()

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on a failure", stdout.String())
	}

	env := decodeEnvelope(t, stderr.String())
	if env["error_code"] != wantCode {
		t.Errorf("error_code = %v, want %q", env["error_code"], wantCode)
	}
	message, _ := env["message"].(string)
	if wantMessage != "" && !strings.Contains(message, wantMessage) {
		t.Errorf("message = %q, want it to contain %q", message, wantMessage)
	}
	if env["tool"] != "sizecheck" {
		t.Errorf("tool = %v, want sizecheck", env["tool"])
	}
}

// TestRunRejectsInvalidInvocation covers the flag-handling contract shared with
// the sibling gates.
func TestRunRejectsInvalidInvocation(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "unknown_flag", args: []string{"-nope"}},
		{name: "positional_argument", args: []string{"unexpected"}},
		{name: "flag_and_positional", args: []string{"-nope", "unexpected"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cfg := runConfig{
				Dir:   t.TempDir(),
				Build: fakeBuilder(map[string]int{}, nil),
			}

			if got := run(context.Background(), cfg, tc.args, &stdout, &stderr); got != contract.ExitValidation {
				t.Fatalf("exit = %d, want %d", got, contract.ExitValidation)
			}
			assertFailureStreams(t, &stdout, &stderr, codeArtifact, "")
		})
	}
}

// TestReductionPercent pins the arithmetic, including the undefined case. A
// zero-byte root probe can only mean the build produced nothing, so it is an
// internal failure rather than a silent 0% or 100%.
func TestReductionPercent(t *testing.T) {
	cases := []struct {
		name     string
		isolated int64
		root     int64
		want     float64
		wantExit int
	}{
		{name: "eighty_percent", isolated: 2_000_000, root: 10_000_000, want: 80},
		{name: "no_reduction", isolated: 10_000_000, root: 10_000_000, want: 0},
		{name: "isolated_larger", isolated: 12_000_000, root: 10_000_000, want: -20},
		{name: "zero_root", isolated: 1, root: 0, wantExit: contract.ExitInternal},
		{name: "negative_root", isolated: 1, root: -1, wantExit: contract.ExitInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got, code := reductionPercent(context.Background(), &stderr, tc.isolated, tc.root)
			if code != tc.wantExit {
				t.Fatalf("exit = %d, want %d", code, tc.wantExit)
			}
			if tc.wantExit != 0 {
				return
			}
			// Compared with a tolerance because the ratio is floating-point
			// division: 1 - 12e6/10e6 lands on -19.999999999999996, not -20.
			// The budget comparison in run has no such problem — it compares
			// against a threshold rather than for equality — so an exact
			// assertion here would be testing IEEE 754, not the gate.
			const epsilon = 1e-9
			if diff := got - tc.want; diff > epsilon || diff < -epsilon {
				t.Fatalf("reduction = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDefaultBudgetsAreTheDocumentedOnes guards the constants themselves against
// a drive-by edit. They are policy, and the doc comments explain that the ceiling
// and the ratio are adjusted under different rules; changing either should be a
// deliberate act that also updates this test.
func TestDefaultBudgetsAreTheDocumentedOnes(t *testing.T) {
	if maxIsolatedBinaryBytes != 3_000_000 {
		t.Errorf("maxIsolatedBinaryBytes = %d, want 3000000 (SC-001)", maxIsolatedBinaryBytes)
	}
	if minReductionPercent != 75.0 {
		t.Errorf("minReductionPercent = %v, want 75.0 (SC-002; lowering it is a spec change)", minReductionPercent)
	}
}

// TestResolveAppliesDefaults pins that a zero-value config falls back to the
// hardcoded budgets and probe paths, which is how main invokes it.
func TestResolveAppliesDefaults(t *testing.T) {
	got := runConfig{}.resolve()

	if got.Dir != "." {
		t.Errorf("Dir = %q, want %q", got.Dir, ".")
	}
	if got.Build == nil {
		t.Error("Build = nil, want the real go build path")
	}
	if got.Ceiling != maxIsolatedBinaryBytes {
		t.Errorf("Ceiling = %d, want %d", got.Ceiling, maxIsolatedBinaryBytes)
	}
	if got.MinReduction != minReductionPercent {
		t.Errorf("MinReduction = %v, want %v", got.MinReduction, minReductionPercent)
	}
	if got.IsolatedPkg != isolatedProbe || got.RootPkg != rootProbe {
		t.Errorf("probes = %q/%q, want %q/%q", got.IsolatedPkg, got.RootPkg, isolatedProbe, rootProbe)
	}
}

// TestRunAgainstTheRealProbes exercises the gate end to end against the committed
// programs and the real toolchain.
//
// The fake-builder cases above prove the DECISION logic; this proves the
// MEASUREMENT logic — that the probe paths resolve, the production build flags
// are accepted, and the real binaries meet both budgets. Without it the gate could
// pass its whole test suite while pointing at a package that no longer exists.
func TestRunAgainstTheRealProbes(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two binaries with the real toolchain")
	}

	var stdout, stderr bytes.Buffer
	cfg := runConfig{Dir: repoRoot(t)}

	if got := run(context.Background(), cfg, nil, &stdout, &stderr); got != contract.ExitSuccess {
		t.Fatalf("exit = %d, want 0\nstderr: %s", got, stderr.String())
	}
	assertPassStreams(t, &stdout, &stderr)

	var doc result
	if err := json.Unmarshal([]byte(strings.TrimRight(stdout.String(), "\n")), &doc); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	t.Logf("isolated=%d bytes root=%d bytes reduction=%.2f%%",
		doc.IsolatedBytes, doc.RootBytes, doc.ReductionPercent)
}

// TestProbeSourcesDifferOnlyBySurface enforces the invariant the size ratio
// depends on: after stripping package docs and rewriting the surface import and
// qualifier to a common token, examples/logging and examples/rootlogging are
// identical. Anything more makes the ratio measure something other than the
// import boundary.
func TestProbeSourcesDifferOnlyBySurface(t *testing.T) {
	root := repoRoot(t)
	isolated, err := os.ReadFile(root + "/examples/logging/main.go")
	if err != nil {
		t.Fatalf("read isolated probe: %v", err)
	}
	rootFacade, err := os.ReadFile(root + "/examples/rootlogging/main.go")
	if err != nil {
		t.Fatalf("read root probe: %v", err)
	}

	gotIsolated := normalizeProbeSource(string(isolated))
	gotRoot := normalizeProbeSource(string(rootFacade))
	if gotIsolated != gotRoot {
		t.Fatalf(
			"size probes diverge beyond the import surface after normalization:\n--- isolated ---\n%s\n--- root ---\n%s",
			gotIsolated,
			gotRoot,
		)
	}
}

// normalizeProbeSource strips the package doc comment and rewrites either probe
// surface (logging or root ax) to a common token so a residual source diff is
// only the import boundary if the programs stay otherwise identical.
func normalizeProbeSource(src string) string {
	// Drop everything up to and including the package clause's preceding docs.
	const pkg = "package main\n"
	if i := strings.Index(src, pkg); i >= 0 {
		src = src[i:]
	}

	replacements := []struct{ old, new string }{
		{`"github.com/rshade/ax-go/logging"`, `"SURFACE"`},
		{`ax "github.com/rshade/ax-go"`, `"SURFACE"`},
		{"logging.", "surface."},
		{"ax.", "surface."},
	}
	for _, r := range replacements {
		src = strings.ReplaceAll(src, r.old, r.new)
	}
	return src
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(dir + "/go.mod"); statErr == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == "" || parent == dir {
			t.Fatal("go.mod not found above working directory")
		}
		dir = parent
	}
}
