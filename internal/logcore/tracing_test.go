package logcore

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/rs/zerolog"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/rshade/ax-go/contract"
)

const (
	fieldTraceID = "trace_id"
	fieldSpanID  = "span_id"

	testTraceIDHex = "0102030405060708090a0b0c0d0e0f10"
	testSpanIDHex  = "0102030405060708"
)

// activeSpanContext builds a context carrying a valid, non-zero W3C span context
// without standing up the OpenTelemetry SDK. logcore reads an existing span
// context and never generates IDs, which is why the trace API alone suffices —
// and why this package can stay isolated from the SDK entirely.
func activeSpanContext(tb testing.TB) context.Context {
	tb.Helper()

	traceID, err := oteltrace.TraceIDFromHex(testTraceIDHex)
	if err != nil {
		tb.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := oteltrace.SpanIDFromHex(testSpanIDHex)
	if err != nil {
		tb.Fatalf("SpanIDFromHex: %v", err)
	}
	return oteltrace.ContextWithSpanContext(
		context.Background(),
		oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: traceID, SpanID: spanID}),
	)
}

// TestTracingHookStampsSpanIdentifiers covers C-04 and C-05: every enabled event
// carries trace_id and span_id, taking the real hex values under an active span
// and the zero-value valid hex constants without one. ADR-0004's consequence is
// binding here — the fields are always present, so a consumer parser never has
// to branch on absence.
func TestTracingHookStampsSpanIdentifiers(t *testing.T) {
	cases := []struct {
		name        string
		ctx         func(testing.TB) context.Context
		wantTraceID string
		wantSpanID  string
	}{
		{
			name:        "active_span",
			ctx:         activeSpanContext,
			wantTraceID: testTraceIDHex,
			wantSpanID:  testSpanIDHex,
		},
		{
			name:        "no_active_span",
			ctx:         func(testing.TB) context.Context { return context.Background() },
			wantTraceID: contract.ZeroTraceID,
			wantSpanID:  contract.ZeroSpanID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(context.Background(), WithWriter(&buf))
			logger.Info(tc.ctx(t)).Msg("traced")

			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("log line was not JSON: %v", err)
			}
			if got[fieldTraceID] != tc.wantTraceID {
				t.Errorf("%s = %v, want %q", fieldTraceID, got[fieldTraceID], tc.wantTraceID)
			}
			if got[fieldSpanID] != tc.wantSpanID {
				t.Errorf("%s = %v, want %q", fieldSpanID, got[fieldSpanID], tc.wantSpanID)
			}
		})
	}
}

// TestTracingHookRunsOnEveryEnabledLevel covers the "every enabled event" half of
// C-04: the hook is attached to the logger, not to one level's path.
func TestTracingHookRunsOnEveryEnabledLevel(t *testing.T) {
	emitters := map[string]func(Logger, context.Context){
		"debug": func(l Logger, ctx context.Context) { l.Debug(ctx).Msg("x") },
		"info":  func(l Logger, ctx context.Context) { l.Info(ctx).Msg("x") },
		"warn":  func(l Logger, ctx context.Context) { l.Warn(ctx).Msg("x") },
		"error": func(l Logger, ctx context.Context) { l.Error(ctx).Msg("x") },
	}

	for name, emit := range emitters {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(context.Background(), WithWriter(&buf), WithLevel(zerolog.DebugLevel))
			emit(logger, activeSpanContext(t))

			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("log line was not JSON: %v", err)
			}
			if got[fieldTraceID] != testTraceIDHex {
				t.Fatalf("%s = %v, want %q", fieldTraceID, got[fieldTraceID], testTraceIDHex)
			}
		})
	}
}

// TestTracingHookSkipsFilteredEvents covers C-06: an event below the configured
// level never constructs, so the hook never runs for it. Asserted through the
// observable consequence — no output at all — because a filtered zerolog event is
// a nil event whose Msg is a no-op.
func TestTracingHookSkipsFilteredEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := New(context.Background(), WithWriter(&buf), WithLevel(zerolog.InfoLevel))

	logger.Debug(activeSpanContext(t)).Msg("filtered")

	if buf.Len() != 0 {
		t.Fatalf("filtered event produced output %q, want none", buf.String())
	}
}

// TestNoActiveSpanPathIsAllocationFree covers the C-05 allocation contract as a
// test rather than only as a benchmark, so a regression fails the ordinary
// `go test` run instead of waiting for bench-check. The threshold is the
// emission path as a whole: with no active span the IDs are package constants,
// so nothing on this path should escape to the heap.
func TestNoActiveSpanPathIsAllocationFree(t *testing.T) {
	logger := New(context.Background(), WithWriter(io.Discard))
	ctx := context.Background()

	avg := testing.AllocsPerRun(100, func() {
		logger.Info(ctx).Msg("allocation contract")
	})

	if avg != 0 {
		t.Fatalf("no-active-span emission allocated %v times per run, want 0 (C-05)", avg)
	}
}

// TestTracingHookHandlesMissingEventContext asserts the hook falls back to a
// background context when zerolog hands it an event with no attached context,
// which happens whenever a caller uses the Zerolog() escape hatch directly.
func TestTracingHookHandlesMissingEventContext(t *testing.T) {
	var buf bytes.Buffer
	logger := New(context.Background(), WithWriter(&buf))

	logger.Zerolog().Info().Msg("no event context")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("log line was not JSON: %v", err)
	}
	if got[fieldTraceID] != contract.ZeroTraceID {
		t.Fatalf("%s = %v, want %q", fieldTraceID, got[fieldTraceID], contract.ZeroTraceID)
	}
}

// TestTraceIDsFallBackToZeroConstants exercises the helper directly across the
// three span-context shapes, so the fallback is pinned independently of the
// zerolog plumbing above.
func TestTraceIDsFallBackToZeroConstants(t *testing.T) {
	cases := []struct {
		name        string
		ctx         context.Context
		wantTraceID string
		wantSpanID  string
	}{
		{
			name:        "no_span_context",
			ctx:         context.Background(),
			wantTraceID: contract.ZeroTraceID,
			wantSpanID:  contract.ZeroSpanID,
		},
		{
			name:        "active_span_context",
			ctx:         activeSpanContext(t),
			wantTraceID: testTraceIDHex,
			wantSpanID:  testSpanIDHex,
		},
		{
			name: "trace_id_without_span_id",
			ctx: func() context.Context {
				traceID, err := oteltrace.TraceIDFromHex(testTraceIDHex)
				if err != nil {
					t.Fatalf("TraceIDFromHex: %v", err)
				}
				return oteltrace.ContextWithSpanContext(
					context.Background(),
					oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: traceID}),
				)
			}(),
			wantTraceID: testTraceIDHex,
			wantSpanID:  contract.ZeroSpanID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotTrace, gotSpan := traceIDs(tc.ctx)
			if gotTrace != tc.wantTraceID {
				t.Errorf("traceID = %q, want %q", gotTrace, tc.wantTraceID)
			}
			if gotSpan != tc.wantSpanID {
				t.Errorf("spanID = %q, want %q", gotSpan, tc.wantSpanID)
			}
		})
	}
}
