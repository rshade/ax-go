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
	"google.golang.org/protobuf/proto"
)

type otlpTraceExport struct {
	traceIDs []string
	names    []string
	err      error
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
			traceIDs: otlpTraceIDs(&req),
			names:    otlpSpanNames(&req),
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
