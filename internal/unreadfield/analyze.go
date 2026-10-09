package unreadfield

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
)

// candidate is one in-scope field and what the walk has learned about it.
type candidate struct {
	key      FieldKey
	declared token.Pos
	assigned []token.Pos
	read     bool
}

// analysis holds the state of one Analyze call. Nothing outlives the call.
type analysis struct {
	fset  *token.FileSet
	pkg   *types.Package
	info  *types.Info
	cands map[*types.Var]*candidate
}

// Analyze classifies one type-checked unit. info must have Types, Defs, Uses,
// and Selections populated. A field is a finding when a composite literal in
// files assigns it and no load-position selector (research R4) or whole-value
// use (research R5) reads it. Positions are physical: //line directives are
// ignored, so every position names the file and line that was parsed. Analyze
// keeps no state between calls, and its result is sorted, so equal
// inputs give equal results.
func Analyze(fset *token.FileSet, files []*ast.File, pkg *types.Package, info *types.Info) Result {
	a := &analysis{fset: fset, pkg: pkg, info: info, cands: make(map[*types.Var]*candidate)}
	a.collectCandidates(files)
	for _, file := range files {
		a.walk(file)
	}
	return a.result()
}

// walk visits every node of file with its ancestors, which the read
// classification and the whole-value allow-list both depend on.
func (a *analysis) walk(file *ast.File) {
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		switch n := n.(type) {
		case *ast.CompositeLit:
			a.recordAssignments(n)
		case *ast.SelectorExpr:
			a.recordSelector(n, stack)
		}
		if e, ok := n.(ast.Expr); ok {
			a.checkWholeValue(e, stack)
		}
		stack = append(stack, n)
		return true
	})
}

// recordAssignments marks the fields a struct literal sets: each key of a
// keyed literal, or every field of a positional one (research R7).
func (a *analysis) recordAssignments(lit *ast.CompositeLit) {
	s := structOf(a.info.TypeOf(lit))
	if s == nil {
		return
	}
	for i, elt := range lit.Elts {
		if kv, isKeyed := elt.(*ast.KeyValueExpr); isKeyed {
			key, _ := kv.Key.(*ast.Ident)
			if v, isField := a.info.Uses[key].(*types.Var); isField {
				a.assign(v, kv.Pos())
			}
			continue
		}
		if i < s.NumFields() {
			a.assign(s.Field(i), elt.Pos())
		}
	}
}

func (a *analysis) assign(v *types.Var, pos token.Pos) {
	if c := a.cands[v.Origin()]; c != nil {
		c.assigned = append(c.assigned, pos)
	}
}

// recordSelector applies the research R4 load/store table to a field
// selector. Every embedded field on a promoted path, whether it leads to a
// field or a method, is read, because selecting through it dereferences it.
func (a *analysis) recordSelector(sel *ast.SelectorExpr, stack []ast.Node) {
	selection, ok := a.info.Selections[sel]
	if !ok {
		return
	}
	recv := selection.Recv()
	path := selection.Index()
	for _, i := range path[:len(path)-1] {
		s := structOf(recv)
		if s == nil {
			break
		}
		a.markRead(s.Field(i))
		recv = s.Field(i).Type()
	}
	field, ok := selection.Obj().(*types.Var)
	if ok && a.isLoad(sel, stack[len(stack)-1]) {
		a.markRead(field)
	}
}

// isLoad reports whether sel, whose parent is parent, reads the field.
func (a *analysis) isLoad(sel *ast.SelectorExpr, parent ast.Node) bool {
	switch p := parent.(type) {
	case *ast.AssignStmt:
		if slices.Contains(p.Lhs, ast.Expr(sel)) {
			return p.Tok != token.ASSIGN && p.Tok != token.DEFINE
		}
		return !a.isSelfAssignment(p, sel)
	case *ast.RangeStmt:
		if (p.Key == sel || p.Value == sel) && p.Tok == token.ASSIGN {
			return false
		}
	}
	return true
}

// isSelfAssignment reports whether rhs appears in x.F = x.F: the same field of
// the same base expression in the matching position, where the base has no
// calls or receives. Two evaluations of next() or <-c can yield different
// values, so next().F = next().F really reads F.
func (a *analysis) isSelfAssignment(assign *ast.AssignStmt, rhs *ast.SelectorExpr) bool {
	if assign.Tok != token.ASSIGN || len(assign.Lhs) != len(assign.Rhs) {
		return false
	}
	i := slices.Index(assign.Rhs, ast.Expr(rhs))
	lhs, ok := assign.Lhs[i].(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ls, rs := a.info.Selections[lhs], a.info.Selections[rhs]
	return ls != nil && rs != nil && ls.Obj() == rs.Obj() &&
		types.ExprString(lhs.X) == types.ExprString(rhs.X) && stable(rhs.X)
}

// stable reports whether evaluating e twice yields the same value: e contains
// no call and no channel receive.
func stable(e ast.Expr) bool {
	ok := true
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			ok = false
		case *ast.UnaryExpr:
			ok = ok && n.Op != token.ARROW
		}
		return ok
	})
	return ok
}

func (a *analysis) markRead(v *types.Var) {
	if c := a.cands[v.Origin()]; c != nil {
		c.read = true
	}
}

func (a *analysis) result() Result {
	var res Result
	for _, c := range a.cands {
		if len(c.assigned) == 0 {
			continue
		}
		res.Assigned = append(res.Assigned, c.key)
		if c.read {
			continue
		}
		assigned := make([]Position, len(c.assigned))
		for i, pos := range c.assigned {
			assigned[i] = a.position(pos)
		}
		slices.SortFunc(assigned, comparePositions)
		res.Findings = append(res.Findings, Finding{
			Key:      c.key,
			Declared: a.position(c.declared),
			Assigned: slices.Compact(assigned),
		})
	}
	slices.SortFunc(res.Assigned, compareKeys)
	slices.SortFunc(res.Findings, compareFindings)
	return res
}

func (a *analysis) position(pos token.Pos) Position {
	p := a.fset.PositionFor(pos, false)
	return Position{File: p.Filename, Line: p.Line, Col: p.Column}
}

// structOf returns the struct underlying t or *t, or nil.
func structOf(t types.Type) *types.Struct {
	if t == nil {
		return nil
	}
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	s, _ := t.Underlying().(*types.Struct)
	return s
}

func compareKeys(x, y FieldKey) int {
	return cmp.Or(
		strings.Compare(x.Package, y.Package),
		strings.Compare(x.Type, y.Type),
		strings.Compare(x.Field, y.Field),
	)
}

func compareFindings(x, y Finding) int { return compareKeys(x.Key, y.Key) }

func comparePositions(x, y Position) int {
	return cmp.Or(strings.Compare(x.File, y.File), cmp.Compare(x.Line, y.Line), cmp.Compare(x.Col, y.Col))
}
