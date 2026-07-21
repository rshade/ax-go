package main

import (
	"context"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// fixtureDir copies a synthetic fixture package out of testdata into a temp
// module so the go tool will list it (the go tool skips testdata trees).
func fixtureDir(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", "fixtures", name)
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatalf("read fixture file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), data, 0o644); err != nil {
			t.Fatalf("write fixture file: %v", err)
		}
	}
	gomod := "module fixture/" + name + "\n\ngo 1.26\n"
	if err := os.WriteFile(filepath.Join(dst, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return dst
}

// scanFixture scans dir with a single host-equivalent profile and returns the
// canonical inventory, failing on any profile drift.
func scanFixture(t *testing.T, dir string) []Feature {
	t.Helper()
	profiles := []Profile{{GOOS: "linux", GOARCH: "amd64"}}
	scans, err := scanAll(context.Background(), dir, profiles)
	if err != nil {
		t.Fatalf("scanAll: %v", err)
	}
	inv, drift := reconcile(scans, profiles)
	if len(drift) != 0 {
		t.Fatalf("unexpected drift: %v", drift)
	}
	return inv
}

func featureIDs(inv []Feature) []string {
	ids := make([]string, len(inv))
	for i, f := range inv {
		ids[i] = f.ID
	}
	return ids
}

func assertInventory(t *testing.T, inv []Feature, want []Feature) {
	t.Helper()
	if !sort.StringsAreSorted(featureIDs(inv)) {
		t.Fatalf("inventory not sorted bytewise by id: %v", featureIDs(inv))
	}
	if len(inv) != len(want) {
		t.Fatalf("inventory length = %d, want %d\ngot:  %v\nwant: %v", len(inv), len(want), inv, want)
	}
	for i := range want {
		got := inv[i]
		got.sourceID = ""
		if got != want[i] {
			t.Errorf("feature %d:\n got %+v\nwant %+v", i, got, want[i])
		}
	}
}

func assertSourceIDs(t *testing.T, inv []Feature, want map[string]string) {
	t.Helper()
	for _, feature := range inv {
		if sourceID, ok := want[feature.ID]; ok && feature.sourceID != sourceID {
			t.Errorf("source ID for %s = %q, want %q", feature.ID, feature.sourceID, sourceID)
		}
	}
}

func TestQualifierDisambiguatesPackageNamesByImportPath(t *testing.T) {
	t.Parallel()
	root := types.NewPackage("example.com/root", "root")
	tests := []struct {
		name       string
		importPath string
		want       string
	}{
		{name: "first foo package", importPath: "example.com/a/foo", want: "func(example.com/a/foo.T)"},
		{name: "second foo package", importPath: "example.com/b/foo", want: "func(example.com/b/foo.T)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pkg := types.NewPackage(tc.importPath, "foo")
			obj := types.NewTypeName(token.NoPos, pkg, "T", nil)
			named := types.NewNamed(obj, types.NewStruct(nil, nil), nil)
			params := types.NewTuple(types.NewVar(token.NoPos, root, "", named))
			sig := types.NewSignatureType(nil, nil, nil, params, nil, false)
			if got := canonicalSignature(sig, qualifier(root)); got != tc.want {
				t.Fatalf("canonical signature = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScanBasic(t *testing.T) {
	inv := scanFixture(t, fixtureDir(t, "basic"))
	assertInventory(t, inv, []Feature{
		{ID: "const:Answer", Kind: "const", Owner: "", Name: "Answer", Signature: "untyped int", Access: "direct"},
		{ID: "const:Named", Kind: "const", Owner: "", Name: "Named", Signature: "string", Access: "direct"},
		{ID: "field:Alias.Count", Kind: "field", Owner: "Alias", Name: "Count", Signature: "int", Access: "alias"},
		{ID: "field:Alias.Name", Kind: "field", Owner: "Alias", Name: "Name", Signature: "string", Access: "alias"},
		{ID: "field:Config.Count", Kind: "field", Owner: "Config", Name: "Count", Signature: "int", Access: "direct"},
		{ID: "field:Config.Name", Kind: "field", Owner: "Config", Name: "Name", Signature: "string", Access: "direct"},
		{ID: "func:Do", Kind: "func", Owner: "", Name: "Do", Signature: "func(int) string", Access: "direct"},
		{ID: "func:Pick", Kind: "func", Owner: "", Name: "Pick", Signature: "func[T any](T) T", Access: "direct"},
		{ID: "interface-method:ReadWriter.Read", Kind: "interface-method", Owner: "ReadWriter",
			Name: "Read", Signature: "func([]byte) (int, error)", Access: "promoted"},
		{ID: "interface-method:ReadWriter.Write", Kind: "interface-method", Owner: "ReadWriter",
			Name: "Write", Signature: "func([]byte) (int, error)", Access: "promoted"},
		{ID: "interface-method:Stringer.String", Kind: "interface-method", Owner: "Stringer",
			Name: "String", Signature: "func() string", Access: "direct"},
		{ID: "method:*Alias.Set", Kind: "method", Owner: "Alias",
			Name: "Set", Signature: "func(string)", Access: "alias"},
		{ID: "method:*Config.Set", Kind: "method", Owner: "Config",
			Name: "Set", Signature: "func(string)", Access: "direct"},
		{
			ID:        "method:Alias.Get",
			Kind:      "method",
			Owner:     "Alias",
			Name:      "Get",
			Signature: "func() string",
			Access:    "alias",
		},
		{
			ID:        "method:Config.Get",
			Kind:      "method",
			Owner:     "Config",
			Name:      "Get",
			Signature: "func() string",
			Access:    "direct",
		},
		{ID: "type:Alias", Kind: "type", Owner: "", Name: "Alias", Signature: "= Config", Access: "alias"},
		{ID: "type:Config", Kind: "type", Owner: "", Name: "Config",
			Signature: "struct{Name string; Count int}", Access: "direct"},
		{ID: "type:ReadWriter", Kind: "type", Owner: "", Name: "ReadWriter",
			Signature: "interface{io.Reader; io.Writer}", Access: "direct"},
		{ID: "type:Stringer", Kind: "type", Owner: "", Name: "Stringer",
			Signature: "interface{String() string}", Access: "direct"},
		{ID: "var:Count", Kind: "var", Owner: "", Name: "Count", Signature: "int", Access: "direct"},
		{ID: "var:Default", Kind: "var", Owner: "", Name: "Default", Signature: "Config", Access: "direct"},
	})
	assertSourceIDs(t, inv, map[string]string{
		"field:Alias.Count": "field:Config.Count",
		"field:Alias.Name":  "field:Config.Name",
		"method:*Alias.Set": "method:*Config.Set",
		"method:Alias.Get":  "method:Config.Get",
	})
}

func TestScanPromoted(t *testing.T) {
	inv := scanFixture(t, fixtureDir(t, "promoted"))
	assertInventory(t, inv, []Feature{
		{ID: "field:Both.Left", Kind: "field", Owner: "Both", Name: "Left", Signature: "Left", Access: "direct"},
		{ID: "field:Both.Right", Kind: "field", Owner: "Both", Name: "Right", Signature: "Right", Access: "direct"},
		{ID: "field:Diamond.ViaLeft", Kind: "field", Owner: "Diamond",
			Name: "ViaLeft", Signature: "ViaLeft", Access: "direct"},
		{ID: "field:Diamond.ViaRight", Kind: "field", Owner: "Diamond",
			Name: "ViaRight", Signature: "ViaRight", Access: "direct"},
		{ID: "field:Inner.A", Kind: "field", Owner: "Inner", Name: "A", Signature: "int", Access: "direct"},
		{ID: "field:Left.Dup", Kind: "field", Owner: "Left", Name: "Dup", Signature: "int", Access: "direct"},
		{ID: "field:Outer.A", Kind: "field", Owner: "Outer", Name: "A", Signature: "int", Access: "promoted"},
		{ID: "field:Outer.Inner", Kind: "field", Owner: "Outer", Name: "Inner", Signature: "Inner", Access: "direct"},
		{ID: "field:Outer.X", Kind: "field", Owner: "Outer", Name: "X", Signature: "string", Access: "direct"},
		{ID: "field:Repeated.X", Kind: "field", Owner: "Repeated", Name: "X", Signature: "int", Access: "direct"},
		{ID: "field:Right.Dup", Kind: "field", Owner: "Right", Name: "Dup", Signature: "string", Access: "direct"},
		{ID: "field:ViaLeft.Repeated", Kind: "field", Owner: "ViaLeft",
			Name: "Repeated", Signature: "Repeated", Access: "direct"},
		{ID: "field:ViaLeft.X", Kind: "field", Owner: "ViaLeft", Name: "X", Signature: "int", Access: "promoted"},
		{ID: "field:ViaRight.Repeated", Kind: "field", Owner: "ViaRight",
			Name: "Repeated", Signature: "Repeated", Access: "direct"},
		{ID: "field:ViaRight.X", Kind: "field", Owner: "ViaRight", Name: "X", Signature: "int", Access: "promoted"},
		{ID: "field:Wrap.Pub", Kind: "field", Owner: "Wrap", Name: "Pub", Signature: "int", Access: "promoted"},
		{ID: "method:Inner.M", Kind: "method", Owner: "Inner", Name: "M", Signature: "func()", Access: "direct"},
		{ID: "method:Outer.M", Kind: "method", Owner: "Outer", Name: "M", Signature: "func()", Access: "promoted"},
		{ID: "type:Both", Kind: "type", Owner: "", Name: "Both", Signature: "struct{Left; Right}", Access: "direct"},
		{ID: "type:Diamond", Kind: "type", Owner: "", Name: "Diamond",
			Signature: "struct{ViaLeft; ViaRight}", Access: "direct"},
		{ID: "type:Inner", Kind: "type", Owner: "", Name: "Inner", Signature: "struct{A int; b int}", Access: "direct"},
		{ID: "type:Left", Kind: "type", Owner: "", Name: "Left", Signature: "struct{Dup int}", Access: "direct"},
		{
			ID:        "type:Outer",
			Kind:      "type",
			Owner:     "",
			Name:      "Outer",
			Signature: "struct{Inner; X string}",
			Access:    "direct",
		},
		{ID: "type:Repeated", Kind: "type", Owner: "", Name: "Repeated", Signature: "struct{X int}", Access: "direct"},
		{ID: "type:Right", Kind: "type", Owner: "", Name: "Right", Signature: "struct{Dup string}", Access: "direct"},
		{ID: "type:ViaLeft", Kind: "type", Owner: "", Name: "ViaLeft", Signature: "struct{Repeated}", Access: "direct"},
		{ID: "type:ViaRight", Kind: "type", Owner: "", Name: "ViaRight",
			Signature: "struct{Repeated}", Access: "direct"},
		{ID: "type:Wrap", Kind: "type", Owner: "", Name: "Wrap", Signature: "struct{hidden}", Access: "direct"},
	})
	assertSourceIDs(t, inv, map[string]string{
		"field:Outer.A":    "field:Inner.A",
		"field:ViaLeft.X":  "field:Repeated.X",
		"field:ViaRight.X": "field:Repeated.X",
		"field:Wrap.Pub":   "field:hidden.Pub",
		"method:Outer.M":   "method:Inner.M",
	})
}

func TestScanHidden(t *testing.T) {
	inv := scanFixture(t, fixtureDir(t, "hidden"))
	assertInventory(t, inv, []Feature{
		{ID: "field:result.V", Kind: "field", Owner: "result", Name: "V", Signature: "int", Access: "direct"},
		{ID: "func:New", Kind: "func", Owner: "", Name: "New", Signature: "func() *result", Access: "direct"},
		{ID: "func:NewIt", Kind: "func", Owner: "", Name: "NewIt", Signature: "func() It", Access: "direct"},
		{ID: "func:NewIt2", Kind: "func", Owner: "", Name: "NewIt2", Signature: "func() it2", Access: "direct"},
		{ID: "interface-method:It.Step", Kind: "interface-method", Owner: "It",
			Name: "Step", Signature: "func()", Access: "direct"},
		{ID: "interface-method:it2.X", Kind: "interface-method", Owner: "it2",
			Name: "X", Signature: "func()", Access: "direct"},
		{ID: "method:*result.Next", Kind: "method", Owner: "result",
			Name: "Next", Signature: "func() bool", Access: "direct"},
		{ID: "type:It", Kind: "type", Owner: "", Name: "It", Signature: "interface{Step()}", Access: "direct"},
	})
}

func TestScanProfileDivergence(t *testing.T) {
	dir := fixtureDir(t, "divergent")
	profiles := []Profile{
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "amd64"},
	}
	scans, err := scanAll(context.Background(), dir, profiles)
	if err != nil {
		t.Fatalf("scanAll: %v", err)
	}
	inv, drift := reconcile(scans, profiles)
	if inv != nil {
		t.Fatalf("divergent profiles must fail closed with nil inventory, got %v", featureIDs(inv))
	}
	want := []DriftItem{
		{ID: "func:WinOnly", Drift: "profile-divergent",
			Expected: "present in all profiles", Actual: "absent in linux/amd64"},
	}
	if len(drift) != len(want) {
		t.Fatalf("drift = %v, want %v", drift, want)
	}
	for i := range want {
		if drift[i] != want[i] {
			t.Errorf("drift %d: got %+v want %+v", i, drift[i], want[i])
		}
	}
}

func TestScanExcludesTestFiles(t *testing.T) {
	inv := scanFixture(t, fixtureDir(t, "testfiles"))
	assertInventory(t, inv, []Feature{
		{ID: "func:Real", Kind: "func", Owner: "", Name: "Real", Signature: "func()", Access: "direct"},
	})
}
