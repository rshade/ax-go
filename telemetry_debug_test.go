package ax

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestExecuteTelemetryDebugWritesSpansToStderr(t *testing.T) {
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
	assertDebugSpanOutput(t, stderr, "app")
	if strings.Contains(string(stdout), "SpanContext") || strings.Contains(string(stdout), `"Name"`) {
		t.Fatalf("stdout contains debug span data: %s", stdout)
	}
}

func TestExecuteTelemetryDebugAbsentIsSilent(t *testing.T) {
	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
	}
	if !bytes.Equal(stdout, []byte("{\"ok\":true}\n")) {
		t.Fatalf("stdout = %s, want normal payload", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestExecuteTelemetryDebugAndOTLPBothReceiveSpans(t *testing.T) {
	receiver := newOTLPTraceReceiver(t)

	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
		"AX_OTEL_DEBUG":               "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": receiver.endpoint(),
	}, defaultTelemetryShutdownTimeout)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
	}
	export := receiver.next(t)
	if len(export.traceIDs) == 0 {
		t.Fatalf("exported trace IDs empty; span names=%v", export.names)
	}
	assertDebugSpanOutput(t, stderr, "app")
	if strings.Contains(string(stdout), export.traceIDs[0]) {
		t.Fatalf("stdout contains exported trace ID %q: %s", export.traceIDs[0], stdout)
	}
}

func TestExecuteTelemetryDebugSharesStderrWithLogger(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	done := make(chan struct{})

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := NewLogger(cmd.Context(), WithLoggerWriter(cmd.ErrOrStderr()))
			go func() {
				defer close(done)
				for i := range 20 {
					logger.Info(cmd.Context()).Int("line", i).Msg("background")
					time.Sleep(time.Millisecond)
				}
			}()
			time.Sleep(2 * time.Millisecond)
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
			if key == "AX_OTEL_DEBUG" {
				return "1"
			}
			return ""
		}),
	)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background logging")
	}

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	assertDebugSpanOutput(t, stderr.String(), "app")
	if stdout.String() != "{\"ok\":true}\n" {
		t.Fatalf("stdout = %s, want normal payload", stdout.String())
	}
}

func assertDebugSpanOutput(t *testing.T, stderr string, spanName string) {
	t.Helper()

	if !strings.Contains(stderr, `"Name"`) {
		t.Fatalf("stderr = %q, want debug span JSON with Name field", stderr)
	}
	if !strings.Contains(stderr, spanName) {
		t.Fatalf("stderr = %q, want debug span name %q", stderr, spanName)
	}
	if !strings.Contains(stderr, "SpanContext") {
		t.Fatalf("stderr = %q, want debug span context", stderr)
	}
}
