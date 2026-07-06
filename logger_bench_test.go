package ax

import (
	"context"
	"io"
	"testing"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// benchActiveSpanContext builds a context carrying a valid (non-zero) W3C span
// context without standing up the OpenTelemetry SDK. The benchmark measures the
// steady-state logging hot path, not telemetry setup, so the cheap
// NewSpanContext path is deliberate: it exercises the tracingHook's
// active-span branch (sc.HasTraceID()/HasSpanID() both true) where the OTel ID
// values are hex-encoded into fresh strings on every log line.
func benchActiveSpanContext(tb testing.TB) context.Context {
	tb.Helper()
	traceID, err := oteltrace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		tb.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := oteltrace.SpanIDFromHex("0102030405060708")
	if err != nil {
		tb.Fatalf("SpanIDFromHex: %v", err)
	}
	return oteltrace.ContextWithSpanContext(
		context.Background(),
		oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
			TraceID: traceID,
			SpanID:  spanID,
		}),
	)
}

// BenchmarkLogger records B/op and allocs/op for the logging hot path
// (logger.Info(ctx).Msg(...)) across the dimensions that determine its
// allocation profile, substantiating or qualifying ADR-0009's "zero/near-zero
// allocation hot path" claim. Every case writes to io.Discard so the numbers
// reflect the logger and its tracingHook, not sink I/O.
//
// The tracingHook (logger.go) runs on every emitted line. Its cost splits on
// span presence: with no active span the trace/span IDs are package constants
// (ZeroTraceID/ZeroSpanID, no allocation); with an active span each ID is
// hex-encoded into a fresh string (allocation per line). The "no_span" vs
// "active_span" delta isolates exactly that hook cost.
func BenchmarkLogger(b *testing.B) {
	bgCtx := context.Background()
	activeCtx := benchActiveSpanContext(b)

	base := NewLogger(bgCtx, WithLoggerWriter(io.Discard))
	labeled := NewLogger(
		bgCtx,
		WithLoggerWriter(io.Discard),
		WithLoggerLabels(Labels{
			Application: "bench",
			Environment: "test",
			Host:        "bench-host",
			Version:     "v1.2.3",
		}),
	)

	plainMsg := func(l Logger, ctx context.Context) {
		l.Info(ctx).Msg("benchmark log line")
	}
	// resource_id is a payload field (never a Loki label) per the cardinality
	// split in AGENTS.md; this case measures the marginal cost of structured
	// fields on top of the always-present trace correlation fields.
	fieldsMsg := func(l Logger, ctx context.Context) {
		l.Info(ctx).
			Str("resource_id", "01890d3e-2b7a-7c9e-9a1e-9f3c0a1b2c3d").
			Int("attempt", 3).
			Msg("benchmark log line")
	}

	cases := []struct {
		name   string
		logger Logger
		ctx    context.Context
		emit   func(Logger, context.Context)
	}{
		{name: "no_span", logger: base, ctx: bgCtx, emit: plainMsg},
		{name: "active_span", logger: base, ctx: activeCtx, emit: plainMsg},
		{name: "active_span_with_labels", logger: labeled, ctx: activeCtx, emit: plainMsg},
		{name: "active_span_with_fields", logger: base, ctx: activeCtx, emit: fieldsMsg},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				tc.emit(tc.logger, tc.ctx)
			}
		})
	}
}

// BenchmarkLoggerDisabledLevel measures a log call that is filtered out by level
// (a Debug call on an Info-level logger). This is the most load-bearing part of
// the "zero-allocation hot path" claim: agents emit Debug liberally, and in
// production those calls must cost ~nothing — including skipping the tracing
// hook entirely so no trace/span ID hex-encoding happens.
//
// Pass benchActiveSpanContext(b) deliberately: if the tracingHook were
// (incorrectly) running on disabled events, the active-span branch would
// hex-encode the IDs and this benchmark would report the same 48 B / 2 allocs
// we see on enabled lines. A clean 0 allocs/op here therefore proves disabled
// logs short-circuit before the hook.
func BenchmarkLoggerDisabledLevel(b *testing.B) {
	// NewLogger defaults to zerolog.InfoLevel, so Debug events are below
	// threshold: l.logger.Debug() returns a disabled (nil) event whose Msg is a
	// no-op, and the hook never runs.
	logger := NewLogger(context.Background(), WithLoggerWriter(io.Discard))
	activeCtx := benchActiveSpanContext(b)

	b.ReportAllocs()
	for b.Loop() {
		logger.Debug(activeCtx).Msg("benchmark log line")
	}
}

// benchDiscardLogger builds an InfoLevel logger that writes to io.Discard so the
// measured profile reflects logger allocations, not OS write cost or console
// formatting (FR-007, research Decision 3). Extra options layer on top of the
// discard writer.
func benchDiscardLogger(opts ...LoggerOption) Logger {
	return NewLogger(context.Background(), append([]LoggerOption{WithLoggerWriter(io.Discard)}, opts...)...)
}

// BenchmarkLoggerEmit substantiates the enabled-level emit versus filtered
// fast-path allocation profile. The enabled/no_fields sub-case measures the
// common "log one line" path; disabled_level measures zerolog's early-return
// path for a level below the configured threshold, reported separately so the
// cheaper filtered path is not blended into the emitted-line number (FR-003,
// FR-005, SC-001, SC-002; FR-011 doc convention).
func BenchmarkLoggerEmit(b *testing.B) {
	logger := benchDiscardLogger()
	ctx := context.Background()

	b.Run("enabled/no_fields", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})

	b.Run("disabled_level", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			logger.Debug(ctx).Msg("benchmark log line")
		}
	})
}

// BenchmarkLoggerTracingHook isolates the always-on tracing hook's
// context-dependent allocation cost. The no_trace_context sub-case takes the
// zero-ID constant path; active_trace_context carries a populated SpanContext
// and so formats hex trace/span IDs, which allocate. Reporting them separately
// keeps the hook's marginal cost visible (FR-004, SC-002; research Decision 2).
func BenchmarkLoggerTracingHook(b *testing.B) {
	logger := benchDiscardLogger()

	b.Run("no_trace_context", func(b *testing.B) {
		ctx := context.Background()
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})

	b.Run("active_trace_context", func(b *testing.B) {
		ctx := activeTraceContext(b)
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})
}

// BenchmarkLoggerFieldShapes captures the representative field-shape and
// labeled-logger profiles so the suite reflects realistic usage, not just the
// emptiest call. The typed_fields sub-case attaches typed payload fields to a
// plain logger; with_labels emits on a logger constructed with low-cardinality
// labels (FR-006).
func BenchmarkLoggerFieldShapes(b *testing.B) {
	ctx := context.Background()

	b.Run("typed_fields", func(b *testing.B) {
		logger := benchDiscardLogger()
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Str("k", "v").Int("n", 1).Msg("benchmark log line")
		}
	})

	b.Run("with_labels", func(b *testing.B) {
		logger := benchDiscardLogger(WithLoggerLabels(Labels{Application: "app", Environment: "test"}))
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})
}
