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

// TestExecuteTelemetryRejectsUntrustedTLS guards FR-004: outbound OTLP export
// must verify TLS certificates. An httptest TLS server presents a self-signed
// certificate that is not in the system trust store, so a correct (non-insecure)
// client rejects it and export fails-open with a diagnostic. If TLS verification
// were ever disabled (InsecureSkipVerify / WithInsecure), the export would
// succeed and this test would fail.
//
// Gated to !ax_no_otlp because its assertion is about the export attempt
// itself: a build that declined export never dials the collector, so
// "ax: otel export failed" is unreachable there. Nothing is lost by the gate —
// with no export path there is no TLS decision left to get wrong.
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
