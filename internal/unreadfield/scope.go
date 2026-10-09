package unreadfield

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
)

// collectCandidates registers every struct field declared in files that is in
// scope (research R6): declared in this unit, not in a generated file, not
// blank, and not exposed to a package outside this one.
func (a *analysis) collectCandidates(files []*ast.File) {
	exposed := a.exposedFields()
	for _, file := range files {
		if ast.IsGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			a.collectDecl(decl, exposed)
		}
	}
}

func (a *analysis) collectDecl(decl ast.Decl, exposed map[*types.Var]bool) {
	switch d := decl.(type) {
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			a.collectIn(spec, exposed)
		}
	case *ast.FuncDecl:
		a.collectIn(d, exposed)
	}
}

// exposedFields returns the fields a downstream package can read without the
// analysis seeing it: every exported or embedded field of a struct reachable
// from an exported package-level type, alias, variable, or function. An
// embedded field is exposed even when its name is not, because reading a
// field or calling a method it promotes reads it.
func (a *analysis) exposedFields() map[*types.Var]bool {
	exposed := make(map[*types.Var]bool)
	seen := make(map[types.Type]bool)
	scope := a.pkg.Scope()
	for _, name := range scope.Names() {
		if obj := scope.Lookup(name); obj.Exported() {
			a.expose(obj.Type(), seen, exposed)
		}
	}
	return exposed
}

// expose records the fields reachable from t through pointers, containers,
// signatures, tuples, type arguments, interface methods, the exported methods
// of this package's named types, and exported or embedded struct fields.
// Unexported, non-embedded fields are not followed: downstream code cannot
// name them. seen guards against recursive types.
func (a *analysis) expose(t types.Type, seen map[types.Type]bool, exposed map[*types.Var]bool) {
	t = types.Unalias(t)
	if t == nil || seen[t] {
		return
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.Named:
		a.exposeNamed(u, seen, exposed)
	case *types.Pointer:
		a.expose(u.Elem(), seen, exposed)
	case *types.Slice:
		a.expose(u.Elem(), seen, exposed)
	case *types.Array:
		a.expose(u.Elem(), seen, exposed)
	case *types.Chan:
		a.expose(u.Elem(), seen, exposed)
	case *types.Map:
		a.expose(u.Key(), seen, exposed)
		a.expose(u.Elem(), seen, exposed)
	case *types.Signature:
		a.expose(u.Params(), seen, exposed)
		a.expose(u.Results(), seen, exposed)
	case *types.Tuple:
		for v := range u.Variables() {
			a.expose(v.Type(), seen, exposed)
		}
	case *types.Interface:
		for m := range u.ExplicitMethods() {
			a.exposeMethod(m, seen, exposed)
		}
		for e := range u.EmbeddedTypes() {
			a.expose(e, seen, exposed)
		}
	case *types.Struct:
		for v := range u.Fields() {
			if v.Exported() || v.Embedded() {
				exposed[v.Origin()] = true
				a.expose(v.Type(), seen, exposed)
			}
		}
	}
}

// exposeNamed follows a named type's type arguments and, for this package's
// own types, its exported methods and underlying type. A type from another
// package cannot contain this package's types except through its arguments.
func (a *analysis) exposeNamed(n *types.Named, seen map[types.Type]bool, exposed map[*types.Var]bool) {
	for arg := range n.TypeArgs().Types() {
		a.expose(arg, seen, exposed)
	}
	if n.Obj().Pkg() != a.pkg {
		return
	}
	for m := range n.Origin().Methods() {
		a.exposeMethod(m, seen, exposed)
	}
	a.expose(n.Origin().Underlying(), seen, exposed)
}

func (a *analysis) exposeMethod(m *types.Func, seen map[types.Type]bool, exposed map[*types.Var]bool) {
	if m.Exported() {
		a.expose(m.Type(), seen, exposed)
	}
}

// collectIn registers the fields of every struct type under root. Only a
// package-level type spec is ever root itself; any other type spec is
// function-local, and its name alone is not unique within the package.
func (a *analysis) collectIn(root ast.Node, exposed map[*types.Var]bool) {
	named := make(map[*ast.StructType]bool)
	ast.Inspect(root, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.TypeSpec:
			if st, ok := n.Type.(*ast.StructType); ok {
				name := n.Name.Name
				if n != root {
					name = a.qualify(name, n.Name.Pos())
				}
				a.register(st, name, exposed)
				named[st] = true
			}
		case *ast.StructType:
			if !named[n] {
				a.register(n, a.qualify("struct{...}", n.Pos()), exposed)
			}
		}
		return true
	})
}

// qualify appends a machine-independent physical position to a type name
// that is not unique on its own.
func (a *analysis) qualify(name string, pos token.Pos) string {
	p := a.fset.PositionFor(pos, false)
	return fmt.Sprintf("%s@%s:%d:%d", name, filepath.Base(p.Filename), p.Line, p.Column)
}

func (a *analysis) register(st *ast.StructType, typeName string, exposed map[*types.Var]bool) {
	s, ok := a.info.TypeOf(st).(*types.Struct)
	if !ok {
		return
	}
	for v := range s.Fields() {
		if v.Name() == "_" || exposed[v.Origin()] {
			continue
		}
		a.cands[v.Origin()] = &candidate{
			key:      FieldKey{Package: a.pkg.Path(), Type: typeName, Field: v.Name()},
			declared: v.Pos(),
		}
	}
}
