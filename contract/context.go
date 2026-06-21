package contract

import "context"

type contextKey string

const (
	modeContextKey           contextKey = "ax.mode"
	dryRunContextKey         contextKey = "ax.dry_run"
	idempotencyKeyContextKey contextKey = "ax.idempotency_key"
	metadataContextKey       contextKey = "ax.metadata"
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

// WithMetadata returns a context carrying explicit machine-envelope metadata.
// Stored values are normalized on read by MetadataFromContext, so they are kept
// here as supplied.
func WithMetadata(ctx context.Context, metadata Metadata) context.Context {
	return context.WithValue(ctx, metadataContextKey, metadata)
}

// MetadataFromContext returns explicit metadata from ctx merged with dry-run
// and idempotency-key context helpers.
func MetadataFromContext(ctx context.Context) Metadata {
	if ctx == nil {
		ctx = context.Background()
	}

	metadata, _ := ctx.Value(metadataContextKey).(Metadata)
	metadata = normalizeMetadata(metadata)
	if key, ok := IdempotencyKeyFromContext(ctx); ok {
		metadata.IdempotencyKey = key
	}
	if DryRunFromContext(ctx) {
		metadata.DryRun = true
	}
	return metadata
}

// TraceIDFromContext returns the explicit trace ID stored in ctx or
// ZeroTraceID when no trace metadata is present.
func TraceIDFromContext(ctx context.Context) string {
	return MetadataFromContext(ctx).TraceID
}

// SpanIDFromContext returns the explicit span ID stored in ctx or ZeroSpanID
// when no span metadata is present.
func SpanIDFromContext(ctx context.Context) string {
	return MetadataFromContext(ctx).SpanID
}
