package ax

import (
	"context"
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

// Logger is the ADR-0009 logging surface, initially backed by zerolog.
type Logger interface {
	Debug(ctx context.Context) *zerolog.Event
	Info(ctx context.Context) *zerolog.Event
	Warn(ctx context.Context) *zerolog.Event
	Error(ctx context.Context) *zerolog.Event
	WithLabels(labels Labels) Logger
	Zerolog() *zerolog.Logger
}

type zerologLogger struct {
	logger zerolog.Logger
}

// LoggerOption configures NewLogger.
type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	writer io.Writer
	level  zerolog.Level
	labels Labels
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
func NewLogger(ctx context.Context, opts ...LoggerOption) Logger {
	cfg := loggerConfig{
		writer: os.Stderr,
		level:  zerolog.InfoLevel,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	base := zerolog.New(cfg.writer).Level(cfg.level).With()
	base = applyLabels(base, cfg.labels)
	logger := base.Logger().Hook(tracingHook{})
	_ = ctx

	return zerologLogger{logger: logger}
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
	return zerologLogger{logger: ctx.Logger()}
}

func (l zerologLogger) Zerolog() *zerolog.Logger {
	logger := l.logger
	return &logger
}

func applyLabels(ctx zerolog.Context, labels Labels) zerolog.Context {
	if labels.Environment != "" {
		ctx = ctx.Str("environment", labels.Environment)
	}
	if labels.Application != "" {
		ctx = ctx.Str("application", labels.Application)
	}
	if labels.Host != "" {
		ctx = ctx.Str("host", labels.Host)
	}
	if labels.Version != "" {
		ctx = ctx.Str("version", labels.Version)
	}
	return ctx
}

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
