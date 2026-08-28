package ax_test

import (
	"context"
	"io"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/internal/logcore"
)

// benchAuditCtx builds a real (non-dry-run) context whose audit lines are
// routed through logcore.WithDiagnosticWriter into io.Discard, so the
// benchmark measures the audit path itself (Logger construction, field
// encoding, JSON marshal) rather than pipe or terminal I/O. This is the same
// context-carried writer mechanism Execute wires into cmd.Context() (see
// execute.go), reused here instead of a per-iteration os.Stderr pipe swap.
func benchAuditCtx() context.Context {
	return logcore.WithDiagnosticWriter(realCtx(), io.Discard)
}

// BenchmarkGuardAudit measures a real (non-dry-run) Guard call with audit
// logging enabled, the default since feature 020-guard-audit-logging. Every
// such call now constructs a Logger and writes two structured JSON lines
// around the effect, a new default-on cost on a hot path used by every
// ax-go-based CLI's mutating commands. Tracked by internal/cmd/benchcheck; see
// AGENTS.md's "Tracked Benchmarks" table.
func BenchmarkGuardAudit(b *testing.B) {
	ctx := benchAuditCtx()
	noop := func(context.Context) error { return nil }

	b.ReportAllocs()
	for b.Loop() {
		_, _ = ax.Guard(ctx, noop)
	}
}

// BenchmarkGuardAuditDisabled measures the same call on a context derived via
// ax.WithAudit(ctx, false). It is a baseline comparison point for humans
// reading go test -bench output, not itself gated for regressions — the
// audit-on path in BenchmarkGuardAudit is the one carrying the new cost.
func BenchmarkGuardAuditDisabled(b *testing.B) {
	ctx := ax.WithAudit(benchAuditCtx(), false)
	noop := func(context.Context) error { return nil }

	b.ReportAllocs()
	for b.Loop() {
		_, _ = ax.Guard(ctx, noop)
	}
}

// BenchmarkPerformAudit measures a real (non-dry-run) Perform call with audit
// logging enabled, mirroring BenchmarkGuardAudit for the commit path.
func BenchmarkPerformAudit(b *testing.B) {
	ctx := benchAuditCtx()
	noop := func(context.Context) error { return nil }

	b.ReportAllocs()
	for b.Loop() {
		_ = ax.Perform(ctx, nil, noop)
	}
}
