package ax

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
	tracecollector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type otlpTraceExport struct {
	traceIDs      []string
	names         []string
	resourceAttrs map[string]string
	statusCodes   []tracepb.Status_StatusCode
	err           error
}

type otlpTraceReceiver struct {
	server  *httptest.Server
	exports chan otlpTraceExport
}

func newOTLPTraceReceiver(t *testing.T) *otlpTraceReceiver {
	t.Helper()

	receiver := &otlpTraceReceiver{
		exports: make(chan otlpTraceExport, 8),
	}
	receiver.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			receiver.exports <- otlpTraceExport{err: fmt.Errorf("read OTLP request: %w", err)}
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		var req tracecollector.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			receiver.exports <- otlpTraceExport{err: fmt.Errorf("decode OTLP request: %w", err)}
			http.Error(w, "decode failed", http.StatusBadRequest)
			return
		}
		receiver.exports <- otlpTraceExport{
			traceIDs:      otlpTraceIDs(&req),
			names:         otlpSpanNames(&req),
			resourceAttrs: otlpResourceAttrs(&req),
			statusCodes:   otlpSpanStatuses(&req),
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.server.Close)
	return receiver
}

func (r *otlpTraceReceiver) endpoint() string {
	return r.server.URL
}

func (r *otlpTraceReceiver) next(t *testing.T) otlpTraceExport {
	t.Helper()

	select {
	case export := <-r.exports:
		if export.err != nil {
			t.Fatalf("OTLP receiver error: %v", export.err)
		}
		return export
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OTLP export")
		return otlpTraceExport{}
	}
}

// otlpResourceAttrs extracts all string-valued resource attributes from an
// OTLP export request. service.name and service.version are always string
// attributes, so string extraction is sufficient for verifying service identity.
//
// Navigate the hierarchy:
//
//	ExportTraceServiceRequest
//	  └─ ResourceSpans[]
//	       └─ Resource.Attributes[]  ← key-value pairs with service identity
//
// Each KeyValue carries a Key string and a Value *AnyValue. For string attrs,
// AnyValue.GetStringValue() returns the value; for other types (int, bool, …)
// it returns "". Skip those — we only need the string attrs.
//
// All spans from a single ax-go command share one resource, so the first
// non-nil resource is sufficient; break after you've processed it.
func otlpResourceAttrs(req *tracecollector.ExportTraceServiceRequest) map[string]string {
	attrs := make(map[string]string)
	for _, rs := range req.GetResourceSpans() {
		res := rs.GetResource()
		if res == nil {
			continue
		}
		for _, kv := range res.GetAttributes() {
			if v := kv.GetValue().GetStringValue(); v != "" {
				attrs[kv.GetKey()] = v
			}
		}
		break
	}
	return attrs
}

func otlpTraceIDs(req *tracecollector.ExportTraceServiceRequest) []string {
	var ids []string
	for _, resourceSpans := range req.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				if id := hex.EncodeToString(span.GetTraceId()); id != "" {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

func otlpSpanNames(req *tracecollector.ExportTraceServiceRequest) []string {
	var names []string
	for _, resourceSpans := range req.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				names = append(names, span.GetName())
			}
		}
	}
	return names
}

// otlpSpanStatuses extracts the status code from every span in the request.
// Spans whose Status field is nil (not yet set) are omitted; a nil status
// means the OTel SDK serialized neither OK nor Error — the span is in the
// default UNSET state and the proto field was elided.
func otlpSpanStatuses(req *tracecollector.ExportTraceServiceRequest) []tracepb.Status_StatusCode {
	var codes []tracepb.Status_StatusCode
	for _, resourceSpans := range req.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				if status := span.GetStatus(); status != nil {
					codes = append(codes, status.GetCode())
				}
			}
		}
	}
	return codes
}

func TestExecuteExportsSpansBeforeExit(t *testing.T) {
	t.Run("new trace", func(t *testing.T) {
		receiver := newOTLPTraceReceiver(t)
		stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
			"OTEL_EXPORTER_OTLP_ENDPOINT": receiver.endpoint(),
		}, defaultTelemetryShutdownTimeout)

		if code != ExitSuccess {
			t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
		}
		export := receiver.next(t)
		if len(export.traceIDs) == 0 {
			t.Fatalf("exported trace IDs empty; span names=%v", export.names)
		}
		if export.traceIDs[0] == ZeroTraceID {
			t.Fatalf("exported trace ID = %q, want non-zero", export.traceIDs[0])
		}
		if bytes.Contains(stdout, []byte(export.traceIDs[0])) {
			t.Fatalf("stdout contains exported trace ID %q: %s", export.traceIDs[0], stdout)
		}
	})

	tests := []struct {
		name        string
		traceparent string
		wantTraceID string
	}{
		{
			name:        "continues inbound trace",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantTraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		},
		{
			name:        "exports despite unsampled inbound trace",
			traceparent: "00-11111111111111111111111111111111-00f067aa0ba902b7-00",
			wantTraceID: "11111111111111111111111111111111",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			receiver := newOTLPTraceReceiver(t)
			_, stderr, code := executeTelemetryCommand(t, map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": receiver.endpoint(),
				"TRACEPARENT":                 tc.traceparent,
			}, defaultTelemetryShutdownTimeout)

			if code != ExitSuccess {
				t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
			}
			export := receiver.next(t)
			if !containsString(export.traceIDs, tc.wantTraceID) {
				t.Fatalf("exported trace IDs = %v, want %q", export.traceIDs, tc.wantTraceID)
			}
		})
	}
}

func executeTelemetryCommand(t *testing.T, env map[string]string, shutdownBudget time.Duration) ([]byte, string, int) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
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
		WithTelemetryShutdownTimeout(shutdownBudget),
		WithEnv(func(key string) string {
			return env[key]
		}),
	)
	return stdout.Bytes(), stderr.String(), code
}

// TestExecuteExportsServiceResourceIdentity verifies that service.name and
// service.version are non-empty in exported OTLP spans (issue #47).
//
// service.name comes from root.Name() (the Cobra Use field); service.version
// comes from ax.WithVersion → WithTelemetryServiceVersion → telemetryResource.
// Both values must survive the full chain and appear in the resource attributes
// of every exported ResourceSpans message.
func TestExecuteExportsServiceResourceIdentity(t *testing.T) {
	const wantVersion = "v1.2.3"
	const wantName = "testapp"

	receiver := newOTLPTraceReceiver(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := &cobra.Command{
		Use: wantName,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
		WithVersion(wantVersion),
		WithTelemetryShutdownTimeout(defaultTelemetryShutdownTimeout),
		WithEnv(func(key string) string {
			if key == "OTEL_EXPORTER_OTLP_ENDPOINT" {
				return receiver.endpoint()
			}
			return ""
		}),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}

	export := receiver.next(t)

	if v := export.resourceAttrs["service.version"]; v == "" {
		t.Fatal("service.version is empty in exported resource; WithVersion did not reach WithTelemetryServiceVersion")
	} else if v != wantVersion {
		t.Fatalf("service.version = %q, want %q", v, wantVersion)
	}

	if v := export.resourceAttrs["service.name"]; v == "" {
		t.Fatal("service.name is empty in exported resource; root.Name() must be non-empty")
	} else if v != wantName {
		t.Fatalf("service.name = %q, want %q", v, wantName)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestExecuteErrorCommandExportsSpanWithErrorStatus verifies that when a Cobra
// command returns an error, Execute sets span status to codes.Error before the
// span is flushed to the OTLP collector (execute.go: span.SetStatus).
//
// The span.End() deferred before Shutdown means the status is already set when
// the SimpleSpanProcessor exports it — so the OTLP receiver must see
// STATUS_CODE_ERROR on the root span, not UNSET or OK.
func TestExecuteErrorCommandExportsSpanWithErrorStatus(t *testing.T) {
	receiver := newOTLPTraceReceiver(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return NewError(cmd.Context(), "validation_error", "bad input",
				WithErrorExitCode(ExitValidation))
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithStdoutIsTTY(false),
		WithTelemetryShutdownTimeout(defaultTelemetryShutdownTimeout),
		WithEnv(func(key string) string {
			if key == "OTEL_EXPORTER_OTLP_ENDPOINT" {
				return receiver.endpoint()
			}
			return ""
		}),
	)

	if code == ExitSuccess {
		t.Fatalf("Execute exit code = %d, want non-zero (command returned error)", code)
	}

	export := receiver.next(t)

	if len(export.statusCodes) == 0 {
		t.Fatalf(
			"no span had an explicit status code; Execute must call span.SetStatus(codes.Error) on failure — names=%v",
			export.names,
		)
	}
	hasError := false
	for _, sc := range export.statusCodes {
		if sc == tracepb.Status_STATUS_CODE_ERROR {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Fatalf(
			"no span with STATUS_CODE_ERROR; status codes=%v names=%v — Execute must call span.SetStatus(codes.Error) on failure",
			export.statusCodes,
			export.names,
		)
	}
}
