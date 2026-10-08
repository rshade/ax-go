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
// blank, and not an exported field of a type nameable outside the package.
func (a *analysis) collectCandidates(files []*ast.File) {
	exportedAliases := a.exportedAliasTargets(files)
	for _, file := range files {
		if ast.IsGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			a.collectDecl(decl, exportedAliases)
		}
	}
}

// exportedAliasTargets returns the package's named types that an exported
// package-level alias exposes (type Exported = hidden, the export_test.go
// pattern). Their exported fields are nameable outside the package.
func (a *analysis) exportedAliasTargets(files []*ast.File) map[*types.TypeName]bool {
	targets := make(map[*types.TypeName]bool)
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, isType := spec.(*ast.TypeSpec)
				if !isType || !ts.Assign.IsValid() || !ts.Name.IsExported() {
					continue
				}
				if named, isNamed := types.Unalias(a.info.TypeOf(ts.Type)).(*types.Named); isNamed &&
					named.Obj().Pkg() == a.pkg {
					targets[named.Origin().Obj()] = true
				}
			}
		}
	}
	return targets
}

func (a *analysis) collectDecl(decl ast.Decl, exportedAliases map[*types.TypeName]bool) {
	switch d := decl.(type) {
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				exp := exposure(s.Name.IsExported())
				if obj, ok := a.info.Defs[s.Name].(*types.TypeName); ok && exportedAliases[obj] {
					exp = true
				}
				a.collectIn(s, exp)
			case *ast.ValueSpec:
				a.collectIn(s, exposure(anyExported(s.Names)))
			}
		}
	case *ast.FuncDecl:
		exp := exposure(d.Name.IsExported())
		if d.Recv != nil {
			a.collectIn(d.Recv, exp)
		}
		a.collectIn(d.Type, exp)
		if d.Body != nil {
			a.collectIn(d.Body, false)
		}
	}
}

// exposure records whether a declaration is nameable outside the package. An
// exposed declaration's exported fields may be read by a downstream consumer
// the analysis cannot see, so they are never candidates.
type exposure bool

func (e exposure) hides(v *types.Var) bool { return bool(e) && v.Exported() }

// collectIn registers the fields of every struct type under root. Only a
// package-level type spec is ever root itself; any other type spec is
// function-local, and its name alone is not unique within the package.
func (a *analysis) collectIn(root ast.Node, exp exposure) {
	named := make(map[*ast.StructType]bool)
	ast.Inspect(root, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.TypeSpec:
			if st, ok := n.Type.(*ast.StructType); ok {
				name := n.Name.Name
				if n != root {
					name = a.qualify(name, n.Name.Pos())
				}
				a.register(st, name, exp)
				named[st] = true
			}
		case *ast.StructType:
			if !named[n] {
				a.register(n, a.qualify("struct{...}", n.Pos()), exp)
			}
		}
		return true
	})
}

// qualify appends a machine-independent position to a type name that is not
// unique on its own.
func (a *analysis) qualify(name string, pos token.Pos) string {
	p := a.fset.Position(pos)
	return fmt.Sprintf("%s@%s:%d:%d", name, filepath.Base(p.Filename), p.Line, p.Column)
}

func (a *analysis) register(st *ast.StructType, typeName string, exp exposure) {
	s, ok := a.info.TypeOf(st).(*types.Struct)
	if !ok {
		return
	}
	for v := range s.Fields() {
		if v.Name() == "_" || exp.hides(v) {
			continue
		}
		a.cands[v.Origin()] = &candidate{
			key:      FieldKey{Package: a.pkg.Path(), Type: typeName, Field: v.Name()},
			declared: v.Pos(),
		}
	}
}

func anyExported(names []*ast.Ident) bool {
	for _, n := range names {
		if n.IsExported() {
			return true
		}
	}
	return false
}
