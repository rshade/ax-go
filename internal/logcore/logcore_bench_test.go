package logcore

import (
	"context"
	"io"
	"testing"
)

// Benchmark naming is load-bearing, twice over.
//
// benchcheck detects a benchmark present in the base ref but absent from the
// current run and treats it as a hard failure, keyed on the BARE benchmark name
// with the package group ignored. So (a) the root package's BenchmarkLogger*
// set must not be renamed, moved, or deleted by this extraction — it still
// exercises this code through the delegation, and it is what SC-006's tracked
// hot path is measured against — and (b) every benchmark here carries a
// distinct Logcore-prefixed name. A duplicate name across two packages would
// make the comparison ambiguous rather than loud, and an ambiguous comparison
// hides regressions instead of reporting them.

// benchDiscardLogger builds an info-level logger writing to io.Discard so the
// measured profile reflects the logger and its tracing hook, not sink I/O.
func benchDiscardLogger(opts ...Option) Logger {
	return New(context.Background(), append([]Option{WithWriter(io.Discard)}, opts...)...)
}

// BenchmarkLogcoreEmit measures the enabled emit path against the level-filtered
// fast path. Reporting them separately keeps the cheaper filtered path from
// being blended into the emitted-line number.
func BenchmarkLogcoreEmit(b *testing.B) {
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

// BenchmarkLogcoreTracingHook isolates the always-on hook's context-dependent
// cost. no_trace_context is the C-05 allocation contract: with no active span the
// IDs are the contract package's zero-value constants, so the path allocates
// nothing. active_trace_context hex-encodes each ID into a fresh string, a fixed
// per-line cost independent of how many fields the line carries.
func BenchmarkLogcoreTracingHook(b *testing.B) {
	logger := benchDiscardLogger()

	b.Run("no_trace_context", func(b *testing.B) {
		ctx := context.Background()
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})

	b.Run("active_trace_context", func(b *testing.B) {
		ctx := activeSpanContext(b)
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})
}

// BenchmarkLogcoreFieldShapes captures the representative field-shape and
// labeled-logger profiles so the suite reflects realistic usage rather than only
// the emptiest call.
func BenchmarkLogcoreFieldShapes(b *testing.B) {
	ctx := context.Background()

	b.Run("typed_fields", func(b *testing.B) {
		logger := benchDiscardLogger()
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Str("k", "v").Int("n", 1).Msg("benchmark log line")
		}
	})

	b.Run("with_labels", func(b *testing.B) {
		logger := benchDiscardLogger(WithLabels(Labels{Application: "app", Environment: "test"}))
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})
}

// BenchmarkLogcoreFanOut measures the marginal cost of the io.MultiWriter
// fan-out that appears once an additional sink is registered, which is the path
// every Loki-enabled consumer takes.
func BenchmarkLogcoreFanOut(b *testing.B) {
	ctx := context.Background()

	b.Run("no_sinks", func(b *testing.B) {
		logger := benchDiscardLogger()
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})

	b.Run("one_sink", func(b *testing.B) {
		logger := benchDiscardLogger(withSinks(discardSink{}))
		b.ReportAllocs()
		for b.Loop() {
			logger.Info(ctx).Msg("benchmark log line")
		}
	})
}

// discardSink is an allocation-free Sink for the fan-out benchmark: it must not
// contribute measurements of its own to the number being reported.
type discardSink struct{}

func (discardSink) Write(p []byte) (int, error) { return len(p), nil }
func (discardSink) Drain(context.Context) error { return nil }
