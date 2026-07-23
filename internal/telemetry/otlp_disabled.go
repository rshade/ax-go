//go:build ax_no_otlp

// This build declines OTLP trace export.
//
// The ax_no_otlp build constraint replaces the real exporter construction in
// otlp.go with the stub below, which is what removes otlptracehttp — and with
// it the gRPC, protobuf, OTLP-proto, and grpc-gateway dependency trees — from
// the link graph. Combined with ax_no_grpc it is worth roughly 63% of a
// root-facade binary's stripped size; on its own it is worth far less, because
// ax.GRPCDial keeps the same subtree reachable through a second root.
//
// Tracing is not disabled here, only its network export. W3C trace-context
// extraction, the recording root span around Execute, trace_id/span_id log
// correlation, and the AX_OTEL_DEBUG local span output all behave exactly as
// they do in the default build.
//
// To restore export, drop ax_no_otlp from the build tags. Nothing else changes:
// no source edit, no import change, no API difference.
//
// A configured OTEL_EXPORTER_OTLP_ENDPOINT is not an error in this build. Start
// stays fail-open — it emits one "ax: otel exporter disabled" diagnostic on
// stderr per telemetry start and returns a usable recording provider. Watch for
// that line if you expected spans to reach a collector.
//
// This file declares no exported symbol and references no type from the
// declined dependency trees, which is what keeps the public surface identical
// and the trees genuinely unlinked.

package telemetry

import (
	"context"
	"errors"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// errOTLPExportDeclined is the sentinel Start reports when an endpoint is
// configured in a build that declined export. It flows through the same
// writeDiagnostic path a construction failure would, so the observable
// behaviour — one stderr diagnostic, no error return, no exit-code change — is
// identical to a fail-open exporter failure.
var errOTLPExportDeclined = errors.New("built with ax_no_otlp; rebuild without that tag to restore export")

func newOTLPExporter(_ context.Context, _ Config) (sdktrace.SpanExporter, error) {
	return nil, errOTLPExportDeclined
}
