package analyzer_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/rshade/ax-go/internal/unreadfield"
	"github.com/rshade/ax-go/internal/unreadfield/analyzer"
)

// fixtures is the module shared with the core's tests. It has no _test.go
// files: analysistest loads with Tests: true and would otherwise analyze the
// plain variant of a package whose only read is in a test.
const fixtures = "../testdata/fixtures"

func TestAnalyzerMatchesWants(t *testing.T) {
	analysistest.Run(t, fixtures, analyzer.New(), "./...")
}

// TestFrontEndsAgree asserts FR-012 / SC-003: the analyzer and the gate's
// core report the same fields at the same lines with the same message.
// Positions are compared physically, as the gate reports them, so the
// linedirective fixture checks that the adapter survives //line remapping.
func TestFrontEndsAgree(t *testing.T) {
	root, err := filepath.Abs(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	var fromAnalyzer []string
	for _, r := range analysistest.Run(t, fixtures, analyzer.New(), "./...") {
		for _, d := range r.Action.Diagnostics {
			p := r.Action.Package.Fset.PositionFor(d.Pos, false)
			rel, relErr := filepath.Rel(root, p.Filename)
			if relErr != nil {
				t.Fatal(relErr)
			}
			fromAnalyzer = append(fromAnalyzer, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), p.Line, d.Message))
		}
	}
	report, err := unreadfield.Run(t.Context(), fixtures, []unreadfield.Configuration{{Name: "default"}}, time.Minute)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var fromCore []string
	for _, f := range report.Findings {
		fromCore = append(fromCore, fmt.Sprintf("%s:%d: %s", f.Declared.File, f.Declared.Line, f.Message()))
	}
	slices.Sort(fromAnalyzer)
	slices.Sort(fromCore)
	if len(fromCore) == 0 {
		t.Fatal("core reported nothing; the fixture module must contain findings")
	}
	if !slices.Equal(fromAnalyzer, fromCore) {
		t.Errorf("front ends disagree:\nanalyzer %q\ncore     %q", fromAnalyzer, fromCore)
	}
}

func TestNewReturnsFreshAnalyzer(t *testing.T) {
	a, b := analyzer.New(), analyzer.New()
	if a == b {
		t.Error("New returned the same *analysis.Analyzer twice")
	}
	if a.Name != "unreadfield" || a.Doc == "" {
		t.Errorf("Name %q, Doc %q", a.Name, a.Doc)
	}
}
