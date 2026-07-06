package ax

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/rs/zerolog"
)

// Labels are low-cardinality Loki-indexed fields.
type Labels struct {
	Environment string
	Application string
	Host        string
	Version     string
}

// labelField* are the zerolog field names written by applyLabels and used as
// Loki stream label keys in loki.go. They are defined here alongside Labels
// so that any change to the label name is made in one place.
const (
	labelFieldEnvironment = "environment"
	labelFieldApplication = "application"
	labelFieldHost        = "host"
	labelFieldVersion     = "version"
)

// Logger is the canonical structured-logging surface, initially backed by
// zerolog. The single-backend guardrail (this interface is a migration seam, not
// a pluggable-backend selector) and the trace-correlation contract are governed
// by Constitution Principles VI and VIII.
type Logger interface {
	Debug(ctx context.Context) *zerolog.Event
	Info(ctx context.Context) *zerolog.Event
	Warn(ctx context.Context) *zerolog.Event
	Error(ctx context.Context) *zerolog.Event
	WithLabels(labels Labels) Logger
	Zerolog() *zerolog.Logger
}

// flusher is satisfied by Logger implementations that support draining buffered
// sinks (e.g. the Loki direct-push sink). It is unexported to keep the public
// Logger interface stable; ax.Flush uses a type assertion to reach it.
type flusher interface {
	flush(ctx context.Context) error
}

// logSink is a write-through log destination that can drain buffered entries on
// shutdown. It is the single contract for an additional sink: every sink is an
// io.Writer (fanned out via io.MultiWriter in NewLogger) and exposes a
// context-aware drain (capped per-sink so shutdown cannot hang). The Loki
// direct-push writer is the only implementation today.
type logSink interface {
	io.Writer
	drain(ctx context.Context) error
}

type zerologLogger struct {
	logger zerolog.Logger
	sinks  []logSink // mirrors cfg.additionalSinks for flush access
}

// LoggerOption configures NewLogger.
type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	ctx             context.Context //nolint:containedctx // carried to bound sink goroutines to the logger lifetime
	writer          io.Writer
	level           zerolog.Level
	labels          Labels
	additionalSinks []logSink // optional extra write-through sinks
}

// WithLoggerWriter sets the logger output writer. Defaults to stderr.
func WithLoggerWriter(w io.Writer) LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.writer = w
	}
}

// WithLoggerLevel sets the minimum zerolog level.
func WithLoggerLevel(level zerolog.Level) LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.level = level
	}
}

// WithLoggerLabels attaches low-cardinality labels to every log line.
func WithLoggerLabels(labels Labels) LoggerOption {
	return func(cfg *loggerConfig) {
		cfg.labels = labels
	}
}

// NewLogger returns an ax Logger backed by zerolog and wired for trace correlation.
// When LoggerOptions include additional sinks (e.g. WithLokiFromEnv), every log
// line is fanned out to all sinks via io.MultiWriter alongside the primary writer.
func NewLogger(ctx context.Context, opts ...LoggerOption) Logger {
	cfg := loggerConfig{
		ctx:    ctx,
		writer: os.Stderr,
		level:  zerolog.InfoLevel,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	w := cfg.writer
	if len(cfg.additionalSinks) > 0 {
		writers := make([]io.Writer, 0, 1+len(cfg.additionalSinks))
		writers = append(writers, cfg.writer)
		for _, s := range cfg.additionalSinks {
			writers = append(writers, s)
		}
		w = io.MultiWriter(writers...)
	}

	base := zerolog.New(w).Level(cfg.level).With()
	base = applyLabels(base, cfg.labels)
	logger := base.Logger().Hook(tracingHook{})

	return zerologLogger{logger: logger, sinks: cfg.additionalSinks}
}

func (l zerologLogger) Debug(ctx context.Context) *zerolog.Event {
	return l.logger.Debug().Ctx(ctx)
}

func (l zerologLogger) Info(ctx context.Context) *zerolog.Event {
	return l.logger.Info().Ctx(ctx)
}

func (l zerologLogger) Warn(ctx context.Context) *zerolog.Event {
	return l.logger.Warn().Ctx(ctx)
}

func (l zerologLogger) Error(ctx context.Context) *zerolog.Event {
	return l.logger.Error().Ctx(ctx)
}

func (l zerologLogger) WithLabels(labels Labels) Logger {
	ctx := l.logger.With()
	ctx = applyLabels(ctx, labels)
	// Carry sinks forward so ax.Flush still drains buffered entries (e.g. the
	// Loki sink) on the derived logger. Loki stream labels are extracted from
	// each emitted log line, so labels added here remain queryable in Loki.
	return zerologLogger{logger: ctx.Logger(), sinks: l.sinks}
}

func (l zerologLogger) Zerolog() *zerolog.Logger {
	logger := l.logger
	return &logger
}

// flush drains each additional sink in order, passing the caller's context so
// sinks can respect cancellation and deadlines (e.g. the Loki writer, which caps
// its own wait). Drain errors are collected and joined.
//
// flush satisfies the unexported flusher interface so ax.Flush can drain
// buffered sinks without modifying the public Logger contract. Exit code
// mapping: sink drain errors are surfaced to ax.Flush callers but do not map
// to a CLI exit code.
func (l zerologLogger) flush(ctx context.Context) error {
	var errs []error
	for _, s := range l.sinks {
		if err := s.drain(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func applyLabels(ctx zerolog.Context, labels Labels) zerolog.Context {
	if labels.Environment != "" {
		ctx = ctx.Str(labelFieldEnvironment, labels.Environment)
	}
	if labels.Application != "" {
		ctx = ctx.Str(labelFieldApplication, labels.Application)
	}
	if labels.Host != "" {
		ctx = ctx.Str(labelFieldHost, labels.Host)
	}
	if labels.Version != "" {
		ctx = ctx.Str(labelFieldVersion, labels.Version)
	}
	return ctx
}

// tracingHook stamps trace_id and span_id onto every emitted log line so log
// output correlates with traces (AGENTS.md: trace_id/span_id on every line when
// a span is active). It runs once per enabled event at Msg time; events filtered
// out by level never construct, so disabled logs skip the hook entirely.
//
// Allocation contract (verified by BenchmarkLogger, the source of truth for the
// ADR-0009 zero/near-zero-allocation claim):
//   - No active span: the IDs are the ZeroTraceID/ZeroSpanID package constants,
//     so the hot path is allocation-free.
//   - Active span: trace.TraceID.String()/SpanID.String() hex-encode each ID
//     into a fresh string — a fixed, bounded per-line cost independent of the
//     number of labels or structured fields on the line.
type tracingHook struct{}

func (tracingHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	ctx := e.GetCtx()
	if ctx == nil {
		ctx = context.Background()
	}
	traceID, spanID := traceIDs(ctx)
	e.Str("trace_id", traceID)
	e.Str("span_id", spanID)
}
