// Package analyzer adapts internal/unreadfield to the go/analysis framework,
// so the unread-field check runs under analysistest (// want fixtures) and,
// later, a lint runner. It holds no classification logic: Run calls
// unreadfield.Analyze, the same function the slopcheck gate uses.
//
// The adapter analyzes each package variant in isolation. It does not apply
// the gate's cross-build-configuration intersection, and run on a plain
// package it does not see reads in that package's tests. The gate
// (internal/cmd/slopcheck) is the enforcing front end.
package analyzer

import (
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/rshade/ax-go/internal/unreadfield"
)

// New returns the unreadfield analyzer. Each call returns a fresh value, so
// no analyzer state is shared between callers. Its Run reports one diagnostic
// per finding at the field's declaration, with the message
// "struct field <Type>.<Field> is assigned but never read", and never fails.
func New() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "unreadfield",
		Doc:  "report struct fields that a composite literal assigns and no code reads",
		Run:  run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	res := unreadfield.Analyze(pass.Fset, pass.Files, pass.Pkg, pass.TypesInfo)
	for _, f := range res.Findings {
		pass.Reportf(declaration(pass, f.Declared), "%s", f.Message())
	}
	return nil, nil //nolint:nilnil // an analyzer with no result type returns (nil, nil) by contract
}

// declaration maps a finding's position back to a token.Pos in pass's file
// set. Analyze reports physical positions, which ignore //line directives, so
// the parsed file and line always exist.
func declaration(pass *analysis.Pass, p unreadfield.Position) token.Pos {
	for _, file := range pass.Files {
		tf := pass.Fset.File(file.Pos())
		if tf != nil && tf.Name() == p.File {
			return tf.LineStart(p.Line) + token.Pos(p.Col-1)
		}
	}
	return token.NoPos
}
