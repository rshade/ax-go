package testutil

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Violation kinds reported by FindErrorCodeViolations.
const (
	// KindCallCode: a NewError call's code argument is not a registered
	// constant.
	KindCallCode = "call_code"
	// KindFieldCode: an ErrorCode composite-literal element is not a
	// registered constant.
	KindFieldCode = "field_code"
	// KindAssignCode: the value assigned to an ErrorCode field is not a
	// registered constant.
	KindAssignCode = "assign_code"
	// KindConstructorValue: NewError is referenced other than as the function
	// of a call, which hides its code argument from the guard.
	KindConstructorValue = "constructor_value"
	// KindDotImport: the contract or root package is dot-imported, which lets
	// an unqualified call escape the guard.
	KindDotImport = "dot_import"
	// KindStalePassThrough: a PassThrough entry matched no site.
	// #nosec G101 -- a violation kind name, not a credential.
	KindStalePassThrough = "stale_pass_through"
)

const (
	contractDirName      = "contract"
	errorConstructorName = "NewError"
	errorCodeFieldName   = "ErrorCode"
	// codeArgIndex is the position of the code in NewError(ctx, code, message).
	codeArgIndex = 1
)

var errInvalidErrorCodeGuard = errors.New("invalid error code guard")

// ErrorCodeGuard configures FindErrorCodeViolations. The registry policy
// (what counts as registered, which directories are out of scope, and which
// constructors may forward a code parameter) belongs to the caller; the guard
// supplies only the mechanism.
type ErrorCodeGuard struct {
	// ModuleDir is the directory walked. Its contract subdirectory supplies
	// the exported string constants a code may name.
	ModuleDir string
	// ModulePath is the module import path; the root package is imported by
	// this path and the contract package by ModulePath + "/contract".
	ModulePath string
	// RootPackageName is the local name of an unaliased root-package import.
	RootPackageName string
	// Registered holds the code values that count as registered. A contract
	// constant is registered when its value is in this set.
	Registered []string
	// ExcludedDirs are module-relative directories skipped with their subtrees.
	ExcludedDirs []ExcludedDir
	// PassThroughs are the only sites where a bare parameter is accepted as a
	// code.
	PassThroughs []PassThrough
}

// ExcludedDir names a module-relative, slash-separated directory whose subtree
// the guard does not check, and why.
type ExcludedDir struct {
	Dir    string
	Reason string
}

// PassThrough accepts the parameter Param of the function Func in the
// module-relative file File as a code expression, because that function is a
// constructor that forwards or stores its caller's code. Reason is required.
type PassThrough struct {
	File   string
	Func   string
	Param  string
	Reason string
}

// ErrorCodeViolation is one site whose code is not a registered constant, or a
// construct that hides such a site. File is module-relative and
// slash-separated; Line is 1-based, or 0 for a stale pass-through entry; Kind is
// one of the Kind constants; Expr is the offending expression as Go source.
type ErrorCodeViolation struct {
	File string
	Line int
	Kind string
	Expr string
}

// FindErrorCodeViolations parses every non-test Go file under guard.ModuleDir
// and returns each site that sets an ax.Error code from anything other than a
// registered contract constant, sorted by file, line, kind, and expression.
//
// An emit site is a call to contract.NewError or the root NewError (under any
// import alias, or unqualified inside the root or contract directory), an
// ErrorCode composite-literal element, or an assignment to an ErrorCode field.
// Its code is registered only when it is written as contract.Name, or as a bare
// Name inside the contract directory, and Name is an exported contract
// constant whose value is in guard.Registered. A literal, a local constant, a
// variable, or a computed string is a violation, so the check never has to
// evaluate an expression. Non-call references to NewError, dot imports of the
// contract or root package, and PassThrough entries that match nothing are
// violations too.
//
// Build constraints are ignored: every file is parsed, so code behind a build
// tag or a GOOS/GOARCH suffix is checked under any test configuration. The walk
// skips _test.go files, testdata, hidden, and node_modules directories, any
// directory below the root holding its own go.mod, and guard.ExcludedDirs.
//
// The check is type-free. It matches import names, the NewError name, and the
// ErrorCode field name syntactically: a local identifier shadowing an import
// name is not detected, and a different type with an ErrorCode field is checked
// as if it were contract.Error.
//
// A non-nil error means the guard could not run (invalid configuration, an
// unreadable directory, or a file that does not parse) and is never a clean
// result; the returned violations are then nil.
func FindErrorCodeViolations(guard ErrorCodeGuard) ([]ErrorCodeViolation, error) {
	if err := guard.validate(); err != nil {
		return nil, err
	}
	constants, err := ParseStringConstants(filepath.Join(guard.ModuleDir, contractDirName))
	if err != nil {
		return nil, fmt.Errorf("read contract constants: %w", err)
	}
	files, err := guard.sourceFiles()
	if err != nil {
		return nil, err
	}

	c := &errorCodeChecker{
		guard:      guard,
		fset:       token.NewFileSet(),
		registered: registeredConstantNames(constants, guard.Registered),
		matched:    make([]bool, len(guard.PassThroughs)),
	}
	for _, rel := range files {
		file, parseErr := parser.ParseFile(c.fset, filepath.Join(guard.ModuleDir, filepath.FromSlash(rel)), nil,
			parser.SkipObjectResolution)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, parseErr)
		}
		c.checkFile(rel, file)
	}
	for i, entry := range guard.PassThroughs {
		if !c.matched[i] {
			c.violations = append(c.violations, ErrorCodeViolation{
				File: entry.File,
				Kind: KindStalePassThrough,
				Expr: entry.Func + "(" + entry.Param + ")",
			})
		}
	}

	slices.SortFunc(c.violations, func(a, b ErrorCodeViolation) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.Expr, b.Expr),
		)
	})
	return c.violations, nil
}

// FormatErrorCodeViolations renders one "file:line: kind: expr" line per
// violation, for a test failure message. It returns "" for no violations.
func FormatErrorCodeViolations(violations []ErrorCodeViolation) string {
	var b strings.Builder
	for _, v := range violations {
		fmt.Fprintf(&b, "%s:%d: %s: %s\n", v.File, v.Line, v.Kind, v.Expr)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ParseStringConstants returns every exported constant declared in the non-test
// Go files of dir whose value is a string literal, keyed by name. It returns an
// error when dir cannot be read, holds no non-test Go file, or a file does not
// parse.
func ParseStringConstants(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	constants := make(map[string]string)
	parsed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isNonTestGoFile(name) {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", name, parseErr)
		}
		parsed++
		collectStringConstants(file, constants)
	}
	if parsed == 0 {
		return nil, fmt.Errorf("%w: no Go files in %s", fs.ErrNotExist, dir)
	}
	return constants, nil
}

func collectStringConstants(file *ast.File, constants map[string]string) {
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			if valueSpec, isValue := spec.(*ast.ValueSpec); isValue {
				collectSpecStringConstants(valueSpec, constants)
			}
		}
	}
}

func collectSpecStringConstants(spec *ast.ValueSpec, constants map[string]string) {
	for i, name := range spec.Names {
		if !name.IsExported() || i >= len(spec.Values) {
			continue
		}
		lit, isLit := spec.Values[i].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			continue
		}
		if value, err := strconv.Unquote(lit.Value); err == nil {
			constants[name.Name] = value
		}
	}
}

func registeredConstantNames(constants map[string]string, registered []string) map[string]bool {
	names := make(map[string]bool)
	for name, value := range constants {
		if slices.Contains(registered, value) {
			names[name] = true
		}
	}
	return names
}

func isNonTestGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func (g ErrorCodeGuard) validate() error {
	switch {
	case g.ModuleDir == "":
		return fmt.Errorf("%w: ModuleDir is empty", errInvalidErrorCodeGuard)
	case g.ModulePath == "":
		return fmt.Errorf("%w: ModulePath is empty", errInvalidErrorCodeGuard)
	case g.RootPackageName == "":
		return fmt.Errorf("%w: RootPackageName is empty", errInvalidErrorCodeGuard)
	case len(g.Registered) == 0:
		return fmt.Errorf("%w: Registered is empty", errInvalidErrorCodeGuard)
	}
	for _, excluded := range g.ExcludedDirs {
		if excluded.Dir == "" || excluded.Reason == "" {
			return fmt.Errorf("%w: excluded dir %q needs a path and a reason", errInvalidErrorCodeGuard, excluded.Dir)
		}
	}
	for _, entry := range g.PassThroughs {
		if entry.File == "" || entry.Func == "" || entry.Param == "" || entry.Reason == "" {
			return fmt.Errorf("%w: pass-through %s %s(%s) needs a file, function, parameter, and reason",
				errInvalidErrorCodeGuard, entry.File, entry.Func, entry.Param)
		}
	}
	return nil
}

// sourceFiles returns the module-relative, slash-separated paths of every Go
// file the guard checks, in lexical order.
func (g ErrorCodeGuard) sourceFiles() ([]string, error) {
	excluded := make(map[string]bool, len(g.ExcludedDirs))
	for _, dir := range g.ExcludedDirs {
		excluded[path.Clean(dir.Dir)] = true
	}

	var files []string
	err := filepath.WalkDir(g.ModuleDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(g.ModuleDir, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules" || excluded[rel] {
				return filepath.SkipDir
			}
			if _, statErr := os.Stat(filepath.Join(p, "go.mod")); statErr == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if isNonTestGoFile(d.Name()) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", g.ModuleDir, err)
	}
	return files, nil
}

type errorCodeChecker struct {
	guard      ErrorCodeGuard
	fset       *token.FileSet
	registered map[string]bool
	matched    []bool
	violations []ErrorCodeViolation
}

// fileScope is what the checker knows about the file and function being
// walked.
type fileScope struct {
	rel           string
	inRoot        bool
	inContract    bool
	contractNames map[string]bool
	rootNames     map[string]bool
	funcName      string
	params        map[string]bool
}

func (c *errorCodeChecker) checkFile(rel string, file *ast.File) {
	dir := path.Dir(rel)
	scope := fileScope{
		rel:           rel,
		inRoot:        dir == ".",
		inContract:    dir == contractDirName,
		contractNames: make(map[string]bool),
		rootNames:     make(map[string]bool),
	}
	contractPath := c.guard.ModulePath + "/" + contractDirName
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || (importPath != contractPath && importPath != c.guard.ModulePath) {
			continue
		}
		name := path.Base(importPath)
		if importPath == c.guard.ModulePath {
			name = c.guard.RootPackageName
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		switch {
		case name == ".":
			c.report(scope, spec.Path, KindDotImport)
		case importPath == contractPath:
			scope.contractNames[name] = true
		default:
			scope.rootNames[name] = true
		}
	}

	for _, decl := range file.Decls {
		declScope := scope
		if fn, ok := decl.(*ast.FuncDecl); ok {
			declScope.funcName = fn.Name.Name
			declScope.params = paramNames(fn.Type)
		}
		c.checkNode(declScope, decl)
	}
}

func paramNames(fn *ast.FuncType) map[string]bool {
	names := make(map[string]bool)
	if fn.Params == nil {
		return names
	}
	for _, field := range fn.Params.List {
		for _, name := range field.Names {
			names[name.Name] = true
		}
	}
	return names
}

// checkNode walks one top-level declaration. Call functions, selector
// fields, and declared names are recorded as they are met so that the
// NewError references they hold are not reported as constructor values.
func (c *errorCodeChecker) checkNode(scope fileScope, decl ast.Decl) {
	notAReference := make(map[ast.Node]bool)
	if fn, ok := decl.(*ast.FuncDecl); ok {
		notAReference[fn.Name] = true
	}
	ast.Inspect(decl, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if c.isConstructor(scope, node.Fun) {
				notAReference[ast.Unparen(node.Fun)] = true
				c.checkCallCode(scope, node)
			}
		case *ast.KeyValueExpr:
			if key, ok := node.Key.(*ast.Ident); ok && key.Name == errorCodeFieldName {
				c.checkCode(scope, node.Value, KindFieldCode)
			}
		case *ast.AssignStmt:
			c.checkAssign(scope, node)
		case *ast.SelectorExpr:
			notAReference[node.Sel] = true
			if !notAReference[node] && c.isConstructor(scope, node) {
				c.report(scope, node, KindConstructorValue)
			}
		case *ast.Ident:
			if !notAReference[node] && c.isConstructor(scope, node) {
				c.report(scope, node, KindConstructorValue)
			}
		}
		return true
	})
}

func (c *errorCodeChecker) checkCallCode(scope fileScope, call *ast.CallExpr) {
	if len(call.Args) <= codeArgIndex {
		c.report(scope, call, KindCallCode)
		return
	}
	c.checkCode(scope, call.Args[codeArgIndex], KindCallCode)
}

func (c *errorCodeChecker) checkAssign(scope fileScope, assign *ast.AssignStmt) {
	for i, lhs := range assign.Lhs {
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != errorCodeFieldName {
			continue
		}
		if len(assign.Rhs) != len(assign.Lhs) {
			c.report(scope, assign.Rhs[0], KindAssignCode)
			continue
		}
		c.checkCode(scope, assign.Rhs[i], KindAssignCode)
	}
}

// isConstructor reports whether expr names contract.NewError or the root
// NewError in this file.
func (c *errorCodeChecker) isConstructor(scope fileScope, expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		return ok && e.Sel.Name == errorConstructorName && (scope.contractNames[pkg.Name] || scope.rootNames[pkg.Name])
	case *ast.Ident:
		return e.Name == errorConstructorName && (scope.inRoot || scope.inContract)
	case *ast.ParenExpr:
		return c.isConstructor(scope, e.X)
	default:
		return false
	}
}

func (c *errorCodeChecker) checkCode(scope fileScope, code ast.Expr, kind string) {
	if c.isRegistered(scope, code) || c.isPassThrough(scope, code) {
		return
	}
	c.report(scope, code, kind)
}

func (c *errorCodeChecker) isRegistered(scope fileScope, code ast.Expr) bool {
	switch e := code.(type) {
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		return ok && scope.contractNames[pkg.Name] && c.registered[e.Sel.Name]
	case *ast.Ident:
		return scope.inContract && c.registered[e.Name]
	case *ast.ParenExpr:
		return c.isRegistered(scope, e.X)
	default:
		return false
	}
}

func (c *errorCodeChecker) isPassThrough(scope fileScope, code ast.Expr) bool {
	ident, ok := code.(*ast.Ident)
	if !ok || !scope.params[ident.Name] {
		return false
	}
	for i, entry := range c.guard.PassThroughs {
		if entry.File == scope.rel && entry.Func == scope.funcName && entry.Param == ident.Name {
			c.matched[i] = true
			return true
		}
	}
	return false
}

func (c *errorCodeChecker) report(scope fileScope, node ast.Node, kind string) {
	var expr bytes.Buffer
	if err := printer.Fprint(&expr, c.fset, node); err != nil {
		expr.Reset()
		fmt.Fprintf(&expr, "<unprintable: %v>", err)
	}
	c.violations = append(c.violations, ErrorCodeViolation{
		File: scope.rel,
		Line: c.fset.Position(node.Pos()).Line,
		Kind: kind,
		Expr: expr.String(),
	})
}
