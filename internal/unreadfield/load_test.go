package unreadfield

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func unitKeys(units []unit) []string {
	keys := make([]string, len(units))
	for i, u := range units {
		keys[i] = u.key
	}
	return keys
}

func unitFiles(u unit) []string {
	var names []string
	for _, f := range u.files {
		names = append(names, filepath.Base(u.fset.Position(f.Pos()).Filename))
	}
	slices.Sort(names)
	return names
}

func TestLoadUnitsSelectsTestVariants(t *testing.T) {
	units, err := loadUnits(t.Context(), "testdata/testonlyread", nil, goList)
	if err != nil {
		t.Fatalf("loadUnits: %v", err)
	}
	if got, want := unitKeys(units), []string{"testonlyread", "testonlyread_test"}; !slices.Equal(got, want) {
		t.Fatalf("unit keys = %q, want %q (plain variant and p.test main must be skipped)", got, want)
	}
	if got, want := unitFiles(units[0]), []string{"a.go", "a_test.go", "export_test.go"}; !slices.Equal(got, want) {
		t.Errorf("internal test variant files = %q, want %q", got, want)
	}
	if got, want := unitFiles(units[1]), []string{"a_x_test.go"}; !slices.Equal(got, want) {
		t.Errorf("external test files = %q, want %q", got, want)
	}
	if units[1].pkg.Path() != "testonlyread_test" {
		t.Errorf("external test package path = %q", units[1].pkg.Path())
	}
}

func TestLoadUnitsAppliesTags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tags  []string
		files []string
	}{
		{name: "default", files: []string{"a.go"}},
		{name: "ax_no_grpc", tags: []string{"ax_no_grpc"}, files: []string{"a.go", "read_grpc.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			units, err := loadUnits(t.Context(), "testdata/taggedread", tc.tags, goList)
			if err != nil {
				t.Fatalf("loadUnits: %v", err)
			}
			if len(units) != 1 {
				t.Fatalf("units = %q, want one", unitKeys(units))
			}
			if got := unitFiles(units[0]); !slices.Equal(got, tc.files) {
				t.Errorf("files = %q, want %q", got, tc.files)
			}
		})
	}
}

func TestLoadUnitsFailsClosed(t *testing.T) {
	_, err := loadUnits(t.Context(), "testdata/broken", nil, goList)
	if err == nil {
		t.Fatal("loadUnits succeeded on a package that does not type-check")
	}
	if errors.Is(err, fs.ErrPermission) {
		t.Errorf("a type error was reported as a permission failure: %v", err)
	}
}

// unreadableModule returns a module whose only source file cannot be opened,
// or skips when this platform or user can still read it.
func unreadableModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "a.go")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module unreadable\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("package unreadable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(src); err == nil {
		t.Skip("a mode-0 file is still readable here (root, or no POSIX permissions)")
	}
	return dir
}

func TestLoadUnitsTypesPermissionFailure(t *testing.T) {
	_, err := loadUnits(t.Context(), unreadableModule(t), nil, goList)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("loadUnits error = %v, want one wrapping fs.ErrPermission", err)
	}
}

func TestDecodeListing(t *testing.T) {
	stream := []byte(`{"ImportPath":"a","Name":"a","Dir":"/m/a","GoFiles":["a.go"]}` + "\n" +
		`{"ImportPath":"fmt","Export":"/cache/fmt","DepOnly":true}`)
	for _, tc := range []struct {
		name    string
		data    []byte
		limit   int
		wantErr bool
		want    int
	}{
		{name: "stream", data: stream, limit: maxListBytes, want: 2},
		{name: "empty", data: nil, limit: maxListBytes, want: 0},
		{name: "over ceiling", data: stream, limit: 10, wantErr: true},
		{name: "malformed", data: []byte(`{"ImportPath":`), limit: maxListBytes, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeListing(tc.data, tc.limit)
			if (err != nil) != tc.wantErr {
				t.Fatalf("decodeListing error = %v, wantErr %v", err, tc.wantErr)
			}
			if len(got) != tc.want {
				t.Errorf("decoded %d packages, want %d", len(got), tc.want)
			}
		})
	}
}

func FuzzDecodeListing(f *testing.F) {
	if data, err := goList(f.Context(), "testdata/testonlyread", nil); err == nil {
		f.Add(data)
	}
	f.Add([]byte(`{"ImportPath":"a","ForTest":"a","ImportMap":{"a":"a [a.test]"}}`))
	f.Add([]byte(`{}{}`))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, data []byte) {
		pkgs, err := decodeListing(data, maxListBytes)
		if err != nil && pkgs != nil {
			t.Fatal("decodeListing returned packages alongside an error")
		}
		if err == nil && len(data) > maxListBytes {
			t.Fatal("decodeListing accepted input over the ceiling")
		}
	})
}

func TestBrokenPackage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pkg     listedPackage
		wantErr bool
	}{
		{name: "clean", pkg: listedPackage{ImportPath: "a"}},
		{name: "own error", pkg: listedPackage{ImportPath: "a", Error: &listError{Err: "bad"}}, wantErr: true},
		{name: "dependency error", pkg: listedPackage{ImportPath: "a", DepsErrors: []*listError{{Err: "bad"}}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := brokenPackage([]listedPackage{tc.pkg})
			if (err != nil) != tc.wantErr {
				t.Fatalf("brokenPackage error = %v, wantErr %v", err, tc.wantErr)
			}
			if errors.Is(err, fs.ErrPermission) {
				t.Errorf("a reported package error was typed as a permission failure: %v", err)
			}
		})
	}
}
