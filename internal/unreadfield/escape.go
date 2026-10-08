package unreadfield

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
)

// checkWholeValue applies research R5. A carrier is a value whose type
// contains a candidate struct. Every use of a carrier outside the allow-list
// of harmless positions marks every field of every struct it contains as
// read, because equality, interface conversion, reflection, and code outside
// the package can all observe a field without naming it.
func (a *analysis) checkWholeValue(e ast.Expr, stack []ast.Node) {
	tv, ok := a.info.Types[e]
	if !ok || !tv.IsValue() || len(stack) == 0 || !a.carries(tv.Type, make(map[types.Type]bool)) {
		return
	}
	if !a.harmless(e, stack) {
		a.markType(tv.Type, make(map[types.Type]bool))
	}
}

// harmless reports whether e's parent only copies, accesses, or passes the
// value to code this analysis sees. A wrapper parent (parentheses, &, *, an
// index or slice base, a receive) is harmless here because the wrapper is
// itself an expression whose own parent is judged.
func (a *analysis) harmless(e ast.Expr, stack []ast.Node) bool {
	switch p := stack[len(stack)-1].(type) {
	case *ast.ParenExpr, *ast.StarExpr, *ast.ExprStmt, *ast.RangeStmt:
		return true
	case *ast.UnaryExpr:
		return p.Op == token.AND || p.Op == token.ARROW
	case *ast.SelectorExpr:
		return p.X == e
	case *ast.IndexExpr:
		return p.X == e
	case *ast.SliceExpr:
		return p.X == e
	case *ast.AssignStmt:
		return a.assignHarmless(p, e)
	case *ast.ValueSpec:
		return p.Type == nil || !types.IsInterface(a.info.TypeOf(p.Type))
	case *ast.CompositeLit:
		return a.elementHarmless(p, slices.Index(p.Elts, e))
	case *ast.KeyValueExpr:
		return p.Value == e && len(stack) > 1 && a.keyedValueHarmless(stack[len(stack)-2], p)
	case *ast.SendStmt:
		return p.Chan == e || !types.IsInterface(chanElem(a.info.TypeOf(p.Chan)))
	case *ast.CallExpr:
		return a.argHarmless(p, e)
	case *ast.ReturnStmt:
		return a.returnHarmless(p, e, stack)
	}
	return false
}

// assignHarmless allows both sides of = and :=, provided every destination
// the value reaches is a blank identifier or has a non-interface type.
func (a *analysis) assignHarmless(assign *ast.AssignStmt, e ast.Expr) bool {
	if slices.Contains(assign.Lhs, e) {
		return true
	}
	if len(assign.Lhs) == len(assign.Rhs) {
		return a.destinationHarmless(assign.Lhs[slices.Index(assign.Rhs, e)])
	}
	for _, lhs := range assign.Lhs {
		if !a.destinationHarmless(lhs) {
			return false
		}
	}
	return true
}

// destinationHarmless treats _ as harmless before any type check: go/types
// records no type for the blank identifier.
func (a *analysis) destinationHarmless(lhs ast.Expr) bool {
	if id, ok := lhs.(*ast.Ident); ok && id.Name == "_" {
		return true
	}
	t := a.info.TypeOf(lhs)
	return t != nil && !types.IsInterface(t)
}

// elementHarmless judges the i-th positional element of a composite literal
// by the type the literal stores it as.
func (a *analysis) elementHarmless(lit *ast.CompositeLit, i int) bool {
	t := a.info.TypeOf(lit)
	if t == nil {
		return false
	}
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return i < u.NumFields() && !types.IsInterface(u.Field(i).Type())
	case *types.Slice:
		return !types.IsInterface(u.Elem())
	case *types.Array:
		return !types.IsInterface(u.Elem())
	}
	return false
}

// keyedValueHarmless judges the value of a key: value element. Keys are never
// harmless: a map key is hashed and compared, which reads every field.
func (a *analysis) keyedValueHarmless(parent ast.Node, kv *ast.KeyValueExpr) bool {
	lit, ok := parent.(*ast.CompositeLit)
	if !ok {
		return false
	}
	if s := structOf(a.info.TypeOf(lit)); s != nil {
		key, _ := kv.Key.(*ast.Ident)
		field, isField := a.info.Uses[key].(*types.Var)
		return isField && !types.IsInterface(field.Type())
	}
	t := a.info.TypeOf(lit)
	if t == nil {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Map:
		return !types.IsInterface(u.Elem())
	case *types.Slice:
		return !types.IsInterface(u.Elem())
	case *types.Array:
		return !types.IsInterface(u.Elem())
	}
	return false
}

// argHarmless allows an argument to the builtins that only move values, or to
// a function or method that resolves statically to this package and takes a
// non-interface, non-type-parameter parameter. Conversions, dynamic callees,
// and functions in other packages are not allowed.
func (a *analysis) argHarmless(call *ast.CallExpr, e ast.Expr) bool {
	fun := ast.Unparen(call.Fun)
	tv := a.info.Types[fun]
	if tv.IsType() {
		return false
	}
	if tv.IsBuiltin() {
		id, _ := fun.(*ast.Ident)
		return id != nil && slices.Contains([]string{"append", "len", "cap", "copy", "delete", "clear"}, id.Name)
	}
	fn := a.staticCallee(fun)
	if fn == nil || fn.Pkg() != a.pkg {
		return false
	}
	sig, ok := fn.Origin().Type().(*types.Signature)
	if !ok {
		return false
	}
	param := paramType(sig, call, slices.Index(call.Args, e))
	return param != nil && !types.IsInterface(param)
}

// staticCallee resolves fun to the function or method it always calls, or
// nil for a func value, method value, interface method, or method expression.
func (a *analysis) staticCallee(fun ast.Expr) *types.Func {
	switch f := fun.(type) {
	case *ast.IndexExpr:
		return a.staticCallee(f.X)
	case *ast.IndexListExpr:
		return a.staticCallee(f.X)
	case *ast.Ident:
		fn, _ := a.info.Uses[f].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		if sel, ok := a.info.Selections[f]; ok {
			fn, _ := sel.Obj().(*types.Func)
			if sel.Kind() != types.MethodVal || types.IsInterface(sel.Recv()) {
				return nil
			}
			return fn
		}
		fn, _ := a.info.Uses[f.Sel].(*types.Func)
		return fn
	}
	return nil
}

// returnHarmless allows a return from an unexported function or a function
// literal whose result types are not interfaces: its callers are in this
// package, where the analysis sees what they do with the value.
func (a *analysis) returnHarmless(ret *ast.ReturnStmt, e ast.Expr, stack []ast.Node) bool {
	var results *ast.FieldList
	for i := len(stack) - 1; i >= 0 && results == nil; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncDecl:
			if fn.Name.IsExported() {
				return false
			}
			results = fn.Type.Results
		case *ast.FuncLit:
			results = fn.Type.Results
		}
	}
	if results == nil {
		return false
	}
	var resultTypes []types.Type
	for _, field := range results.List {
		for range max(1, len(field.Names)) {
			resultTypes = append(resultTypes, a.info.TypeOf(field.Type))
		}
	}
	// One expression per result is judged by its own result type; a single
	// call spread across several results must fit every one of them.
	if i := slices.Index(ret.Results, e); len(ret.Results) == len(resultTypes) && i >= 0 {
		resultTypes = resultTypes[i : i+1]
	}
	for _, t := range resultTypes {
		if t == nil || types.IsInterface(t) {
			return false
		}
	}
	return true
}

// carries reports whether t contains a candidate struct. Named types from
// other packages are entered only through their type arguments: a non-generic
// foreign type cannot contain this package's types.
func (a *analysis) carries(t types.Type, seen map[types.Type]bool) bool {
	found := false
	a.visitType(t, seen, func(v *types.Var) { found = found || a.cands[v.Origin()] != nil })
	return found
}

// markType marks every field of every struct t contains as read.
func (a *analysis) markType(t types.Type, seen map[types.Type]bool) {
	a.visitType(t, seen, a.markRead)
}

// visitType calls visit for every struct field reachable from t through
// pointers, slices, arrays, maps, channels, tuples, nested struct fields, and
// type arguments. seen guards against recursive types.
func (a *analysis) visitType(t types.Type, seen map[types.Type]bool, visit func(*types.Var)) {
	t = types.Unalias(t)
	if t == nil || seen[t] {
		return
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.Named:
		for arg := range u.TypeArgs().Types() {
			a.visitType(arg, seen, visit)
		}
		if u.Obj().Pkg() == a.pkg {
			a.visitType(u.Origin().Underlying(), seen, visit)
		}
	case *types.Pointer:
		a.visitType(u.Elem(), seen, visit)
	case *types.Slice:
		a.visitType(u.Elem(), seen, visit)
	case *types.Array:
		a.visitType(u.Elem(), seen, visit)
	case *types.Chan:
		a.visitType(u.Elem(), seen, visit)
	case *types.Map:
		a.visitType(u.Key(), seen, visit)
		a.visitType(u.Elem(), seen, visit)
	case *types.Tuple:
		for v := range u.Variables() {
			a.visitType(v.Type(), seen, visit)
		}
	case *types.Struct:
		for v := range u.Fields() {
			visit(v)
			a.visitType(v.Type(), seen, visit)
		}
	}
}

func chanElem(t types.Type) types.Type {
	if c, ok := t.Underlying().(*types.Chan); ok {
		return c.Elem()
	}
	return nil
}

// paramType returns the type the i-th argument of call is passed as,
// unwrapping a variadic tail unless the call spreads a slice with "...".
func paramType(sig *types.Signature, call *ast.CallExpr, i int) types.Type {
	params := sig.Params()
	n := params.Len()
	if i < 0 || n == 0 {
		return nil
	}
	if sig.Variadic() && i >= n-1 {
		last := params.At(n - 1).Type()
		if call.Ellipsis.IsValid() {
			return last
		}
		if s, ok := last.(*types.Slice); ok {
			return s.Elem()
		}
		return nil
	}
	if i < n {
		return params.At(i).Type()
	}
	return nil
}
