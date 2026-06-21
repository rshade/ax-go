package ax

import (
	"context"

	"github.com/rshade/ax-go/contract"
)

// WithMode returns a context carrying the resolved output mode.
func WithMode(ctx context.Context, mode Mode) context.Context {
	return contract.WithMode(ctx, mode)
}

// ModeFromContext returns the resolved output mode stored in ctx.
func ModeFromContext(ctx context.Context) (Mode, bool) {
	return contract.ModeFromContext(ctx)
}

// WithDryRun returns a context carrying the dry-run state.
func WithDryRun(ctx context.Context, dryRun bool) context.Context {
	return contract.WithDryRun(ctx, dryRun)
}

// DryRunFromContext reports whether dry-run behavior is active.
func DryRunFromContext(ctx context.Context) bool {
	return contract.DryRunFromContext(ctx)
}

// WithIdempotencyKey returns a context carrying the idempotency key for the run.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	return contract.WithIdempotencyKey(ctx, key)
}

// IdempotencyKeyFromContext returns the idempotency key stored in ctx.
func IdempotencyKeyFromContext(ctx context.Context) (string, bool) {
	return contract.IdempotencyKeyFromContext(ctx)
}
