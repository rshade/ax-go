//go:build !ax_no_grpc && ax_no_otlp

package ax

import (
	"context"
	"strings"
	"testing"
)

// This file compiles only in the no-otlp configuration: ax_no_otlp set,
// ax_no_grpc absent. It is the mirror of buildtags_single_test.go and completes
// the independence proof for FR-002.

// TestNoOTLPConfigurationKeepsGRPCDial asserts ax.GRPCDial is still present and
// functional when only export is declined. Referencing the symbol at all is the
// core of the assertion: under ax_no_grpc this file would not compile.
func TestNoOTLPConfigurationKeepsGRPCDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := GRPCDial(ctx, "localhost:9999"); err == nil {
		t.Fatal("GRPCDial with a cancelled context should return an error")
	}
}

// TestNoOTLPConfigurationDeclinesExportFailOpen asserts a configured endpoint is
// a diagnostic rather than a failure, and that the command still succeeds with
// an unchanged payload (FR-008, C3).
func TestNoOTLPConfigurationDeclinesExportFailOpen(t *testing.T) {
	baselineStdout, _, baselineCode := executeTelemetryCommand(t, map[string]string{}, defaultTelemetryShutdownTimeout)

	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:4318",
	}, defaultTelemetryShutdownTimeout)

	if code != baselineCode {
		t.Fatalf("Execute exit code = %d, want baseline %d; declining export must be fail-open", code, baselineCode)
	}
	if string(stdout) != string(baselineStdout) {
		t.Fatalf("stdout changed under the export decline\nbaseline: %s\ngot: %s", baselineStdout, stdout)
	}
	if !strings.Contains(stderr, "ax: otel exporter disabled") {
		t.Fatalf("stderr = %q, want the exporter-disabled diagnostic", stderr)
	}
}

// TestNoOTLPConfigurationKeepsDebugSpans asserts AX_OTEL_DEBUG local span output
// still works with export declined (FR-011). stdouttrace links zero gRPC
// packages, so there is no reason for the export decline to disturb it — this
// test is what keeps that true.
func TestNoOTLPConfigurationKeepsDebugSpans(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	_, stderr, code := executeTelemetryCommand(t, map[string]string{
		"AX_OTEL_DEBUG": "1",
		"TRACEPARENT":   "00-" + traceID + "-00f067aa0ba902b7-01",
	}, defaultTelemetryShutdownTimeout)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
	}
	assertDebugSpanOutput(t, stderr)
	if !strings.Contains(stderr, traceID) {
		t.Fatalf("debug span output does not carry the inbound trace %q: %q", traceID, stderr)
	}
}
