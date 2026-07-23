package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseConfigSymbol is the required-symbol fixture used across the report and
// hasExample cases.
const (
	parseConfigSymbol          = "ParseConfig"
	qualifiedParseConfigSymbol = "ax.ParseConfig"
)

// pkgSet builds the per-package map shape report now takes, so the cases below
// stay readable.
func pkgSet(entries map[string][]string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for pkg, names := range entries {
		set := map[string]bool{}
		for _, name := range names {
			set[name] = true
		}
		out[pkg] = set
	}
	return out
}

func TestReportExitCodes(t *testing.T) {
	cases := []struct {
		name         string
		required     []string
		exported     map[string][]string
		examples     map[string][]string
		baseline     map[string]bool
		wantExit     int
		wantContains string
	}{
		{
			name:         "covered symbol passes",
			required:     []string{qualifiedParseConfigSymbol},
			exported:     map[string][]string{"ax": {parseConfigSymbol}},
			examples:     map[string][]string{"ax": {"ExampleParseConfig"}},
			baseline:     map[string]bool{},
			wantExit:     0,
			wantContains: "OK: no example-coverage regressions",
		},
		{
			name:         "missing without baseline is a regression",
			required:     []string{qualifiedParseConfigSymbol},
			exported:     map[string][]string{"ax": {parseConfigSymbol}},
			examples:     map[string][]string{},
			baseline:     map[string]bool{},
			wantExit:     1,
			wantContains: "NEW regression",
		},
		{
			name:         "missing with baseline entry is exempt",
			required:     []string{qualifiedParseConfigSymbol},
			exported:     map[string][]string{"ax": {parseConfigSymbol}},
			examples:     map[string][]string{},
			baseline:     map[string]bool{qualifiedParseConfigSymbol: true},
			wantExit:     0,
			wantContains: "baseline-exempt",
		},
		{
			name:         "stale baseline entry fails the ratchet",
			required:     []string{qualifiedParseConfigSymbol},
			exported:     map[string][]string{"ax": {parseConfigSymbol}},
			examples:     map[string][]string{"ax": {"ExampleParseConfig"}},
			baseline:     map[string]bool{qualifiedParseConfigSymbol: true},
			wantExit:     1,
			wantContains: "prune",
		},
		{
			name:         "unknown required symbol fails",
			required:     []string{"ax.RenamedAway"},
			exported:     map[string][]string{},
			examples:     map[string][]string{},
			baseline:     map[string]bool{"ax.RenamedAway": true},
			wantExit:     1,
			wantContains: "renamed or removed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got := report(&out, tc.required, pkgSet(tc.exported), pkgSet(tc.examples), tc.baseline)
			if got != tc.wantExit {
				t.Fatalf("report exit = %d, want %d\noutput:\n%s", got, tc.wantExit, out.String())
			}
			if !strings.Contains(out.String(), tc.wantContains) {
				t.Fatalf("report output missing %q:\n%s", tc.wantContains, out.String())
			}
		})
	}
}

// TestReportDoesNotLetOnePackageSatisfyAnothersRequirement is the decisive case,
// and the entire reason required symbols became package-qualified.
//
// Root has had an ExampleNewLogger for a long time. If required symbols were bare
// names checked against one flat union of every scanned directory's examples,
// pointing the scanner at logging/ would leave root's example satisfying
// logging's requirement — the contract would be stated in the docs, the gate
// would report OK, and the new surface's example would never be enforced. A green
// gate is the failure mode here, not a red one, because a green gate is trusted.
//
// The assertion is therefore that the gate FAILS in exactly that configuration.
func TestReportDoesNotLetOnePackageSatisfyAnothersRequirement(t *testing.T) {
	required := []string{"ax.NewLogger", "logging.NewLogger"}
	exported := pkgSet(map[string][]string{
		"ax":      {"NewLogger"},
		"logging": {"NewLogger"},
	})
	// Root has the example; the isolated surface does not.
	examples := pkgSet(map[string][]string{
		"ax": {"ExampleNewLogger"},
	})

	var out bytes.Buffer
	got := report(&out, required, exported, examples, map[string]bool{})

	if got != 1 {
		t.Fatalf(
			"report exit = %d, want 1: root's ExampleNewLogger must NOT satisfy logging.NewLogger\noutput:\n%s",
			got, out.String(),
		)
	}
	if !strings.Contains(out.String(), "logging.NewLogger") {
		t.Fatalf("report output does not name logging.NewLogger as the uncovered symbol:\n%s", out.String())
	}
	if strings.Contains(out.String(), "- ax.NewLogger") {
		t.Fatalf("report wrongly flagged ax.NewLogger, which does have an example:\n%s", out.String())
	}
}

// TestReportRequiresAnExampleInEachPackage is the complement: once both packages
// carry their own example, both requirements are satisfied and the gate passes.
// Without this, the test above would also pass on a gate that simply always
// failed.
func TestReportRequiresAnExampleInEachPackage(t *testing.T) {
	required := []string{"ax.NewLogger", "logging.NewLogger"}
	exported := pkgSet(map[string][]string{
		"ax":      {"NewLogger"},
		"logging": {"NewLogger"},
	})
	examples := pkgSet(map[string][]string{
		"ax":      {"ExampleNewLogger"},
		"logging": {"ExampleNewLogger"},
	})

	var out bytes.Buffer
	if got := report(&out, required, exported, examples, map[string]bool{}); got != 0 {
		t.Fatalf("report exit = %d, want 0 when each package has its own example\noutput:\n%s", got, out.String())
	}
}

// TestReportScopesUnknownSymbolsPerPackage pins that the exported-symbol check is
// package-scoped too. A symbol exported by root but absent from logging must be
// reported as unknown for logging, not silently accepted because some scanned
// package happens to export that name.
func TestReportScopesUnknownSymbolsPerPackage(t *testing.T) {
	required := []string{"logging.WithLokiFromEnv"}
	exported := pkgSet(map[string][]string{
		"ax": {"WithLokiFromEnv"},
	})
	examples := pkgSet(map[string][]string{
		"ax": {"ExampleWithLokiFromEnv"},
	})

	var out bytes.Buffer
	got := report(&out, required, exported, examples, map[string]bool{})
	if got != 1 {
		t.Fatalf(
			"report exit = %d, want 1 for a symbol not exported by its own package\noutput:\n%s",
			got, out.String(),
		)
	}
	if !strings.Contains(out.String(), "renamed or removed") {
		t.Fatalf("report output did not flag the symbol as not exported:\n%s", out.String())
	}
}

// TestReportBaselineIsPackageQualified pins that ratchet exemptions are scoped to
// one package. Baselining ax.NewLogger must not exempt logging.NewLogger.
func TestReportBaselineIsPackageQualified(t *testing.T) {
	required := []string{"ax.NewLogger", "logging.NewLogger"}
	exported := pkgSet(map[string][]string{
		"ax":      {"NewLogger"},
		"logging": {"NewLogger"},
	})

	var out bytes.Buffer
	got := report(&out, required, exported, pkgSet(nil), map[string]bool{"ax.NewLogger": true})

	if got != 1 {
		t.Fatalf(
			"report exit = %d, want 1: an ax baseline entry must not exempt logging\noutput:\n%s",
			got, out.String(),
		)
	}
	if !strings.Contains(out.String(), "logging.NewLogger") {
		t.Fatalf("report output does not name logging.NewLogger:\n%s", out.String())
	}
}

// TestReadBaselineMigratesUnqualifiedLines covers the format transition. Entries
// written before qualification existed necessarily referred to the only package
// that was scanned, so prefixing them with the root alias is deterministic and
// keeps the ratchet one-way across the change — a reset would silently forgive
// every previously-baselined gap.
func TestReadBaselineMigratesUnqualifiedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.txt")
	content := strings.Join([]string{
		"# a comment",
		"",
		"ParseConfig",        // legacy, unqualified
		"ax.PatchConfig",     // already qualified
		"logging.NewLogger",  // qualified, non-root package
		"  ResolveVersion  ", // legacy with surrounding whitespace
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	got, err := readBaseline(path)
	if err != nil {
		t.Fatalf("readBaseline: %v", err)
	}

	want := map[string]bool{
		"ax.ParseConfig":    true,
		"ax.PatchConfig":    true,
		"logging.NewLogger": true,
		"ax.ResolveVersion": true,
	}
	if len(got) != len(want) {
		t.Fatalf("baseline has %d entries, want %d: %v", len(got), len(want), got)
	}
	for entry := range want {
		if !got[entry] {
			t.Errorf("baseline entry %q absent; got %v", entry, got)
		}
	}
}

// TestSplitQualified pins the parsing rule directly, including the root-package
// default that makes the migration above deterministic.
func TestSplitQualified(t *testing.T) {
	cases := []struct {
		entry      string
		wantPkg    string
		wantSymbol string
	}{
		{entry: "ax.NewLogger", wantPkg: "ax", wantSymbol: "NewLogger"},
		{entry: "logging.NewLogger", wantPkg: "logging", wantSymbol: "NewLogger"},
		{entry: "NewLogger", wantPkg: rootPackageAlias, wantSymbol: "NewLogger"},
	}

	for _, tc := range cases {
		t.Run(tc.entry, func(t *testing.T) {
			gotPkg, gotSymbol := splitQualified(tc.entry)
			if gotPkg != tc.wantPkg || gotSymbol != tc.wantSymbol {
				t.Fatalf("splitQualified(%q) = (%q, %q), want (%q, %q)",
					tc.entry, gotPkg, gotSymbol, tc.wantPkg, tc.wantSymbol)
			}
		})
	}
}

// TestRequiredSymbolsAreAllQualified guards the required set against a bare name
// creeping back in. One unqualified entry silently reverts to root-package
// attribution, which is the failure this whole change exists to prevent.
func TestRequiredSymbolsAreAllQualified(t *testing.T) {
	for _, entry := range requiredSymbols() {
		if !strings.Contains(entry, ".") {
			t.Errorf(
				"required symbol %q is unqualified; it would be attributed to the root package "+
					"and could be satisfied by the wrong package's example",
				entry,
			)
		}
	}
}

// TestRequiredSymbolsCoverEveryScannedPackage pins that each scanned directory
// actually has at least one requirement. Scanning a package that nothing requires
// is a gate doing work while enforcing nothing.
func TestRequiredSymbolsCoverEveryScannedPackage(t *testing.T) {
	required := map[string]bool{}
	for _, entry := range requiredSymbols() {
		pkg, _ := splitQualified(entry)
		required[pkg] = true
	}

	for alias := range scannedPackages() {
		if !required[alias] {
			t.Errorf("package %q is scanned but has no required symbol; the scan enforces nothing", alias)
		}
	}
}

func TestCollectVerifiedExamplesRequiresOutputComment(t *testing.T) {
	const src = `package ax_test

import "fmt"

func ExampleVerified() {
	fmt.Println("ok")
	// Output: ok
}

func ExampleEmptyOutput() {
	// Output:
}

func ExampleUnverified() {
	fmt.Println("compiles but never runs")
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x_test.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	examples := map[string]bool{}
	collectVerifiedExamples(file, examples)

	if !examples["ExampleVerified"] {
		t.Fatal("ExampleVerified (with // Output:) not counted as verified coverage")
	}
	if !examples["ExampleEmptyOutput"] {
		t.Fatal("ExampleEmptyOutput (explicit empty // Output:) not counted as verified coverage")
	}
	if examples["ExampleUnverified"] {
		t.Fatal("ExampleUnverified (no // Output:) counted as coverage; compile-only examples are not verified")
	}
}

func TestHasExampleResolvesSuffixedNames(t *testing.T) {
	examples := map[string]bool{"ExampleParseConfig_oversize": true}

	if !hasExample(parseConfigSymbol, examples) {
		t.Fatal("hasExample(ParseConfig) = false, want suffixed example to count")
	}
	if hasExample("Parse", examples) {
		t.Fatal("hasExample(Parse) = true, want prefix matching to not cross symbol boundaries")
	}
}
