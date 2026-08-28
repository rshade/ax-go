// Package diagwriter carries a context-scoped diagnostic writer between a
// runtime that has already resolved and locked its own stderr destination
// (ax.Execute, the MCP dispatcher) and a Logger a callee constructs internally
// via a bare NewLogger(ctx) call with no explicit writer option.
//
// It exists as its own package, importing nothing beyond context and io from
// the standard library, so that both internal/logcore (which needs zerolog)
// and internal/mcpserver (which the mcp public surface's import-isolation
// gate, mcp/import_isolation_test.go's TestMCPStaysThin, forbids from ever
// pulling zerolog into its transitive dependency graph) can depend on this
// mechanism without either pulling in the other's dependency tree. Do not add
// an import here — doing so risks breaking one of those two isolation
// guarantees.
package diagwriter

import (
	"context"
	"io"
)

type contextKey struct{}

// WithWriter returns a context carrying w as the diagnostic writer a Logger
// constructed via NewLogger(ctx) with no explicit writer option should use
// instead of its os.Stderr default. It lets a runtime that has already
// resolved and locked its own stderr writer (ax.Execute, the MCP dispatcher)
// route logger output a callee constructs internally through the same sink,
// without every internal caller needing to pass an explicit writer option.
func WithWriter(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, contextKey{}, w)
}

// FromContext returns the diagnostic writer carried by ctx, if any.
func FromContext(ctx context.Context) (io.Writer, bool) {
	if ctx == nil {
		return nil, false
	}
	w, ok := ctx.Value(contextKey{}).(io.Writer)
	return w, ok
}
