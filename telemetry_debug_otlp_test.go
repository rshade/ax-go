//go:build !ax_no_otlp

package ax

import (
	"strings"
	"testing"
)

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
	assertDebugSpanOutput(t, stderr)
	if strings.Contains(string(stdout), export.traceIDs[0]) {
		t.Fatalf("stdout contains exported trace ID %q: %s", export.traceIDs[0], stdout)
	}
}
