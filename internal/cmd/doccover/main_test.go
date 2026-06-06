package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// parseConfigSymbol is the required-symbol fixture used across the report and
// hasExample cases.
const parseConfigSymbol = "ParseConfig"

func TestReportExitCodes(t *testing.T) {
	cases := []struct {
		name         string
		required     []string
		exported     map[string]bool
		examples     map[string]bool
		baseline     map[string]bool
		wantExit     int
		wantContains string
	}{
		{
			name:         "covered symbol passes",
			required:     []string{parseConfigSymbol},
			exported:     map[string]bool{parseConfigSymbol: true},
			examples:     map[string]bool{"ExampleParseConfig": true},
			baseline:     map[string]bool{},
			wantExit:     0,
			wantContains: "OK: no example-coverage regressions",
		},
		{
			name:         "missing without baseline is a regression",
			required:     []string{parseConfigSymbol},
			exported:     map[string]bool{parseConfigSymbol: true},
			examples:     map[string]bool{},
			baseline:     map[string]bool{},
			wantExit:     1,
			wantContains: "NEW regression",
		},
		{
			name:         "missing with baseline entry is exempt",
			required:     []string{parseConfigSymbol},
			exported:     map[string]bool{parseConfigSymbol: true},
			examples:     map[string]bool{},
			baseline:     map[string]bool{parseConfigSymbol: true},
			wantExit:     0,
			wantContains: "baseline-exempt",
		},
		{
			name:         "stale baseline entry fails the ratchet",
			required:     []string{parseConfigSymbol},
			exported:     map[string]bool{parseConfigSymbol: true},
			examples:     map[string]bool{"ExampleParseConfig": true},
			baseline:     map[string]bool{parseConfigSymbol: true},
			wantExit:     1,
			wantContains: "prune",
		},
		{
			name:         "unknown required symbol fails",
			required:     []string{"RenamedAway"},
			exported:     map[string]bool{},
			examples:     map[string]bool{},
			baseline:     map[string]bool{"RenamedAway": true},
			wantExit:     1,
			wantContains: "renamed or removed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got := report(&out, tc.required, tc.exported, tc.examples, tc.baseline)
			if got != tc.wantExit {
				t.Fatalf("report exit = %d, want %d\noutput:\n%s", got, tc.wantExit, out.String())
			}
			if !strings.Contains(out.String(), tc.wantContains) {
				t.Fatalf("report output missing %q:\n%s", tc.wantContains, out.String())
			}
		})
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
