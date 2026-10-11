package contract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

func TestKnownErrorCodesProperties(t *testing.T) {
	codes := KnownErrorCodes()

	if !slices.IsSorted(codes) {
		t.Errorf("KnownErrorCodes() = %q, want byte-wise sorted", codes)
	}
	if compacted := slices.Compact(slices.Clone(codes)); len(compacted) != len(codes) {
		t.Errorf("KnownErrorCodes() = %q, want no duplicates", codes)
	}
	if slices.Contains(codes, "invalid_schema_declaration") {
		t.Error("KnownErrorCodes() lists invalid_schema_declaration, an authoring-time code")
	}

	want := slices.Clone(codes)
	codes[0] = "mutated"
	if got := KnownErrorCodes(); !slices.Equal(got, want) {
		t.Errorf("KnownErrorCodes() after caller mutation = %q, want %q", got, want)
	}
}

// TestErrorCodeConstantSpellings pins each code constant to its frozen
// spelling. The envelope tests pin each failure path to the same literal, so an
// adopter matching on the constant matches the envelope.
func TestErrorCodeConstantSpellings(t *testing.T) {
	tests := []struct {
		constant string
		want     string
	}{
		{ErrorCodeConfigInvalid, "config_invalid"},
		{ErrorCodeConfigMaxBytesInvalid, "config_max_bytes_invalid"},
		{ErrorCodeConfigOptionInvalid, "config_option_invalid"},
		{ErrorCodeConfigPatchInvalid, "config_patch_invalid"},
		{ErrorCodeConfigTooLarge, "config_too_large"},
		{ErrorCodeConfirmationRequired, "confirmation_required"},
		{ErrorCodeInternalError, "internal_error"},
		{ErrorCodeValidationError, "validation_error"},
		{ErrorCodeWarningsAsErrors, "warnings_as_errors"},
		{ErrorCodeInvalidSchemaDeclaration, "invalid_schema_declaration"},
	}
	for _, tc := range tests {
		if tc.constant != tc.want {
			t.Errorf("constant for %q = %q", tc.want, tc.constant)
		}
	}
}

// TestErrorCodeRegistryConsistency keeps the code constants and the two
// registry lists in step: every ErrorCode* constant is listed exactly once,
// every list entry is a constant, and the lists name constants rather than
// literals. Together with TestErrorCodesAreRegistered, a new code that skips
// either half of the registry fails the suite.
func TestErrorCodeRegistryConsistency(t *testing.T) {
	constants, err := testutil.ParseStringConstants(".")
	if err != nil {
		t.Fatalf("ParseStringConstants() error = %v", err)
	}
	known, authoring := KnownErrorCodes(), authoringTimeErrorCodes()

	if !slices.IsSorted(authoring) || len(slices.Compact(slices.Clone(authoring))) != len(authoring) {
		t.Errorf("authoringTimeErrorCodes() = %q, want sorted with no duplicates", authoring)
	}
	for _, code := range authoring {
		if slices.Contains(known, code) {
			t.Errorf("%q is in both KnownErrorCodes() and authoringTimeErrorCodes()", code)
		}
	}

	byValue := make(map[string][]string)
	for name, value := range constants {
		if !strings.HasPrefix(name, "ErrorCode") {
			continue
		}
		byValue[value] = append(byValue[value], name)
		if !slices.Contains(known, value) && !slices.Contains(authoring, value) {
			t.Errorf("%s = %q is in neither KnownErrorCodes() nor authoringTimeErrorCodes()", name, value)
		}
	}
	for _, code := range slices.Concat(known, authoring) {
		if names := byValue[code]; len(names) != 1 {
			t.Errorf("registered code %q is the value of %d ErrorCode* constants %v, want exactly 1",
				code, len(names), names)
		}
	}

	fset := token.NewFileSet()
	for _, fn := range []string{"KnownErrorCodes", "authoringTimeErrorCodes"} {
		for _, elt := range registryListElements(t, fset, fn) {
			if _, ok := elt.(*ast.Ident); !ok {
				t.Errorf("%s: %s lists a %T, want a constant identifier", fset.Position(elt.Pos()), fn, elt)
			}
		}
	}
}

// registryListElements returns the elements of the slice literal that the
// named function in codes.go returns.
func registryListElements(t *testing.T, fset *token.FileSet, fn string) []ast.Expr {
	t.Helper()
	file, err := parser.ParseFile(fset, "codes.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse codes.go: %v", err)
	}
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != fn || funcDecl.Body == nil || len(funcDecl.Body.List) != 1 {
			continue
		}
		ret, ok := funcDecl.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		if lit, ok := ret.Results[0].(*ast.CompositeLit); ok && len(lit.Elts) > 0 {
			return lit.Elts
		}
	}
	t.Fatalf("codes.go: %s must be a single return of a non-empty slice literal", fn)
	return nil
}
