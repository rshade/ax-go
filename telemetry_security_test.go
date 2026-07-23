package ax

import (
	"strings"
	"testing"

	internaltelemetry "github.com/rshade/ax-go/internal/telemetry"
)

// This file is untagged. Diagnostic sanitisation is a log-forging defence that
// applies to every diagnostic ax writes to stderr, including the
// "otel exporter disabled" line the ax_no_otlp build emits — so the assertion
// must hold in every configuration. The export-specific TLS assertion lives in
// telemetry_security_otlp_test.go, which is gated to builds that can export.
func TestTelemetryDiagnosticSanitizesControlCharacters(t *testing.T) {
	got := internaltelemetry.SanitizeDiagnostic("bad\nforged: line\ttab\x1b[31mred\x7f")
	if strings.ContainsAny(got, "\n\t\x1b\x7f") {
		t.Fatalf("SanitizeDiagnostic left control characters (log-forging risk): %q", got)
	}
	if !strings.Contains(got, "forged: line") {
		t.Fatalf("SanitizeDiagnostic dropped legitimate content: %q", got)
	}
}
