package ax

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestExecuteTelemetryRejectsUntrustedTLS guards FR-004: outbound OTLP export
// must verify TLS certificates. An httptest TLS server presents a self-signed
// certificate that is not in the system trust store, so a correct (non-insecure)
// client rejects it and export fails-open with a diagnostic. If TLS verification
// were ever disabled (InsecureSkipVerify / WithInsecure), the export would
// succeed and this test would fail.
func TestExecuteTelemetryRejectsUntrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	baselineStdout, _, baselineCode := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)

	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": server.URL, // https:// with a self-signed cert
	}, 500*time.Millisecond)

	if code != baselineCode {
		t.Fatalf("Execute exit code = %d, want baseline %d (fail-open); stderr=%s", code, baselineCode, stderr)
	}
	if !bytes.Equal(stdout, baselineStdout) {
		t.Fatalf("stdout changed under TLS failure\nbaseline: %s\ngot: %s", baselineStdout, stdout)
	}
	if !strings.Contains(stderr, "ax: otel export failed") {
		t.Fatalf("stderr = %q, want export-failed diagnostic proving TLS verification is enforced", stderr)
	}
}

func TestTelemetryDiagnosticSanitizesControlCharacters(t *testing.T) {
	got := telemetryDiagnostic("bad\nforged: line\ttab\x1b[31mred\x7f")
	if strings.ContainsAny(got, "\n\t\x1b\x7f") {
		t.Fatalf("telemetryDiagnostic left control characters (log-forging risk): %q", got)
	}
	if !strings.Contains(got, "forged: line") {
		t.Fatalf("telemetryDiagnostic dropped legitimate content: %q", got)
	}
}
