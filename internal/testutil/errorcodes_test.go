package testutil

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const errorCodeFixtureDir = "testdata/errorcodes"

func fixtureGuard() ErrorCodeGuard {
	return ErrorCodeGuard{
		ModuleDir:       errorCodeFixtureDir,
		ModulePath:      moduleRootImportPath,
		RootPackageName: "ax",
		Registered:      []string{"known_code"},
		ExcludedDirs: []ExcludedDir{
			{Dir: "examples", Reason: "adopter code"},
			{Dir: "internal/cmd", Reason: "gate tools"},
		},
		PassThroughs: []PassThrough{
			{File: "error.go", Func: "NewError", Param: "code", Reason: "forwards its caller's code"},
			{File: "contract/error.go", Func: "NewError", Param: "code", Reason: "stores its caller's code"},
			{File: "stale.go", Func: "Forward", Param: "code", Reason: "matches nothing"},
		},
	}
}

// TestFindErrorCodeViolationsFixture is the proof that the guard can fail:
// the fixture plants one or more of every violation kind, plus valid sites and
// excluded paths that each hold a literal code and must stay silent.
func TestFindErrorCodeViolationsFixture(t *testing.T) {
	got, err := FindErrorCodeViolations(fixtureGuard())
	if err != nil {
		t.Fatalf("FindErrorCodeViolations() error = %v", err)
	}

	want := []ErrorCodeViolation{
		{File: "bad/bad.go", Line: 14, Kind: KindAssignCode, Expr: `"assign_literal"`},
		{File: "bad/bad.go", Line: 15, Kind: KindConstructorValue, Expr: "contract.NewError"},
		{File: "bad/bad.go", Line: 18, Kind: KindCallCode, Expr: `"string_literal"`},
		{File: "bad/bad.go", Line: 19, Kind: KindCallCode, Expr: "localCode"},
		{File: "bad/bad.go", Line: 20, Kind: KindCallCode, Expr: "contract.ErrorCodeOrphan"},
		{File: "bad/bad.go", Line: 21, Kind: KindCallCode, Expr: "contract.ModeLike"},
		{File: "bad/bad.go", Line: 22, Kind: KindCallCode, Expr: "code"},
		{File: "bad/bad.go", Line: 23, Kind: KindCallCode, Expr: `"con" + "cat"`},
		{File: "bad/bad.go", Line: 24, Kind: KindCallCode, Expr: `"root_literal"`},
		{File: "bad/bad.go", Line: 25, Kind: KindFieldCode, Expr: `"field_literal"`},
		{File: "bad/bad.go", Line: 33, Kind: KindAssignCode, Expr: "pair()"},
		{File: "contract/inside.go", Line: 8, Kind: KindCallCode, Expr: `"inside_literal"`},
		{File: "contract/inside.go", Line: 9, Kind: KindCallCode, Expr: "unexportedCode"},
		{File: "dot/dot.go", Line: 3, Kind: KindDotImport, Expr: `"github.com/rshade/ax-go/contract"`},
		{File: "error.go", Line: 14, Kind: KindCallCode, Expr: `"root_unqualified"`},
		{File: "stale.go", Line: 0, Kind: KindStalePassThrough, Expr: "Forward(code)"},
		{File: "tagged/tagged_windows.go", Line: 12, Kind: KindCallCode, Expr: `"tagged_literal"`},
	}
	if !slices.Equal(got, want) {
		t.Errorf("FindErrorCodeViolations() =\n%s\nwant\n%s",
			FormatErrorCodeViolations(got), FormatErrorCodeViolations(want))
	}

	if formatted := FormatErrorCodeViolations(got[:1]); formatted != `bad/bad.go:14: assign_code: "assign_literal"` {
		t.Errorf("FormatErrorCodeViolations() = %q", formatted)
	}
	if formatted := FormatErrorCodeViolations(nil); formatted != "" {
		t.Errorf("FormatErrorCodeViolations(nil) = %q, want empty", formatted)
	}
}

func TestFindErrorCodeViolationsRejectsInvalidGuard(t *testing.T) {
	unparsable := t.TempDir()
	writeFixtureFile(t, unparsable, "contract/codes.go", "package contract\n\nconst ErrorCodeKnown = \"known_code\"\n")
	writeFixtureFile(t, unparsable, "broken/broken.go", "package broken\n\nfunc {\n")

	brokenContract := t.TempDir()
	writeFixtureFile(t, brokenContract, "contract/codes.go", "package contract\n\nconst (\n")

	tests := []struct {
		name   string
		mutate func(*ErrorCodeGuard)
	}{
		{name: "empty module dir", mutate: func(g *ErrorCodeGuard) { g.ModuleDir = "" }},
		{name: "empty module path", mutate: func(g *ErrorCodeGuard) { g.ModulePath = "" }},
		{name: "empty root package name", mutate: func(g *ErrorCodeGuard) { g.RootPackageName = "" }},
		{name: "empty registered set", mutate: func(g *ErrorCodeGuard) { g.Registered = nil }},
		{name: "excluded dir without reason", mutate: func(g *ErrorCodeGuard) { g.ExcludedDirs[0].Reason = "" }},
		{name: "excluded dir without path", mutate: func(g *ErrorCodeGuard) { g.ExcludedDirs[0].Dir = "" }},
		{name: "pass-through without reason", mutate: func(g *ErrorCodeGuard) { g.PassThroughs[0].Reason = "" }},
		{name: "pass-through without param", mutate: func(g *ErrorCodeGuard) { g.PassThroughs[0].Param = "" }},
		{name: "module without contract dir", mutate: func(g *ErrorCodeGuard) { g.ModuleDir = t.TempDir() }},
		{name: "contract dir does not parse", mutate: func(g *ErrorCodeGuard) { g.ModuleDir = brokenContract }},
		{name: "source file does not parse", mutate: func(g *ErrorCodeGuard) { g.ModuleDir = unparsable }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			guard := fixtureGuard()
			tc.mutate(&guard)
			violations, err := FindErrorCodeViolations(guard)
			if err == nil {
				t.Fatalf("FindErrorCodeViolations() error = nil, violations = %v; want an error", violations)
			}
			if violations != nil {
				t.Errorf("FindErrorCodeViolations() violations = %v alongside error, want nil", violations)
			}
		})
	}
}

func TestParseStringConstants(t *testing.T) {
	got, err := ParseStringConstants(filepath.Join(errorCodeFixtureDir, "contract"))
	if err != nil {
		t.Fatalf("ParseStringConstants() error = %v", err)
	}
	want := map[string]string{
		"ErrorCodeKnown":  "known_code",
		"ErrorCodeOrphan": "orphan_code",
		"ModeLike":        "json",
	}
	if !maps.Equal(got, want) {
		t.Errorf("ParseStringConstants() = %v, want %v", got, want)
	}

	if _, err := ParseStringConstants(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no Go files") {
		t.Errorf("ParseStringConstants(empty dir) error = %v, want a no Go files error", err)
	}
}

func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
