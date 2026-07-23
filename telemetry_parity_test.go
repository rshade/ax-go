package ax

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// This file is deliberately untagged. Its assertions are the central promise of
// the ax_no_otlp / ax_no_grpc feature — that declining export degrades tracing
// to *no export*, never to *no tracing* (FR-009, FR-010, FR-021, SC-003).
//
// A parity claim asserted only in the default configuration proves nothing
// about the configurations it is a claim about, so every test here must compile
// and run under all four tag combinations. Nothing in this file may reference a
// symbol whose presence varies by tag.

// TestTelemetryParityExtractsInboundW3CContext asserts inbound W3C trace
// context survives Execute in every configuration: the trace ID continues, the
// root span is a child of the inbound span, and tracestate is carried forward.
func TestTelemetryParityExtractsInboundW3CContext(t *testing.T) {
	const (
		traceID      = "4bf92f3577b34da6a3ce929d0e0e4736"
		parentSpanID = "00f067aa0ba902b7"
		traceState   = "rojo=00f067aa0ba902b7"
	)

	var stdout, stderr bytes.Buffer
	var runTraceID, runTraceState string
	var runParentSpanID string

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			runTraceID = TraceIDFromContext(ctx)
			sc := trace.SpanContextFromContext(ctx)
			runTraceState = sc.TraceState().String()
			if readOnly, ok := trace.SpanFromContext(ctx).(sdktrace.ReadOnlySpan); ok {
				runParentSpanID = readOnly.Parent().SpanID().String()
			}
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithStdoutIsTTY(false),
		WithEnv(func(key string) string {
			switch key {
			case "TRACEPARENT":
				return "00-" + traceID + "-" + parentSpanID + "-01"
			case "TRACESTATE":
				return traceState
			default:
				return ""
			}
		}),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if runTraceID != traceID {
		t.Fatalf("TraceIDFromContext = %q, want inbound trace %q — W3C extraction lost", runTraceID, traceID)
	}
	if runTraceState != traceState {
		t.Fatalf("TraceState = %q, want %q — TRACESTATE not propagated", runTraceState, traceState)
	}
	if runParentSpanID != parentSpanID {
		t.Fatalf("root span parent = %q, want inbound span %q — root span is not a child of the inbound span",
			runParentSpanID, parentSpanID)
	}
}

// TestTelemetryParityRootSpanIsRecording asserts Execute always wraps the
// command in a recording root span with a valid span context. Without a
// recording span there is nothing for log correlation to attach to, so this is
// the assertion that distinguishes "no export" from "no tracing".
func TestTelemetryParityRootSpanIsRecording(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var recording, validSpanContext bool
	var runTraceID, runSpanID string

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			span := trace.SpanFromContext(ctx)
			recording = span.IsRecording()
			validSpanContext = span.SpanContext().IsValid()
			runTraceID = TraceIDFromContext(ctx)
			runSpanID = SpanIDFromContext(ctx)
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithStdoutIsTTY(false),
		WithEnv(func(string) string { return "" }),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if !recording {
		t.Fatal("root span is not recording — tracing degraded to no tracing, not merely no export")
	}
	if !validSpanContext {
		t.Fatal("root span context is not valid")
	}
	if runTraceID == ZeroTraceID {
		t.Fatalf("TraceIDFromContext = %q, want non-zero", runTraceID)
	}
	if runSpanID == ZeroSpanID {
		t.Fatalf("SpanIDFromContext = %q, want non-zero", runSpanID)
	}
}

// TestTelemetryParityLogLinesCarryTraceCorrelation asserts 100% — not merely
// "most" — of the log lines emitted inside the root span carry trace_id and
// span_id matching that span, and that neither leaks to stdout.
func TestTelemetryParityLogLinesCarryTraceCorrelation(t *testing.T) {
	const lineCount = 8

	var stdout, stderr bytes.Buffer
	var runTraceID, runSpanID string

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			runTraceID = TraceIDFromContext(ctx)
			runSpanID = SpanIDFromContext(ctx)
			logger := NewLogger(ctx, WithLoggerWriter(cmd.ErrOrStderr()))
			for i := range lineCount {
				logger.Info(ctx).Int("line", i).Msg("ran")
			}
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithStdoutIsTTY(false),
		WithEnv(func(string) string { return "" }),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}

	records := decodeLogRecords(t, stderr.String())
	if len(records) != lineCount {
		t.Fatalf("log records = %d, want %d; stderr=%s", len(records), lineCount, stderr.String())
	}
	for i, record := range records {
		if record["trace_id"] != runTraceID {
			t.Fatalf("record %d trace_id = %v, want active trace %q", i, record["trace_id"], runTraceID)
		}
		if record["span_id"] != runSpanID {
			t.Fatalf("record %d span_id = %v, want active span %q", i, record["span_id"], runSpanID)
		}
	}
	if strings.Contains(stdout.String(), runTraceID) || strings.Contains(stdout.String(), runSpanID) {
		t.Fatalf("stdout leaked trace correlation IDs: %s", stdout.String())
	}
}

// TestTelemetryParityDebugSpansReachStderr asserts the AX_OTEL_DEBUG local-span
// path stays available in every configuration. It is backed by stdouttrace,
// which links zero gRPC packages, so declining OTLP export must not disturb it
// (FR-011).
func TestTelemetryParityDebugSpansReachStderr(t *testing.T) {
	baselineStdout, _, baselineCode := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)

	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
		"AX_OTEL_DEBUG": "1",
	}, defaultTelemetryShutdownTimeout)

	if code != baselineCode {
		t.Fatalf("Execute exit code = %d, want baseline %d; stderr=%s", code, baselineCode, stderr)
	}
	if !bytes.Equal(stdout, baselineStdout) {
		t.Fatalf("stdout changed with debug telemetry\nbaseline: %s\ngot: %s", baselineStdout, stdout)
	}
	assertDebugSpanOutput(t, stderr)
}
