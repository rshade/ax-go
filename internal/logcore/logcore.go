package logcore

import (
	"context"
	"io"
	"os"

	"github.com/rs/zerolog"
)

// Labels are low-cardinality descriptors attached to every log line and eligible
// for promotion to log-aggregation stream labels by a sink that implements
// LabelSanctioner.
//
// The set is closed by design (Constitution Principle VIII's cardinality split):
// environment, application, host, and version are indexed labels; trace_id,
// span_id, user_id, durations, and resource IDs are payload and must never be
// promoted. Adding a field here is a public-surface change, because both public
// logging surfaces alias this type.
type Labels struct {
	Environment string
	Application string
	Host        string
	Version     string
}

// labelField* are the zerolog field names written by applyLabels. They are
// defined alongside Labels so that any change to a label name is made in one
// place; loki.go in package ax consumes the same values as its stream label keys.
const (
	labelFieldEnvironment = "environment"
	labelFieldApplication = "application"
	labelFieldHost        = "host"
	labelFieldVersion     = "version"
)

// Logger is the canonical structured-logging surface, backed by zerolog. The
// single-backend guardrail (this interface is a migration seam, not a
// pluggable-backend selector) and the trace-correlation contract are governed by
// Constitution Principles VI and VIII.
//
// Every emitted line carries trace_id and span_id: the active span's hex values
// when one is present, and the zero-value valid hex constants when none is, so a
// consumer parser never has to branch on absence.
type Logger interface {
	Debug(ctx context.Context) *zerolog.Event
	Info(ctx context.Context) *zerolog.Event
	Warn(ctx context.Context) *zerolog.Event
	Error(ctx context.Context) *zerolog.Event
	WithLabels(labels Labels) Logger
	Zerolog() *zerolog.Logger
}

// Config is the accumulated construction state an Option mutates.
//
// Its fields are exported rather than hidden behind accessors because the Loki
// direct-push addon lives in package ax and must register its sink across the
// package boundary. In particular the addon takes the ADDRESS of Writer, so a
// WithWriter applied after the sink is registered is still observed by the
// sink's diagnostic path — that aliasing is what makes option order irrelevant.
//
// Config is named in Option's signature (so godoc for logging.LoggerOption
// mentions this internal type), but surfacecheck inventories aliases by target
// name rather than expanded signature and therefore does not record Config or
// its fields. Field-set discipline is review and convention, not baseline
// drift; adding a field here does not require a surface-check regeneration.
type Config struct {
	Ctx             context.Context //nolint:containedctx // carried to bound sink goroutines to the logger lifetime
	Writer          io.Writer
	Level           zerolog.Level
	Labels          Labels
	AdditionalSinks []Sink
}

// Option configures New. Both public surfaces alias this type, so an option
// manufactured by one surface is accepted by the other's constructor.
type Option func(*Config)

// WithWriter sets the primary logger output writer. Defaults to stderr, which
// keeps log output on the diagnostic stream (Constitution Principle I).
func WithWriter(w io.Writer) Option {
	return func(cfg *Config) {
		cfg.Writer = w
	}
}

// WithLevel sets the minimum zerolog level. Events below it never construct, so
// a filtered call costs nothing beyond the level comparison.
func WithLevel(level zerolog.Level) Option {
	return func(cfg *Config) {
		cfg.Level = level
	}
}

// WithLabels attaches low-cardinality labels to every log line. Empty fields are
// omitted entirely rather than emitted as empty strings.
func WithLabels(labels Labels) Option {
	return func(cfg *Config) {
		cfg.Labels = labels
	}
}

// New returns a Logger backed by zerolog and wired for trace correlation. It
// never returns nil.
//
// The construction order is contractual. Every Option is applied before any sink
// is sanctioned, so stream-label promotion follows the FINAL label set regardless
// of the order the caller passed its options in. When additional sinks are
// registered, each log line is fanned out to all of them alongside the primary
// writer via io.MultiWriter.
//
// A sink that implements LabelSanctioner is told the label set; one that does not
// is left alone and never rejected, keeping the sink seam generic for
// destinations with no label concept.
func New(ctx context.Context, opts ...Option) Logger {
	cfg := Config{
		Ctx:    ctx,
		Writer: os.Stderr,
		Level:  zerolog.InfoLevel,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}

	sanctionLabels(cfg.AdditionalSinks, cfg.Labels)

	w := cfg.Writer
	if len(cfg.AdditionalSinks) > 0 {
		writers := make([]io.Writer, 0, 1+len(cfg.AdditionalSinks))
		writers = append(writers, cfg.Writer)
		for _, s := range cfg.AdditionalSinks {
			writers = append(writers, s)
		}
		w = io.MultiWriter(writers...)
	}

	base := zerolog.New(w).Level(cfg.Level).With()
	base = applyLabels(base, cfg.Labels)
	logger := base.Logger().Hook(tracingHook{})

	return zerologLogger{logger: logger, sinks: cfg.AdditionalSinks}
}

// sanctionLabels tells every sink that implements LabelSanctioner which label set
// is in force. The capability is asserted rather than required so the Sink seam
// stays fully generic: a file rotator or ring buffer has no label concept and
// must not be forced to grow one.
func sanctionLabels(sinks []Sink, labels Labels) {
	for _, s := range sinks {
		if ls, ok := s.(LabelSanctioner); ok {
			ls.SanctionLabels(labels)
		}
	}
}

type zerologLogger struct {
	logger zerolog.Logger
	sinks  []Sink // mirrors Config.AdditionalSinks for flush access
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
	// Carry sinks forward so Flush still drains buffered entries on the derived
	// logger, and re-sanction the new label pairs so they — and only they — are
	// promoted from each emitted line into stream labels; payload fields that
	// reuse a label key name stay payload-only.
	sanctionLabels(l.sinks, labels)
	return zerologLogger{logger: ctx.Logger(), sinks: l.sinks}
}

func (l zerologLogger) Zerolog() *zerolog.Logger {
	logger := l.logger
	return &logger
}
