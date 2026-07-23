//go:build !ax_no_otlp

package ax

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
