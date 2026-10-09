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
// index or slice base, a receive, the callee of a call) is harmless here
// because the wrapper is itself an expression whose own parent is judged.
// Every copy is judged by copyHarmless against the type it is stored as.
func (a *analysis) harmless(e ast.Expr, stack []ast.Node) bool {
	switch p := stack[len(stack)-1].(type) {
	case *ast.ParenExpr, *ast.StarExpr, *ast.ExprStmt:
		return true
	case *ast.UnaryExpr:
		return p.Op == token.AND || p.Op == token.ARROW
	case *ast.SelectorExpr:
		return p.X == e
	case *ast.IndexExpr:
		return p.X == e
	case *ast.SliceExpr:
		return p.X == e
	case *ast.RangeStmt:
		return a.rangeHarmless(p, e)
	case *ast.AssignStmt:
		return a.assignHarmless(p, e)
	case *ast.ValueSpec:
		return p.Type == nil || a.copyHarmless(a.info.TypeOf(e), a.info.TypeOf(p.Type))
	case *ast.CompositeLit:
		return a.copyHarmless(a.info.TypeOf(e), a.elementType(p, slices.Index(p.Elts, e)))
	case *ast.KeyValueExpr:
		return p.Value == e && len(stack) > 1 && a.keyedValueHarmless(stack[len(stack)-2], p)
	case *ast.SendStmt:
		return p.Chan == e || a.copyHarmless(a.info.TypeOf(e), chanElem(a.info.TypeOf(p.Chan)))
	case *ast.CallExpr:
		return p.Fun == e || a.argHarmless(p, e)
	case *ast.ReturnStmt:
		return a.returnHarmless(p, e, stack)
	}
	return false
}

// copyHarmless reports whether storing a value of type src as type dst keeps
// every candidate field src carries reachable through the same field object,
// so a later read through the copy is seen. An interface or type-parameter
// destination reaches no field, and a distinct but identical struct type (two
// anonymous struct literals) reaches different field objects.
func (a *analysis) copyHarmless(src, dst types.Type) bool {
	reach := make(map[*types.Var]bool)
	if dst != nil {
		a.visitType(dst, make(map[types.Type]bool), func(v *types.Var) { reach[v.Origin()] = true })
	}
	harmless := true
	if src != nil {
		a.visitType(src, make(map[types.Type]bool), func(v *types.Var) {
			harmless = harmless && (a.cands[v.Origin()] == nil || reach[v.Origin()])
		})
	}
	return harmless
}

// assignHarmless allows the left side of = and :=, and a right-hand value
// every destination of which is harmless. A single tuple-valued right-hand
// side (a call, or a comma-ok form) is paired with the destinations
// element by element.
func (a *analysis) assignHarmless(assign *ast.AssignStmt, e ast.Expr) bool {
	if slices.Contains(assign.Lhs, e) {
		return true
	}
	src := a.info.TypeOf(e)
	if len(assign.Lhs) == len(assign.Rhs) {
		return a.destinationHarmless(src, assign.Lhs[slices.Index(assign.Rhs, e)])
	}
	tuple, ok := src.(*types.Tuple)
	if !ok || tuple.Len() != len(assign.Lhs) {
		return false
	}
	for i, lhs := range assign.Lhs {
		if !a.destinationHarmless(tuple.At(i).Type(), lhs) {
			return false
		}
	}
	return true
}

// destinationHarmless judges storing a src value into lhs. An absent or blank
// destination discards the value and is checked before any type lookup:
// go/types records no type for the blank identifier.
func (a *analysis) destinationHarmless(src types.Type, lhs ast.Expr) bool {
	if lhs == nil {
		return true
	}
	if id, ok := lhs.(*ast.Ident); ok && id.Name == "_" {
		return true
	}
	return a.copyHarmless(src, a.info.TypeOf(lhs))
}

// rangeHarmless allows the key and value variables themselves, and a ranged
// collection whose elements are copied into new variables (:=) or into
// existing ones that store them harmlessly (=). A range over a function
// iterator that assigns to existing variables is not judged.
func (a *analysis) rangeHarmless(r *ast.RangeStmt, e ast.Expr) bool {
	if r.X != e || r.Tok != token.ASSIGN {
		return true
	}
	t := a.info.TypeOf(e)
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	var key, value types.Type
	switch u := t.Underlying().(type) {
	case *types.Slice:
		value = u.Elem()
	case *types.Array:
		value = u.Elem()
	case *types.Map:
		key, value = u.Key(), u.Elem()
	case *types.Chan:
		key = u.Elem()
	default:
		return false
	}
	return a.destinationHarmless(key, r.Key) && a.destinationHarmless(value, r.Value)
}

// elementType returns the type a composite literal stores its i-th positional
// element as, or nil.
func (a *analysis) elementType(lit *ast.CompositeLit, i int) types.Type {
	t := a.info.TypeOf(lit)
	if t == nil {
		return nil
	}
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		if i >= 0 && i < u.NumFields() {
			return u.Field(i).Type()
		}
	case *types.Slice:
		return u.Elem()
	case *types.Array:
		return u.Elem()
	}
	return nil
}

// keyedValueHarmless judges the value of a key: value element. Keys are never
// harmless: a map key is hashed and compared, which reads every field.
func (a *analysis) keyedValueHarmless(parent ast.Node, kv *ast.KeyValueExpr) bool {
	lit, ok := parent.(*ast.CompositeLit)
	if !ok {
		return false
	}
	src := a.info.TypeOf(kv.Value)
	if s := structOf(a.info.TypeOf(lit)); s != nil {
		key, _ := kv.Key.(*ast.Ident)
		field, isField := a.info.Uses[key].(*types.Var)
		return isField && a.copyHarmless(src, field.Type())
	}
	t := a.info.TypeOf(lit)
	if t == nil {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Map:
		return a.copyHarmless(src, u.Elem())
	case *types.Slice:
		return a.copyHarmless(src, u.Elem())
	case *types.Array:
		return a.copyHarmless(src, u.Elem())
	}
	return false
}

// argHarmless allows an argument to the builtins that only move values, or to
// a function or method that resolves statically to this package and stores
// the argument harmlessly as its parameter type. The parameter is taken from
// the generic origin, so a *T or []T parameter reaches no field: a generic
// callee can pass it on to code that sees only an interface. Conversions,
// dynamic callees, and functions in other packages are not allowed.
func (a *analysis) argHarmless(call *ast.CallExpr, e ast.Expr) bool {
	fun := ast.Unparen(call.Fun)
	tv := a.info.Types[fun]
	if tv.IsType() {
		return false
	}
	if tv.IsBuiltin() {
		return a.builtinArgHarmless(call, fun, e)
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
	return param != nil && a.copyHarmless(a.info.TypeOf(e), param)
}

// builtinArgHarmless allows the slice, map, or channel operand of a builtin
// that only measures or moves values, and an appended value the slice element
// type stores harmlessly. A delete key is hashed and compared, so it is never
// harmless.
func (a *analysis) builtinArgHarmless(call *ast.CallExpr, fun, e ast.Expr) bool {
	id, _ := fun.(*ast.Ident)
	if id == nil {
		return false
	}
	switch id.Name {
	case "len", "cap", "copy", "clear":
		return true
	case "delete":
		return slices.Index(call.Args, e) == 0
	case "append":
		if slices.Index(call.Args, e) == 0 {
			return true
		}
		s, ok := a.info.TypeOf(call).Underlying().(*types.Slice)
		if !ok {
			return false
		}
		if call.Ellipsis.IsValid() {
			return a.copyHarmless(a.info.TypeOf(e), s)
		}
		return a.copyHarmless(a.info.TypeOf(e), s.Elem())
	}
	return false
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
// literal that stores the value harmlessly as its result type: its callers
// are in this package, where the analysis sees what they do with the value.
// A function literal is itself a value carrying its result types, judged
// wherever it goes.
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
	// call spread across several results is paired with them element by
	// element.
	src := a.info.TypeOf(e)
	if i := slices.Index(ret.Results, e); len(ret.Results) == len(resultTypes) && i >= 0 {
		return a.copyHarmless(src, resultTypes[i])
	}
	tuple, ok := src.(*types.Tuple)
	if !ok || tuple.Len() != len(resultTypes) {
		return false
	}
	for i, t := range resultTypes {
		if !a.copyHarmless(tuple.At(i).Type(), t) {
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
// pointers, slices, arrays, maps, channels, function parameters and results,
// tuples, nested struct fields, and type arguments. A function value carries
// the structs its signature mentions: code that receives it can obtain them.
// seen guards against recursive types.
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
	case *types.Signature:
		a.visitType(u.Params(), seen, visit)
		a.visitType(u.Results(), seen, visit)
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
