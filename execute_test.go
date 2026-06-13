package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestExecuteLogLinesCarryRootSpanContextWithoutCollector(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var runTraceID string
	var runSpanID string

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runTraceID = TraceIDFromContext(cmd.Context())
			runSpanID = SpanIDFromContext(cmd.Context())
			logger := NewLogger(cmd.Context(), WithLoggerWriter(cmd.ErrOrStderr()))
			logger.Info(cmd.Context()).Str("event", "first").Msg("ran")
			logger.Info(cmd.Context()).Str("event", "second").Msg("ran")
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
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if runTraceID == ZeroTraceID {
		t.Fatalf("TraceIDFromContext during run = %q, want non-zero", runTraceID)
	}
	if runSpanID == ZeroSpanID {
		t.Fatalf("SpanIDFromContext during run = %q, want non-zero", runSpanID)
	}

	records := decodeLogRecords(t, stderr.String())
	if len(records) != 2 {
		t.Fatalf("log records = %d, want 2; stderr=%s", len(records), stderr.String())
	}
	for _, record := range records {
		if record["trace_id"] != runTraceID {
			t.Fatalf("log trace_id = %v, want active trace %q", record["trace_id"], runTraceID)
		}
		if record["span_id"] != runSpanID {
			t.Fatalf("log span_id = %v, want active span %q", record["span_id"], runSpanID)
		}
	}
	if strings.Contains(stdout.String(), runTraceID) {
		t.Fatalf("stdout contains trace_id %q: %s", runTraceID, stdout.String())
	}
	if strings.Contains(stdout.String(), runSpanID) {
		t.Fatalf("stdout contains span_id %q: %s", runSpanID, stdout.String())
	}
}

func TestExecuteContinuesInboundTraceparent(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var runTraceID string

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runTraceID = TraceIDFromContext(cmd.Context())
			logger := NewLogger(cmd.Context(), WithLoggerWriter(cmd.ErrOrStderr()))
			logger.Info(cmd.Context()).Msg("ran")
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
		WithEnv(func(key string) string {
			if key == "TRACEPARENT" {
				return "00-" + traceID + "-00f067aa0ba902b7-01"
			}
			return ""
		}),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if runTraceID != traceID {
		t.Fatalf("TraceIDFromContext during run = %q, want inbound trace %q", runTraceID, traceID)
	}
	records := decodeLogRecords(t, stderr.String())
	if len(records) != 1 {
		t.Fatalf("log records = %d, want 1; stderr=%s", len(records), stderr.String())
	}
	if records[0]["trace_id"] != traceID {
		t.Fatalf("log trace_id = %v, want inbound trace %q", records[0]["trace_id"], traceID)
	}
	if strings.Contains(stdout.String(), traceID) {
		t.Fatalf("stdout contains trace_id %q: %s", traceID, stdout.String())
	}
}

func TestExecuteTelemetryFailOpen(t *testing.T) {
	baselineStdout, _, baselineCode := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)

	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "malformed endpoint",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "://bad-endpoint",
			},
		},
		{
			name: "unreachable collector",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:1",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := executeTelemetryCommand(t, tc.env, 100*time.Millisecond)

			if code != baselineCode {
				t.Fatalf("Execute exit code = %d, want baseline %d; stderr=%s", code, baselineCode, stderr)
			}
			if !bytes.Equal(stdout, baselineStdout) {
				t.Fatalf("stdout changed under telemetry failure\nbaseline: %s\ngot: %s", baselineStdout, stdout)
			}
			if !strings.Contains(stderr, "ax: otel") {
				t.Fatalf("stderr = %q, want telemetry diagnostic", stderr)
			}
		})
	}
}

func TestExecuteTelemetryStuckCollectorHonorsShutdownBudget(t *testing.T) {
	baselineStdout, _, _ := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	start := time.Now()
	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": server.URL,
	}, 100*time.Millisecond)
	elapsed := time.Since(start)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
	}
	if elapsed > time.Second {
		t.Fatalf("Execute took %s with stuck collector, want under 1s", elapsed)
	}
	if !bytes.Equal(stdout, baselineStdout) {
		t.Fatalf("stdout = %s, want normal payload", stdout)
	}
	if !strings.Contains(stderr, "ax: otel") {
		t.Fatalf("stderr = %q, want telemetry diagnostic", stderr)
	}
}

func TestExecuteTelemetryStreamSeparationAcrossModes(t *testing.T) {
	baselineStdout, _, baselineCode := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)

	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)
	assertTelemetryStdoutUnchanged(t, "no-op", stdout, baselineStdout, code, baselineCode, stderr)

	receiver := newOTLPTraceReceiver(t)
	stdout, stderr, code = executeTelemetryCommand(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": receiver.endpoint(),
	}, defaultTelemetryShutdownTimeout)
	assertTelemetryStdoutUnchanged(t, "otlp", stdout, baselineStdout, code, baselineCode, stderr)
	export := receiver.next(t)
	for _, traceID := range export.traceIDs {
		if bytes.Contains(stdout, []byte(traceID)) {
			t.Fatalf("stdout contains OTLP trace ID %q: %s", traceID, stdout)
		}
	}

	stdout, stderr, code = executeTelemetryCommand(t, map[string]string{
		"AX_OTEL_DEBUG": "1",
	}, defaultTelemetryShutdownTimeout)
	assertTelemetryStdoutUnchanged(t, "debug", stdout, baselineStdout, code, baselineCode, stderr)
	if strings.Contains(string(stdout), "SpanContext") || strings.Contains(string(stdout), `"Name"`) {
		t.Fatalf("stdout contains debug telemetry bytes: %s", stdout)
	}
}

func TestExecuteInjectsAXContext(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, ok := ModeFromContext(cmd.Context())
			if !ok {
				t.Fatal("mode missing from context")
			}
			key, ok := IdempotencyKeyFromContext(cmd.Context())
			if !ok {
				t.Fatal("idempotency key missing from context")
			}
			return WriteJSON(cmd.OutOrStdout(), map[string]any{
				"mode":    mode,
				"dry_run": DryRunFromContext(cmd.Context()),
				"key":     key,
			})
		},
	}
	root.SetArgs([]string{"--format=json", "--dry-run", "--idempotency-key=abc"})

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithStdoutIsTTY(true),
		WithEnv(func(string) string { return "" }),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not JSON: %v", err)
	}
	if got["mode"] != string(ModeJSON) {
		t.Fatalf("mode = %v, want %q", got["mode"], ModeJSON)
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true", got["dry_run"])
	}
	if got["key"] != "abc" {
		t.Fatalf("key = %v, want abc", got["key"])
	}
}

func decodeLogRecords(t *testing.T, logs string) []map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(logs), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line was not JSON: %v; line=%q", err, line)
		}
		records = append(records, record)
	}
	return records
}

func assertTelemetryStdoutUnchanged(
	t *testing.T,
	name string,
	stdout []byte,
	baselineStdout []byte,
	code int,
	baselineCode int,
	stderr string,
) {
	t.Helper()

	if code != baselineCode {
		t.Fatalf("%s exit code = %d, want baseline %d; stderr=%s", name, code, baselineCode, stderr)
	}
	if !bytes.Equal(stdout, baselineStdout) {
		t.Fatalf("%s stdout changed\nbaseline: %s\ngot: %s", name, baselineStdout, stdout)
	}
}

func TestExecuteWritesErrorsToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return NewError(cmd.Context(), "validation_error", "bad input", WithErrorExitCode(ExitValidation))
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithVersion("v0.1.0"),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitValidation {
		t.Fatalf("Execute exit code = %d, want %d", code, ExitValidation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var got map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("stderr was not JSON: %v", err)
	}
	if got["error_code"] != "validation_error" {
		t.Fatalf("error_code = %v, want validation_error", got["error_code"])
	}
	if got["tool"] != "app" {
		t.Fatalf("tool = %v, want app", got["tool"])
	}
	if got["version"] != "v0.1.0" {
		t.Fatalf("version = %v, want v0.1.0", got["version"])
	}
}

func TestExecuteSchemaCommandWritesStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app __schema",
	}
	root.SetArgs([]string{"__schema"})

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithVersion("v0.1.0"),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got Schema
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not schema JSON: %v", err)
	}
	if got.Tool != "app" {
		t.Fatalf("Tool = %q, want app", got.Tool)
	}
}
