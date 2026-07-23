package ax

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

// TestHTTPClientTimeout verifies the timeout contract: NewHTTPClient never
// returns a client with an unbounded timeout. The default is DefaultHTTPTimeout,
// WithHTTPTimeout overrides it, and non-positive overrides fall back to the
// default rather than disabling the timeout.
func TestHTTPClientTimeout(t *testing.T) {
	tests := []struct {
		name string
		opts []HTTPClientOption
		want time.Duration
	}{
		{name: "default", opts: nil, want: DefaultHTTPTimeout},
		{name: "override", opts: []HTTPClientOption{WithHTTPTimeout(5 * time.Second)}, want: 5 * time.Second},
		{name: "zero falls back to default", opts: []HTTPClientOption{WithHTTPTimeout(0)}, want: DefaultHTTPTimeout},
		{
			name: "negative falls back to default",
			opts: []HTTPClientOption{WithHTTPTimeout(-time.Second)},
			want: DefaultHTTPTimeout,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := NewHTTPClient(tc.opts...)
			if client.Timeout != tc.want {
				t.Fatalf("NewHTTPClient().Timeout = %s, want %s", client.Timeout, tc.want)
			}
		})
	}
}

// TestHTTPClientDefaultTimeout verifies the zero-argument HTTPClient constructor
// delegates to the default-timeout path, so callers using the original API still
// get a bounded client.
func TestHTTPClientDefaultTimeout(t *testing.T) {
	if got := HTTPClient().Timeout; got != DefaultHTTPTimeout {
		t.Fatalf("HTTPClient().Timeout = %s, want %s", got, DefaultHTTPTimeout)
	}
}

func TestHTTPClientPropagatesActiveSpanTraceparent(t *testing.T) {
	ctx, telemetry, err := StartTelemetry(
		context.Background(),
		WithTelemetryEnv(func(string) string { return "" }),
		WithTelemetryServiceName("app"),
	)
	if err != nil {
		t.Fatalf("StartTelemetry returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := telemetry.Shutdown(context.Background()); err != nil {
			t.Fatalf("Telemetry.Shutdown returned error: %v", err)
		}
	})

	ctx, span := otel.Tracer("github.com/rshade/ax-go/test").Start(ctx, "app")
	defer span.End()
	traceID := TraceIDFromContext(ctx)

	seen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("Traceparent")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}
	resp, err := HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("HTTPClient.Do returned error: %v", err)
	}
	defer resp.Body.Close()

	traceparent := <-seen
	if traceparent == "" {
		t.Fatal("traceparent header missing")
	}
	if !strings.Contains(traceparent, traceID) {
		t.Fatalf("traceparent = %q, want trace ID %q", traceparent, traceID)
	}
}

// TestHTTPClientRejectsInvalidTLSCert verifies that HTTPClient() uses standard
// TLS verification: it must reject self-signed certificates that the system's
// trust store does not recognize.
func TestHTTPClientRejectsInvalidTLSCert(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	_, err = HTTPClient().Do(req)
	if err == nil {
		t.Fatal("HTTPClient should reject self-signed cert; got nil error")
	}
	// Confirm the failure is a typed x509 certificate error, not a generic network error.
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("expected *url.Error wrapping TLS failure, got: %v", err)
	}
	var certErr x509.CertificateInvalidError
	var authErr x509.UnknownAuthorityError
	if !errors.As(err, &certErr) && !errors.As(err, &authErr) {
		t.Fatalf("expected x509 certificate error, got: %v", err)
	}
}
