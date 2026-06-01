package ax

import "context"

type contextKey string

const (
	modeContextKey           contextKey = "ax.mode"
	dryRunContextKey         contextKey = "ax.dry_run"
	idempotencyKeyContextKey contextKey = "ax.idempotency_key"
)

// WithMode returns a context carrying the resolved output mode.
func WithMode(ctx context.Context, mode Mode) context.Context {
	return context.WithValue(ctx, modeContextKey, mode)
}

// ModeFromContext returns the resolved output mode stored in ctx.
func ModeFromContext(ctx context.Context) (Mode, bool) {
	mode, ok := ctx.Value(modeContextKey).(Mode)
	return mode, ok
}

// WithDryRun returns a context carrying the dry-run state.
func WithDryRun(ctx context.Context, dryRun bool) context.Context {
	return context.WithValue(ctx, dryRunContextKey, dryRun)
}

// DryRunFromContext reports whether dry-run behavior is active.
func DryRunFromContext(ctx context.Context) bool {
	dryRun, _ := ctx.Value(dryRunContextKey).(bool)
	return dryRun
}

// WithIdempotencyKey returns a context carrying the idempotency key for the run.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, idempotencyKeyContextKey, key)
}

// IdempotencyKeyFromContext returns the idempotency key stored in ctx.
func IdempotencyKeyFromContext(ctx context.Context) (string, bool) {
	key, ok := ctx.Value(idempotencyKeyContextKey).(string)
	return key, ok && key != ""
}
