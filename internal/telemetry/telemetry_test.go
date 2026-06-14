package telemetry

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestSanitizeDiagnostic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "sanitizes control characters",
			input: "bad\nforged: line\ttab\x1b[31mred\x7f",
			want:  "bad forged: line tab [31mred ",
		},
		{
			name:  "passes normal strings through unchanged",
			input: "normal error message",
			want:  "normal error message",
		},
		{
			name:  "replaces DEL (0x7f) with space",
			input: "before\x7fafter",
			want:  "before after",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeDiagnostic(tc.input); got != tc.want {
				t.Fatalf("SanitizeDiagnostic(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTelemetryResource(t *testing.T) {
	const (
		sdkNameKey     = "telemetry.sdk.name"
		serviceNameKey = "service.name"
		serviceVerKey  = "service.version"
	)

	attrsOf := func(res *resource.Resource) map[string]string {
		m := make(map[string]string)
		for _, kv := range res.Attributes() {
			m[string(kv.Key)] = kv.Value.AsString()
		}
		return m
	}

	t.Run("merges SDK defaults and omits empty service attributes", func(t *testing.T) {
		got := attrsOf(telemetryResource(Config{}))
		if v, ok := got[sdkNameKey]; !ok || v == "" {
			t.Fatalf("telemetry.sdk.name missing/empty (value=%q present=%v); SDK default resource was dropped", v, ok)
		}
		if name := got[serviceNameKey]; !strings.HasPrefix(name, "unknown_service") {
			t.Fatalf(
				"service.name = %q, want the SDK unknown_service fallback; an empty config must not overwrite it with \"\"",
				name,
			)
		}
		if v, ok := got[serviceVerKey]; ok {
			t.Fatalf("service.version present (%q) for empty config; want it unset rather than empty", v)
		}
	})

	t.Run("applies configured service identity alongside SDK defaults", func(t *testing.T) {
		got := attrsOf(telemetryResource(Config{ServiceName: "app", ServiceVersion: "1.2.3"}))
		if got[serviceNameKey] != "app" {
			t.Fatalf("service.name = %q, want %q", got[serviceNameKey], "app")
		}
		if got[serviceVerKey] != "1.2.3" {
			t.Fatalf("service.version = %q, want %q", got[serviceVerKey], "1.2.3")
		}
		if v, ok := got[sdkNameKey]; !ok || v == "" {
			t.Fatalf(
				"telemetry.sdk.name missing/empty (value=%q present=%v); SDK defaults must survive merge with service identity",
				v,
				ok,
			)
		}
	})
}

func TestNormalizeOTLPEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
		wantErr  string
	}{
		{
			name:     "http joins traces path",
			endpoint: "http://collector:4318",
			want:     "http://collector:4318/v1/traces",
		},
		{
			name:     "https accepted",
			endpoint: "https://collector:4318",
			want:     "https://collector:4318/v1/traces",
		},
		{
			name:     "existing path preserved",
			endpoint: "http://collector/base",
			want:     "http://collector/base/v1/traces",
		},
		{
			name:     "unparseable endpoint",
			endpoint: "://bad-endpoint",
			wantErr:  "parse OTEL_EXPORTER_OTLP_ENDPOINT",
		},
		{
			name:     "non-http scheme rejected",
			endpoint: "grpc://collector:4317",
			wantErr:  "must use http or https scheme",
		},
		{
			name:     "missing host rejected",
			endpoint: "http://",
			wantErr:  "must include a host",
		},
		{
			name:     "query rejected",
			endpoint: "http://collector:4318?debug=1",
			wantErr:  "must not include query or fragment",
		},
		{
			name:     "fragment rejected",
			endpoint: "http://collector:4318#frag",
			wantErr:  "must not include query or fragment",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeOTLPEndpoint(tc.endpoint)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("normalizeOTLPEndpoint(%q) error = nil, want substring %q", tc.endpoint, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("normalizeOTLPEndpoint(%q) error = %q, want substring %q", tc.endpoint, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeOTLPEndpoint(%q) returned error: %v", tc.endpoint, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeOTLPEndpoint(%q) = %q, want %q", tc.endpoint, got, tc.want)
			}
		})
	}
}

func TestNormalizeOTLPEndpointWrapsParseError(t *testing.T) {
	_, err := normalizeOTLPEndpoint("://bad-endpoint")
	if err == nil {
		t.Fatal("normalizeOTLPEndpoint returned nil error for unparseable endpoint")
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("normalizeOTLPEndpoint error chain = %v, want a *url.Error reachable via errors.As", err)
	}
}

type stubExporter struct {
	exportErr   error
	shutdownErr error
}

func (s *stubExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return s.exportErr
}

func (s *stubExporter) Shutdown(context.Context) error {
	return s.shutdownErr
}

func TestStartExtractsTraceState(t *testing.T) {
	const (
		traceID    = "4bf92f3577b34da6a3ce929d0e0e4736"
		spanID     = "00f067aa0ba902b7"
		traceState = "rojo=00f067aa0ba902b7"
	)

	ctx, tp, err := Start(context.Background(), Config{
		TraceParent: "00-" + traceID + "-" + spanID + "-01",
		TraceState:  traceState,
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
	})

	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		t.Fatal("span context is not valid after Start with TRACEPARENT+TRACESTATE")
	}
	if got := sc.TraceState().String(); got != traceState {
		t.Fatalf("TraceState = %q, want %q", got, traceState)
	}
}

func TestDiagnosticExporterShutdownFailOpen(t *testing.T) {
	var buf bytes.Buffer
	de := &diagnosticExporter{
		exporter: &stubExporter{shutdownErr: errors.New("collector unreachable")},
		stderr:   &buf,
	}

	if err := de.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error %v, want nil (fail-open)", err)
	}
	got := buf.String()
	if !strings.Contains(got, "ax: otel export failed") {
		t.Fatalf("Shutdown diagnostic = %q, want a fail-open export diagnostic", got)
	}
	if !strings.Contains(got, "collector unreachable") {
		t.Fatalf("Shutdown diagnostic = %q, want the underlying cause", got)
	}
}

func TestDiagnosticExporterShutdownSanitizesDiagnostic(t *testing.T) {
	var buf bytes.Buffer
	de := &diagnosticExporter{
		exporter: &stubExporter{shutdownErr: errors.New("bad\nforged-line\x1b[31m\x7f")},
		stderr:   &buf,
	}

	if err := de.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error %v, want nil (fail-open)", err)
	}
	// The single trailing newline from writeDiagnostic is expected; nothing else.
	body := strings.TrimSuffix(buf.String(), "\n")
	if strings.ContainsAny(body, "\n\x1b\x7f") {
		t.Fatalf("Shutdown diagnostic retained control characters (log-forging risk): %q", buf.String())
	}
}

func TestDiagnosticExporterReportsFailureOnce(t *testing.T) {
	var buf bytes.Buffer
	de := &diagnosticExporter{
		exporter: &stubExporter{
			exportErr:   errors.New("export boom"),
			shutdownErr: errors.New("shutdown boom"),
		},
		stderr: &buf,
	}

	if err := de.ExportSpans(context.Background(), nil); err != nil {
		t.Fatalf("ExportSpans returned error %v, want nil (fail-open)", err)
	}
	if err := de.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error %v, want nil (fail-open)", err)
	}

	if n := strings.Count(buf.String(), "ax: otel export failed"); n != 1 {
		t.Fatalf("diagnostic emitted %d times, want exactly 1 (shared sync.Once); output=%q", n, buf.String())
	}
}
