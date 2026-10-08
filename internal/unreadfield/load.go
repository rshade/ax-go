package unreadfield

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// maxListBytes bounds one go list stream. A whole-repository listing measured
// 2.4 MB; the ceiling matches deadcheck's report bound.
const maxListBytes = 16 << 20

// lister runs go list for one tag set in dir. It is a parameter so tests can
// substitute a command that outlives its context.
type lister func(ctx context.Context, dir string, tags []string) ([]byte, error)

// listedPackage is the subset of go list -json the loader needs.
type listedPackage struct {
	ImportPath string            `json:"ImportPath"`
	Name       string            `json:"Name"`
	Dir        string            `json:"Dir"`
	Export     string            `json:"Export"`
	ForTest    string            `json:"ForTest"`
	DepOnly    bool              `json:"DepOnly"`
	GoFiles    []string          `json:"GoFiles"`
	CgoFiles   []string          `json:"CgoFiles"`
	ImportMap  map[string]string `json:"ImportMap"`
}

// unit is one type-checked package variant ready for Analyze.
type unit struct {
	key   string
	fset  *token.FileSet
	files []*ast.File
	pkg   *types.Package
	info  *types.Info
}

// cappedBuffer stops copying a subprocess stream once it would exceed max.
type cappedBuffer struct {
	buffer bytes.Buffer
	max    int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.max-b.buffer.Len() {
		return 0, fmt.Errorf("go list output exceeds %d bytes", b.max)
	}
	return b.buffer.Write(p)
}

// goList runs go list -deps -export -test -json ./... in dir. Test variants
// carry production and internal test files together, and -export provides
// the compiler export data the gc importer reads.
func goList(ctx context.Context, dir string, tags []string) ([]byte, error) {
	args := []string{"list", "-deps", "-export", "-test", "-json"}
	if len(tags) > 0 {
		args = append(args, "-tags="+strings.Join(tags, ","))
	}
	args = append(args, "./...")
	// #nosec G204 -- arguments are fixed policy; no shell interprets them.
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	stdout := &cappedBuffer{max: maxListBytes}
	stderr := &cappedBuffer{max: maxListBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.buffer.String()))
	}
	return stdout.buffer.Bytes(), nil
}

// decodeListing decodes a go list -json stream of concatenated objects. Input
// over limit is rejected before decoding.
func decodeListing(data []byte, limit int) ([]listedPackage, error) {
	if len(data) > limit {
		return nil, fmt.Errorf("go list output exceeds %d bytes", limit)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var pkgs []listedPackage
	for {
		var lp listedPackage
		err := dec.Decode(&lp)
		if errors.Is(err, io.EOF) {
			return pkgs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decoding go list output: %w", err)
		}
		pkgs = append(pkgs, lp)
	}
}

// loadUnits lists dir under tags and type-checks every analyzable unit,
// sorted by key. It fails closed: a listing error, a parse or type error, or
// missing export data fails the whole load. When ctx ends while go list runs,
// the child's "signal: killed" is replaced by the context error so callers
// can classify a timeout.
func loadUnits(ctx context.Context, dir string, tags []string, list lister) ([]unit, error) {
	data, err := list(ctx, dir, tags)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("go list: %w", ctxErr)
		}
		return nil, fmt.Errorf("go list: %w", err)
	}
	pkgs, err := decodeListing(data, maxListBytes)
	if err != nil {
		return nil, err
	}
	exports := make(map[string]string, len(pkgs))
	for _, lp := range pkgs {
		if lp.Export != "" {
			exports[lp.ImportPath] = lp.Export
		}
	}
	var units []unit
	for _, lp := range selectUnits(pkgs) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("loading %s: %w", lp.ImportPath, ctxErr)
		}
		u, checkErr := checkUnit(lp, exports)
		if checkErr != nil {
			return nil, checkErr
		}
		units = append(units, u)
	}
	slices.SortFunc(units, func(a, b unit) int { return strings.Compare(a.key, b.key) })
	return units, nil
}

// selectUnits keeps one variant per package: the internal test variant
// "p [p.test]" when it exists (it already holds p's production files), else
// the plain package, plus the external test package "p_test [p.test]".
// Dependencies, the synthetic p.test main, and variants of one package
// recompiled for another package's test are skipped.
func selectUnits(pkgs []listedPackage) []listedPackage {
	hasTestVariant := make(map[string]bool)
	for _, lp := range pkgs {
		if lp.ForTest != "" && unitKey(lp.ImportPath) == lp.ForTest {
			hasTestVariant[lp.ForTest] = true
		}
	}
	var selected []listedPackage
	for _, lp := range pkgs {
		key := unitKey(lp.ImportPath)
		switch {
		case lp.DepOnly:
		case lp.ForTest == "" && strings.HasSuffix(lp.ImportPath, ".test"):
		case lp.ForTest == "" && hasTestVariant[key]:
		case lp.ForTest != "" && key != lp.ForTest && key != lp.ForTest+"_test":
		default:
			selected = append(selected, lp)
		}
	}
	return selected
}

// unitKey strips the " [p.test]" variant suffix go list appends.
func unitKey(importPath string) string {
	key, _, _ := strings.Cut(importPath, " [")
	return key
}

func checkUnit(lp listedPackage, exports map[string]string) (unit, error) {
	fset := token.NewFileSet()
	names := append(slices.Clone(lp.GoFiles), lp.CgoFiles...)
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(lp.Dir, name)
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return unit{}, fmt.Errorf("parsing %s: %w", lp.ImportPath, err)
		}
		files = append(files, file)
	}
	lookup := func(path string) (io.ReadCloser, error) {
		if mapped, ok := lp.ImportMap[path]; ok {
			path = mapped
		}
		export, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(export) // #nosec G304 -- path comes from the toolchain's own listing.
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	conf := types.Config{
		Importer:    importer.ForCompiler(fset, "gc", lookup),
		Sizes:       types.SizesFor("gc", runtime.GOARCH),
		FakeImportC: true,
	}
	key := unitKey(lp.ImportPath)
	pkg, err := conf.Check(key, fset, files, info)
	if err != nil {
		return unit{}, fmt.Errorf("type-checking %s: %w", lp.ImportPath, err)
	}
	return unit{key: key, fset: fset, files: files, pkg: pkg, info: info}, nil
}
