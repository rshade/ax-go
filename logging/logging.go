package logging

import (
	"context"
	"io"

	"github.com/rs/zerolog"

	"github.com/rshade/ax-go/internal/logcore"
)

// The declarations below are identity-preserving type aliases (=) and thin funcs
// over internal/logcore, the single logger implementation. Root package ax aliases
// the SAME declarations, so the two public surfaces are siblings rather than a
// chain and a value from either satisfies the other without conversion.
//
// Sink, LabelSanctioner, and Config are deliberately NOT re-exported. That
// omission is the whole of the no-pluggable-backend guarantee for external
// consumers: logcore lives under internal/, which the toolchain forbids any other
// module from importing, so with no re-export here there is no reachable name
// through which an external backend could ever be registered.

// Logger is the canonical structured-logging surface, backed by zerolog. Every
// emitted line carries trace_id and span_id — the active span's values when one
// is present, and valid zero-value hex constants when none is, so a consumer
// parser never has to branch on absence.
//
// This is the same type as ax.Logger. The interface is a migration seam, not a
// pluggable-backend selector (Constitution Principle VI).
type Logger = logcore.Logger

// Labels are the low-cardinality descriptors attached to every log line:
// environment, application, host, and version. Empty fields are omitted from the
// line entirely rather than emitted as empty strings.
//
// The set is closed. High-cardinality values — trace and span IDs, user IDs,
// durations, resource IDs — are payload fields and must never be carried here.
//
// This is the same type as ax.Labels.
type Labels = logcore.Labels

// LoggerOption configures NewLogger. This is the same type as ax.LoggerOption, so
// an option manufactured by root ax — including ax.WithLokiFromEnv — is accepted
// by NewLogger without conversion.
type LoggerOption = logcore.Option

// WithLoggerWriter sets the logger output writer. Defaults to stderr, which keeps
// log output on the diagnostic stream; the payload stream is reserved for a
// command's final machine payload (Constitution Principle I).
func WithLoggerWriter(w io.Writer) LoggerOption {
	return logcore.WithWriter(w)
}

// WithLoggerLevel sets the minimum zerolog level. Defaults to info. Events below
// the configured level never construct, so a filtered call costs nothing beyond
// the level comparison and never runs the trace-correlation hook.
func WithLoggerLevel(level zerolog.Level) LoggerOption {
	return logcore.WithLevel(level)
}

// WithLoggerLabels attaches low-cardinality labels to every log line. Option
// order is irrelevant: every option is applied before the label set is acted on.
func WithLoggerLabels(labels Labels) LoggerOption {
	return logcore.WithLabels(labels)
}

// NewLogger returns a Logger backed by zerolog and wired for trace correlation.
// It never returns nil, so a caller never has to check.
//
// The returned logger writes to stderr at info level unless WithLoggerWriter or
// WithLoggerLevel says otherwise. With no active span in ctx, the emission path
// allocates nothing.
func NewLogger(ctx context.Context, opts ...LoggerOption) Logger {
	return logcore.New(ctx, opts...)
}

// Flush drains any buffered log entries held by l's sinks and returns nil for a
// nil logger, so it is safe to call unconditionally in a shutdown path.
//
// For a consumer of this package alone it performs no work and always returns
// nil. That is not a temporary state: the only buffering destination ax-go ships
// is the Loki direct-push sink, which lives in root package ax because it
// requires net/http — precisely the dependency this surface exists to exclude.
// Nothing reachable from here can register a sink, so there is nothing to drain.
//
// It is provided anyway for two reasons. A consumer that mixes surfaces may hold
// a logger built by ax.NewLogger with a Loki sink attached, and this function
// drains it correctly because the types are identical. And a shutdown path that
// calls Flush unconditionally keeps working unchanged if it later migrates to the
// root facade.
func Flush(ctx context.Context, l Logger) error {
	return logcore.Flush(ctx, l)
}
