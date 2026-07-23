//go:build ax_no_grpc && !ax_no_otlp

package ax

import (
	"strings"
	"testing"
)

// This file compiles only in the no-grpc configuration: ax_no_grpc set,
// ax_no_otlp absent. Its sibling buildtags_single_otlp_test.go covers the
// mirror-image configuration.
//
// Together they are the proof that the two constraints are genuinely
// independent (FR-002). A single tag must decline exactly one capability and
// leave the other fully intact — if declining the gRPC helper also broke export,
// the two knobs would be coupled and the four-configuration contract would be a
// fiction.

// TestNoGRPCConfigurationKeepsExportReachable asserts that declining the gRPC
// dial helper leaves the OTLP export path fully operational. The export path is
// exercised end to end against a live receiver: reaching it at all requires the
// otlptracehttp exporter, which ax_no_grpc must not have removed.
func TestNoGRPCConfigurationKeepsExportReachable(t *testing.T) {
	receiver := newOTLPTraceReceiver(t)

	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": receiver.endpoint(),
	}, defaultTelemetryShutdownTimeout)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
	}
	if strings.Contains(stderr, "exporter disabled") {
		t.Fatalf("stderr reports the exporter disabled under ax_no_grpc alone: %q", stderr)
	}

	export := receiver.next(t)
	if len(export.traceIDs) == 0 {
		t.Fatalf("no spans exported under ax_no_grpc; span names=%v", export.names)
	}
	if string(stdout) != "{\"ok\":true}\n" {
		t.Fatalf("stdout = %q, want the payload alone", stdout)
	}
}

// TestNoGRPCConfigurationStillTraces asserts the surviving half of tracing is
// untouched when only the dial helper is declined.
func TestNoGRPCConfigurationStillTraces(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	_, stderr, code := executeTelemetryCommand(t, map[string]string{
		"TRACEPARENT":   "00-" + traceID + "-00f067aa0ba902b7-01",
		"AX_OTEL_DEBUG": "1",
	}, defaultTelemetryShutdownTimeout)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
	}
	if !strings.Contains(stderr, traceID) {
		t.Fatalf("debug span output does not carry the inbound trace %q: %q", traceID, stderr)
	}
}
