package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden, when set via `go test -update`, rewrites the golden fixtures
// under testdata/ from the current formatter output. It must be package-scoped
// because the flag package registers flags at the package level.
//
//nolint:gochecknoglobals // test-only golden-file update flag must be package-scoped for the flag package
var updateGolden = flag.Bool("update", false, "update golden files in testdata/")

const sampleProfile = `mode: atomic
github.com/example/foo/a.go:1.1,2.10 3 1
github.com/example/foo/b.go:1.1,2.10 2 0
github.com/example/bar/c.go:1.1,2.10 4 1
github.com/example/bar/d.go:5.1,6.10 1 1
`

func TestParseCoverage(t *testing.T) {
	pkgs, err := parseCoverage(strings.NewReader(sampleProfile))
	if err != nil {
		t.Fatalf("parseCoverage returned error: %v", err)
	}

	foo, ok := pkgs["github.com/example/foo"]
	if !ok {
		t.Fatal("expected package github.com/example/foo in result")
	}
	if foo.Stmts != 5 || foo.Covered != 3 {
		t.Fatalf("foo: got Stmts=%d Covered=%d, want 5/3", foo.Stmts, foo.Covered)
	}
	if foo.Pct != 60.0 {
		t.Fatalf("foo: got Pct=%.1f, want 60.0", foo.Pct)
	}

	bar, ok := pkgs["github.com/example/bar"]
	if !ok {
		t.Fatal("expected package github.com/example/bar in result")
	}
	if bar.Stmts != 5 || bar.Covered != 5 {
		t.Fatalf("bar: got Stmts=%d Covered=%d, want 5/5", bar.Stmts, bar.Covered)
	}
	if bar.Pct != 100.0 {
		t.Fatalf("bar: got Pct=%.1f, want 100.0", bar.Pct)
	}
}

func TestParseCoverageMalformed(t *testing.T) {
	const bad = "mode: atomic\nnot-a-valid-coverage-line\n"
	if _, err := parseCoverage(strings.NewReader(bad)); err == nil {
		t.Fatal("expected error on malformed coverage line, got nil")
	}
}

// testConfig returns a deterministic floor configuration used across the
// checkFloors tests so they do not depend on the real repo's calibrated floors.
func testConfig() floorConfig {
	return floorConfig{
		repoWide:     70.0,
		defaultFloor: 25.0,
		perPackage: map[string]float64{
			"github.com/example/foo": 50.0,
			"github.com/example/bar": 80.0,
		},
		excluded: map[string]bool{
			"github.com/example/excluded": true,
		},
	}
}

func pkg(path string, stmts, covered int) PackageCoverage {
	return PackageCoverage{ImportPath: path, Stmts: stmts, Covered: covered, Pct: percent(covered, stmts)}
}

func TestCheckFloors_Pass(t *testing.T) {
	pkgs := map[string]PackageCoverage{
		"github.com/example/foo": pkg("github.com/example/foo", 10, 9), // 90% >= 50%
		"github.com/example/bar": pkg("github.com/example/bar", 10, 9), // 90% >= 80%
	}
	result := checkFloors(pkgs, testConfig())
	if !result.Pass {
		t.Fatalf("expected pass, got violations: %+v", result.Violations)
	}
}

func TestCheckFloors_PackageViolation(t *testing.T) {
	pkgs := map[string]PackageCoverage{
		"github.com/example/foo": pkg("github.com/example/foo", 10, 9), // 90% >= 50%
		"github.com/example/bar": pkg("github.com/example/bar", 10, 5), // 50% < 80%
	}
	result := checkFloors(pkgs, testConfig())
	if result.Pass {
		t.Fatal("expected failure, got pass")
	}
	found := false
	for _, v := range result.Violations {
		if v.Scope == "github.com/example/bar" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a violation naming github.com/example/bar, got %+v", result.Violations)
	}
}

func TestCheckFloors_RepoWideViolation(t *testing.T) {
	// Each package meets its own floor, but the aggregate is below repo-wide.
	pkgs := map[string]PackageCoverage{
		"github.com/example/foo": pkg("github.com/example/foo", 100, 55), // 55% >= 50%
		"github.com/example/bar": pkg("github.com/example/bar", 100, 81), // 81% >= 80%
	}
	// aggregate = 136/200 = 68% < 70%
	result := checkFloors(pkgs, testConfig())
	if result.Pass {
		t.Fatal("expected repo-wide failure, got pass")
	}
	found := false
	for _, v := range result.Violations {
		if v.Scope == "repo-wide" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a repo-wide violation, got %+v", result.Violations)
	}
}

func TestCheckFloors_ExcludedPackage(t *testing.T) {
	pkgs := map[string]PackageCoverage{
		"github.com/example/foo":      pkg("github.com/example/foo", 100, 90),   // 90% >= 50%
		"github.com/example/bar":      pkg("github.com/example/bar", 100, 90),   // 90% >= 80%
		"github.com/example/excluded": pkg("github.com/example/excluded", 1, 0), // 0% but excluded
	}
	result := checkFloors(pkgs, testConfig())
	if !result.Pass {
		t.Fatalf("expected pass (excluded package must not fail per-package gate), got %+v", result.Violations)
	}
	for _, v := range result.Violations {
		if v.Scope == "github.com/example/excluded" {
			t.Fatal("excluded package must not produce a per-package violation")
		}
	}
	if len(result.Excluded) != 1 || result.Excluded[0].ImportPath != "github.com/example/excluded" {
		t.Fatalf("expected excluded package in result.Excluded, got %+v", result.Excluded)
	}
}

func TestCheckFloors_MissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(filepath.Join(t.TempDir(), "does-not-exist.out"), &stdout, &stderr)
	if code != exitBadInput {
		t.Fatalf("run on missing file = %d, want %d", code, exitBadInput)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected nothing on stdout for bad input, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "covercheck:") {
		t.Fatalf("expected diagnostic on stderr, got %q", stderr.String())
	}
}

// goldenPassResult builds a deterministic passing CheckResult used by the pass
// golden test.
func goldenPassResult() CheckResult {
	return checkFloors(map[string]PackageCoverage{
		"github.com/example/foo":      pkg("github.com/example/foo", 1000, 900), // 90%
		"github.com/example/bar":      pkg("github.com/example/bar", 1000, 850), // 85%
		"github.com/example/excluded": pkg("github.com/example/excluded", 100, 0),
	}, testConfig())
}

func goldenFailResult() CheckResult {
	return checkFloors(map[string]PackageCoverage{
		"github.com/example/foo": pkg("github.com/example/foo", 100, 40), // 40% < 50%
		"github.com/example/bar": pkg("github.com/example/bar", 100, 20), // 20% < 80%
	}, testConfig())
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("updating golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s: %v (run `go test -update` to create it)", path, err)
	}
	if got != string(want) {
		t.Fatalf("output mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestFormatOutput_Pass(t *testing.T) {
	var buf bytes.Buffer
	formatPass(&buf, goldenPassResult(), testConfig())
	assertGolden(t, "pass.golden", buf.String())
}

func TestFormatOutput_Fail(t *testing.T) {
	var buf bytes.Buffer
	formatFail(&buf, goldenFailResult())
	assertGolden(t, "fail.golden", buf.String())
}

// writeTempProfile writes content to a temp coverage file and returns its path.
func writeTempProfile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp profile: %v", err)
	}
	return path
}

// TestRun_Pass exercises run end-to-end on a profile that meets every floor and
// asserts the success contract: exit 0, summary on stdout, nothing on stderr.
// internal/config (floor > 0, not excluded) at 100% is robust to floor changes.
func TestRun_Pass(t *testing.T) {
	const profile = "mode: atomic\ngithub.com/rshade/ax-go/internal/config/a.go:1.1,2.10 10 1\n"
	var stdout, stderr bytes.Buffer
	if code := run(writeTempProfile(t, profile), &stdout, &stderr); code != exitOK {
		t.Fatalf("run = %d, want exitOK (%d); stderr=%q", code, exitOK, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("expected pass summary on stdout, got nothing")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr on pass, got %q", stderr.String())
	}
}

// TestRun_Violation exercises run end-to-end on a failing profile and asserts the
// violation contract: exit 1, violations on stderr, nothing on stdout.
func TestRun_Violation(t *testing.T) {
	const profile = "mode: atomic\ngithub.com/rshade/ax-go/internal/config/a.go:1.1,2.10 10 0\n"
	var stdout, stderr bytes.Buffer
	if code := run(writeTempProfile(t, profile), &stdout, &stderr); code != exitViolation {
		t.Fatalf("run = %d, want exitViolation (%d)", code, exitViolation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout on violation, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "FAIL") {
		t.Fatalf("expected violation list on stderr, got %q", stderr.String())
	}
}

// TestRun_MalformedContent verifies a file that exists but holds malformed data
// is bad input (exit 2) with a diagnostic on stderr and nothing on stdout.
func TestRun_MalformedContent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(writeTempProfile(t, "mode: atomic\nnot-a-valid-line\n"), &stdout, &stderr)
	if code != exitBadInput {
		t.Fatalf("run on malformed content = %d, want exitBadInput (%d)", code, exitBadInput)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected nothing on stdout for bad input, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "cannot parse") {
		t.Fatalf("expected parse diagnostic on stderr, got %q", stderr.String())
	}
}

// TestRun_OversizedProfile verifies an over-cap coverage file is rejected as bad
// input (exit 2) rather than read unboundedly.
func TestRun_OversizedProfile(t *testing.T) {
	var b strings.Builder
	b.WriteString("mode: atomic\n")
	const line = "github.com/rshade/ax-go/internal/config/a.go:1.1,2.10 3 1\n"
	for b.Len() <= maxProfileBytes {
		b.WriteString(line)
	}
	var stdout, stderr bytes.Buffer
	code := run(writeTempProfile(t, b.String()), &stdout, &stderr)
	if code != exitBadInput {
		t.Fatalf("run on oversized profile = %d, want exitBadInput (%d)", code, exitBadInput)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected nothing on stdout for oversized input, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "size limit") {
		t.Fatalf("expected size-limit diagnostic on stderr, got %q", stderr.String())
	}
}

func TestParseProfileLine(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantErr bool
		wantPkg string
	}{
		{"valid", "github.com/x/y/f.go:1.1,2.10 3 1", false, "github.com/x/y"},
		{"wrong field count", "not-valid", true, ""},
		{"no colon in block", "github.com/x/y/f.go 3 1", true, ""},
		{"no slash in path", "f.go:1.1,2.10 3 1", true, ""},
		{"non-integer numStmt", "a/b/c.go:1.1,2.10 z 1", true, ""},
		{"non-integer count", "a/b/c.go:1.1,2.10 3 z", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPkg, _, _, err := parseProfileLine(tc.line)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseProfileLine(%q) err = %v, wantErr %v", tc.line, err, tc.wantErr)
			}
			if !tc.wantErr && gotPkg != tc.wantPkg {
				t.Fatalf("parseProfileLine(%q) pkg = %q, want %q", tc.line, gotPkg, tc.wantPkg)
			}
		})
	}
}

func TestPercentZeroTotal(t *testing.T) {
	if got := percent(0, 0); got != 0 {
		t.Fatalf("percent(0, 0) = %v, want 0", got)
	}
	if got := percent(5, 0); got != 0 {
		t.Fatalf("percent(5, 0) = %v, want 0", got)
	}
}

// TestDefaultFloorConfig guards the production policy table against silent edits:
// it must match the AGENTS.md "Coverage Policy" table.
func TestDefaultFloorConfig(t *testing.T) {
	cfg := defaultFloorConfig()
	if cfg.repoWide != repoWideFloor {
		t.Fatalf("repoWide = %v, want %v", cfg.repoWide, repoWideFloor)
	}
	if cfg.defaultFloor != defaultPackageFloor {
		t.Fatalf("defaultFloor = %v, want %v", cfg.defaultFloor, defaultPackageFloor)
	}
	wantPerPackage := map[string]float64{
		"github.com/rshade/ax-go":                       80.0,
		"github.com/rshade/ax-go/examples/integration":  85.0,
		"github.com/rshade/ax-go/internal/cli":          100.0,
		"github.com/rshade/ax-go/internal/cmd/doccover": 45.0,
		"github.com/rshade/ax-go/internal/config":       65.0,
		"github.com/rshade/ax-go/internal/mcp":          90.0,
		"github.com/rshade/ax-go/internal/schema":       95.0,
		"github.com/rshade/ax-go/internal/telemetry":    60.0,
		"github.com/rshade/ax-go/internal/testutil":     25.0,
	}
	for path, want := range wantPerPackage {
		if got, ok := cfg.perPackage[path]; !ok || got != want {
			t.Fatalf("perPackage[%q] = %v (present=%v), want %v", path, got, ok, want)
		}
	}
	if len(cfg.perPackage) != len(wantPerPackage) {
		t.Fatalf("perPackage has %d entries, want %d", len(cfg.perPackage), len(wantPerPackage))
	}
	if len(cfg.excluded) != 0 {
		t.Fatalf("excluded has %d entries, want 0", len(cfg.excluded))
	}
}

func FuzzParseCoverageProfile(f *testing.F) {
	f.Add([]byte(sampleProfile))
	f.Add([]byte("mode: atomic\n"))
	f.Add([]byte(""))
	f.Add([]byte("garbage\x00\xff bytes"))
	f.Add([]byte("mode: atomic\na/b/c 3 1\n"))             // no colon in block
	f.Add([]byte("mode: atomic\nfile.go:1.1,2.10 3 1\n"))  // no slash before colon
	f.Add([]byte("mode: atomic\na/b/c.go:1.1,2.10 z 1\n")) // non-integer statement count
	f.Fuzz(func(_ *testing.T, data []byte) {
		// Must never panic, regardless of input. An error return is fine.
		_, _ = parseCoverage(bytes.NewReader(data))
	})
}
