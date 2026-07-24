package logging_test

import (
	"context"
	"io"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/logging"
)

// The assertions in this file are largely COMPILE-TIME. If the alias chain were
// broken — if either surface redeclared a type instead of aliasing it — this file
// would fail to build, which is the strongest form the guarantee can take.
//
// It lives in the external test package logging_test so its import of root ax
// cannot contribute to the logging package's own dependency graph, keeping the
// import-isolation assertion unambiguous.

// Package-level declarations pin the type identities before any test runs, so a
// broken alias is a build failure rather than a test failure.
var (
	_ ax.Logger            = logging.Logger(nil)
	_ logging.Logger       = ax.Logger(nil)
	_ ax.Labels            = logging.Labels{}
	_ logging.Labels       = ax.Labels{}
	_ ax.LoggerOption      = logging.LoggerOption(nil)
	_ logging.LoggerOption = ax.LoggerOption(nil)

	// The functions must stay FUNCTIONS at both levels. go-apidiff classifies a
	// func→var conversion as breaking, and these declarations fail to compile if
	// either surface ever makes that change.
	_ func(context.Context, ...ax.LoggerOption) ax.Logger           = ax.NewLogger
	_ func(context.Context, ...logging.LoggerOption) logging.Logger = logging.NewLogger
	_ func(context.Context, ax.Logger) error                        = ax.Flush
	_ func(context.Context, logging.Logger) error                   = logging.Flush
)

// TestCrossSurfaceIdentity exercises the contracts/logging-package.md
// § Cross-surface identity contract at runtime as well as at compile time: a
// value produced by either surface is accepted by the other without conversion.
func TestCrossSurfaceIdentity(t *testing.T) {
	ctx := context.Background()

	var a ax.Logger = logging.NewLogger(ctx, logging.WithLoggerWriter(io.Discard))
	if a == nil {
		t.Fatal("logging.NewLogger produced a nil ax.Logger")
	}

	var b logging.Logger = ax.NewLogger(ctx, ax.WithLoggerWriter(io.Discard))
	if b == nil {
		t.Fatal("ax.NewLogger produced a nil logging.Logger")
	}

	if err := ax.Flush(ctx, logging.NewLogger(ctx, logging.WithLoggerWriter(io.Discard))); err != nil {
		t.Errorf("ax.Flush on a logging logger = %v, want nil", err)
	}
	if err := logging.Flush(ctx, ax.NewLogger(ctx, ax.WithLoggerWriter(io.Discard))); err != nil {
		t.Errorf("logging.Flush on an ax logger = %v, want nil", err)
	}
}

// TestRootManufacturedOptionIsAcceptedByIsolatedConstructor is the sharpest case
// in the identity contract (FR-005).
//
// ax.WithLokiFromEnv is manufactured by root ax, closes over root-only machinery,
// and is handed to the ISOLATED constructor. It compiling and running at all
// proves the alias chain is genuinely unbroken — LoggerOption is one type across
// all three packages, not three structurally-identical ones. A redeclaration
// anywhere in the chain fails this at compile time.
//
// Note what this does NOT do: importing root ax here does not give the logging
// package a dependency on net/http, because this is an external test package. The
// isolation assertion in import_isolation_test.go is unaffected, and both hold
// simultaneously.
func TestRootManufacturedOptionIsAcceptedByIsolatedConstructor(t *testing.T) {
	// Unset so the option is a no-op: this test is about type identity, not about
	// standing up a Loki endpoint.
	t.Setenv("AX_LOKI_URL", "")

	ctx := context.Background()
	logger := logging.NewLogger(ctx,
		logging.WithLoggerWriter(io.Discard),
		ax.WithLokiFromEnv(),
	)

	if logger == nil {
		t.Fatal("logging.NewLogger returned nil when given a root-manufactured option")
	}
	logger.Info(ctx).Msg("root option accepted by the isolated constructor")
}

// TestOptionsInterleaveAcrossSurfaces goes one step further than the contract
// requires: options from BOTH surfaces are mixed in one call, in an order that
// interleaves them. If LoggerOption were merely structurally similar rather than
// identical, this would not compile.
func TestOptionsInterleaveAcrossSurfaces(t *testing.T) {
	t.Setenv("AX_LOKI_URL", "")

	ctx := context.Background()
	logger := logging.NewLogger(ctx,
		ax.WithLoggerWriter(io.Discard),
		logging.WithLoggerLabels(logging.Labels{Application: "mixed"}),
		ax.WithLokiFromEnv(),
		logging.WithLoggerWriter(io.Discard),
	)
	if logger == nil {
		t.Fatal("interleaved options produced a nil logger")
	}

	// And the same set through the root constructor.
	rootLogger := ax.NewLogger(ctx,
		logging.WithLoggerWriter(io.Discard),
		ax.WithLoggerLabels(ax.Labels{Application: "mixed"}),
	)
	if rootLogger == nil {
		t.Fatal("interleaved options produced a nil root logger")
	}
}

// TestDerivedLoggerCrossesSurfaces asserts WithLabels — which returns the
// interface type — also preserves identity, so a logger derived on one surface
// remains usable on the other.
func TestDerivedLoggerCrossesSurfaces(t *testing.T) {
	ctx := context.Background()

	derivedFromIsolated := logging.NewLogger(ctx, logging.WithLoggerWriter(io.Discard)).
		WithLabels(logging.Labels{Application: "derived"})
	var asRoot ax.Logger = derivedFromIsolated
	if asRoot == nil {
		t.Fatal("a logger derived on the isolated surface is not an ax.Logger")
	}

	derivedFromRoot := ax.NewLogger(ctx, ax.WithLoggerWriter(io.Discard)).
		WithLabels(ax.Labels{Application: "derived"})
	var asIsolated logging.Logger = derivedFromRoot
	if asIsolated == nil {
		t.Fatal("a logger derived on the root surface is not a logging.Logger")
	}

	if err := logging.Flush(ctx, asRoot); err != nil {
		t.Errorf("logging.Flush on a cross-surface derived logger = %v, want nil", err)
	}
}
