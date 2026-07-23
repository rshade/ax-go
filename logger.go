package ax

import (
	"context"
	"io"

	"github.com/rs/zerolog"

	"github.com/rshade/ax-go/internal/logcore"
)

// The logger implementation lives in internal/logcore, shared by this root
// facade and the import-isolated public package logging. The two are SIBLINGS
// over that implementation, never a chain: root ax must not import logging.
//
// Identity would hold either way, because aliases are transitive. The direction
// is load-bearing for two other reasons. The root runtime should depend on an
// internal package rather than on a public one; and logging's parity test imports
// root ax to compare the two surfaces byte-for-byte, so root importing logging
// would make that test an import cycle.
//
// Every declaration below is either an identity-preserving type alias (=) or a
// thin func. Neither form may be relaxed: a redeclared interface would break the
// cross-surface identity contract while still compiling, and converting a func to
// a var is classified as a breaking change by go-apidiff.

// Labels are low-cardinality Loki-indexed fields.
type Labels = logcore.Labels

// Logger is the canonical structured-logging surface, initially backed by
// zerolog. The single-backend guardrail (this interface is a migration seam, not
// a pluggable-backend selector) and the trace-correlation contract are governed
// by Constitution Principles VI and VIII.
type Logger = logcore.Logger

// LoggerOption configures NewLogger.
type LoggerOption = logcore.Option

// WithLoggerWriter sets the logger output writer. Defaults to stderr.
func WithLoggerWriter(w io.Writer) LoggerOption {
	return logcore.WithWriter(w)
}

// WithLoggerLevel sets the minimum zerolog level.
func WithLoggerLevel(level zerolog.Level) LoggerOption {
	return logcore.WithLevel(level)
}

// WithLoggerLabels attaches low-cardinality labels to every log line.
func WithLoggerLabels(labels Labels) LoggerOption {
	return logcore.WithLabels(labels)
}

// NewLogger returns an ax Logger backed by zerolog and wired for trace correlation.
// When LoggerOptions include additional sinks (e.g. WithLokiFromEnv), every log
// line is fanned out to all sinks via io.MultiWriter alongside the primary writer.
func NewLogger(ctx context.Context, opts ...LoggerOption) Logger {
	return logcore.New(ctx, opts...)
}

// Flush performs a best-effort, non-destructive drain of any buffered Loki log
// entries for the given Logger. It blocks until the buffer is empty, the context
// is cancelled, or an internal 2-second deadline elapses — whichever comes
// first. Remaining entries are dropped after the deadline.
//
// The error return is reserved for future sink implementations: the Loki sink's
// Drain returns nil on every path (push failures are fail-open diagnostics on
// the configured writer, never returned errors), so Flush currently always
// returns nil. Callers should keep checking it — new sinks may surface drain
// failures — but a failed Loki push must never change the CLI exit code.
//
// Flush is a no-op (returns nil) when:
//   - l has no Loki sink (AX_LOKI_URL was not set)
//   - l is nil
//   - the sink's background goroutine already stopped because its logger context
//     was cancelled
//
// Callers may invoke Flush multiple times; later writes remain deliverable by a
// later Flush call. Callers should invoke Flush in their shutdown path, before
// os.Exit or cobra.Command cleanup, to ensure in-flight log lines reach Loki.
func Flush(ctx context.Context, l Logger) error {
	return logcore.Flush(ctx, l)
}
