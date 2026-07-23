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
	gomod := "module " + fixturePkg(name) + "\n\ngo 1.26\n"
	if err := os.WriteFile(filepath.Join(dst, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return dst
}

// fixturePkg returns the import path of the fixture module named name.
func fixturePkg(name string) string { return "fixture/" + name }

// hostProfile is the single profile fixture scans use; fixtures exercise the
// symbol walk, not the platform matrix.
func hostProfile() []Profile { return []Profile{{GOOS: goosLinux, GOARCH: goarchAMD64}} }

// defaultOnly is the single untagged configuration fixture scans use.
func defaultOnly() []Configuration { return []Configuration{{Name: configDefault}} }

// scanFixture scans dir with one configuration and one profile and returns the
// canonical inventory, failing on any signature divergence.
func scanFixture(t *testing.T, name string) []Observation {
	t.Helper()
	dir := fixtureDir(t, name)
	pkg := fixturePkg(name)
	scans, err := scanAll(context.Background(), dir, defaultOnly(), hostProfile(), []string{pkg})
	if err != nil {
		t.Fatalf("scanAll: %v", err)
	}
	inv, drift := reconcile(scans, pkg)
	if len(drift) != 0 {
		t.Fatalf("unexpected drift: %v", drift)
	}
	return inv
}

func featureIDs(inv []Observation) []string {
	ids := make([]string, len(inv))
	for i, o := range inv {
		ids[i] = o.Feature.ID
	}
	return ids
}

func assertInventory(t *testing.T, inv []Observation, want []Feature) {
	t.Helper()
	if !sort.StringsAreSorted(featureIDs(inv)) {
		t.Fatalf("inventory not sorted bytewise by id: %v", featureIDs(inv))
	}
	if len(inv) != len(want) {
		t.Fatalf("inventory length = %d, want %d\ngot:  %v\nwant: %v",
			len(inv), len(want), featureIDs(inv), want)
	}
	for i := range want {
		got := inv[i].Feature
		got.sourceID = ""
		got.Package = ""
		if got != want[i] {
			t.Errorf("feature %d:\n got %+v\nwant %+v", i, got, want[i])
		}
	}
}

func assertSourceIDs(t *testing.T, inv []Observation, want map[string]string) {
	t.Helper()
	for _, o := range inv {
		if sourceID, ok := want[o.Feature.ID]; ok && o.Feature.sourceID != sourceID {
			t.Errorf("source ID for %s = %q, want %q", o.Feature.ID, o.Feature.sourceID, sourceID)
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
	inv := scanFixture(t, "basic")
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
	inv := scanFixture(t, "promoted")
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
	inv := scanFixture(t, "hidden")
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

func TestScanExcludesTestFiles(t *testing.T) {
	inv := scanFixture(t, "testfiles")
	assertInventory(t, inv, []Feature{
		{ID: "func:Real", Kind: "func", Owner: "", Name: "Real", Signature: "func()", Access: "direct"},
	})
}

// scanDivergent scans the platform-divergent fixture across two profiles.
func scanDivergent(t *testing.T) ([]Observation, []DriftItem) {
	t.Helper()
	dir := fixtureDir(t, "divergent")
	pkg := fixturePkg("divergent")
	profiles := []Profile{
		{GOOS: goosLinux, GOARCH: goarchAMD64},
		{GOOS: goosWindows, GOARCH: goarchAMD64},
	}
	scans, err := scanAll(context.Background(), dir, defaultOnly(), profiles, []string{pkg})
	if err != nil {
		t.Fatalf("scanAll: %v", err)
	}
	inv, drift := reconcile(scans, pkg)
	return inv, drift
}

// TestReconcileRecordsProfilePresenceInsteadOfFailing pins the behaviour that
// changed when the configuration axis was introduced: a declaration that only
// exists on some targets is a recorded fact, not an automatic failure. The
// gate still catches it, but as presence drift against a reviewed baseline —
// see TestPresenceDriftDetectsUnreviewedProfileChange.
func TestReconcileRecordsProfilePresenceInsteadOfFailing(t *testing.T) {
	inv, drift := scanDivergent(t)
	if len(drift) != 0 {
		t.Fatalf("platform-specific presence must not be divergence drift, got %v", drift)
	}
	var winOnly *Observation
	for i := range inv {
		if inv[i].Feature.ID == "func:WinOnly" {
			winOnly = &inv[i]
		}
	}
	if winOnly == nil {
		t.Fatalf("func:WinOnly missing from inventory %v", featureIDs(inv))
	}
	windows := Combination{Configuration: configDefault, Profile: "windows/amd64"}
	linux := Combination{Configuration: configDefault, Profile: "linux/amd64"}
	if !winOnly.Present[windows] {
		t.Error("func:WinOnly must be recorded present on windows/amd64")
	}
	if winOnly.Present[linux] {
		t.Error("func:WinOnly must not be recorded present on linux/amd64")
	}
}

// TestPresenceDriftDetectsUnreviewedProfileChange proves the gate still fails
// closed on platform divergence: a baseline claiming a windows-only symbol is
// present everywhere is drift.
func TestPresenceDriftDetectsUnreviewedProfileChange(t *testing.T) {
	inv, drift := scanDivergent(t)
	if len(drift) != 0 {
		t.Fatalf("unexpected drift: %v", drift)
	}
	pkg := fixturePkg("divergent")
	universe := DefaultCombinations(defaultOnly(), []Profile{
		{GOOS: goosLinux, GOARCH: goarchAMD64},
		{GOOS: goosWindows, GOARCH: goarchAMD64},
	})

	features := make([]baselineFeature, 0, len(inv))
	for _, o := range inv {
		features = append(features, baselineFeature{
			ID:             o.Feature.ID,
			Signature:      o.Feature.Signature,
			Configurations: presenceSet{All: true},
			Profiles:       presenceSet{All: true},
		})
	}
	base := &baselineDoc{
		SchemaVersion: baselineSchemaVersion,
		Packages:      []packageBaseline{{Path: pkg, Features: features}},
	}

	got := diffBaseline(inv, base, universe, pkg)
	if len(got) != 1 {
		t.Fatalf("drift = %v, want exactly one presence-changed item", got)
	}
	want := DriftItem{
		ID:       "func:WinOnly",
		Drift:    driftPresenceChanged,
		Expected: "present in configuration default on linux/amd64",
		Actual:   "absent in configuration default on linux/amd64",
	}
	if got[0] != want {
		t.Errorf("drift:\n got %+v\nwant %+v", got[0], want)
	}
}

// TestReconcileFailsClosedOnSignatureDivergence pins the invariant that
// survived the merge: one feature has exactly one signature, so a rendering
// that differs between combinations fails closed with a nil inventory rather
// than silently recording one of the two.
func TestReconcileFailsClosedOnSignatureDivergence(t *testing.T) {
	t.Parallel()
	linux := Combination{Configuration: configDefault, Profile: "linux/amd64"}
	windows := Combination{Configuration: configDefault, Profile: "windows/amd64"}
	feature := func(sig string) Feature {
		return Feature{Package: "fixture/x", ID: "func:F", Kind: kindFunc, Name: "F", Signature: sig}
	}
	scans := []combinationScan{
		{combination: linux, features: []Feature{feature("func() int")}},
		{combination: windows, features: []Feature{feature("func() string")}},
	}
	inv, drift := reconcile(scans, "fixture/x")
	if inv != nil {
		t.Fatalf("signature divergence must fail closed with nil inventory, got %v", featureIDs(inv))
	}
	if len(drift) != 1 || drift[0].Drift != driftSignatureDivergent || drift[0].ID != "func:F" {
		t.Fatalf("drift = %+v, want one signature-divergent item for func:F", drift)
	}
}
