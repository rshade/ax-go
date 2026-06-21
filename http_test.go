package ax

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

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

// TestGRPCDialCancelledContextReturnsError verifies that GRPCDial respects a
// pre-cancelled context and returns context.Canceled before attempting a dial.
func TestGRPCDialCancelledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GRPCDial(ctx, "localhost:9999")
	if err == nil {
		t.Fatal("GRPCDial with cancelled context should return error")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context cancellation error, got: %v", err)
	}
}

// TestGRPCDialWithInsecureCredsReturnsConn verifies that GRPCDial returns a
// non-nil ClientConn when the caller explicitly opts into plaintext transport.
// Callers that need TLS pass grpc.WithTransportCredentials(credentials.NewTLS(...)).
func TestGRPCDialWithInsecureCredsReturnsConn(t *testing.T) {
	conn, err := GRPCDial(
		context.Background(),
		"localhost:9999",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("GRPCDial with insecure creds returned error: %v", err)
	}
	if conn == nil {
		t.Fatal("GRPCDial returned nil conn")
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close: %v", err)
	}
}

// TestGRPCDialIsSecureByDefault verifies that GRPCDial enforces TLS by default.
//
// Strategy (Approach C): start an in-process plaintext gRPC server on a random
// local port, then show two things in sequence:
//  1. GRPCDial WITHOUT credentials fails immediately (grpc.NewClient v1.65+
//     returns errNoTransportSecurity before touching the network).
//  2. GRPCDial WITH insecure.NewCredentials() reaches the same server and
//     receives codes.Unimplemented for an unknown method — proving the server
//     IS reachable, so the earlier failure was credential enforcement, not an
//     unreachable host.
func TestGRPCDialIsSecureByDefault(t *testing.T) {
	// Start a plaintext (no-TLS) gRPC server on a random local port.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	srv := grpc.NewServer()
	t.Cleanup(srv.GracefulStop)
	go func() { _ = srv.Serve(lis) }()

	addr := lis.Addr().String()

	// Without transport credentials GRPCDial must refuse immediately —
	// even though a reachable plaintext server is listening on the target.
	// grpc.NewClient returns errNoTransportSecurity before any network I/O,
	// making accidental cleartext connections structurally impossible.
	_, err = GRPCDial(context.Background(), addr)
	if err == nil {
		t.Fatal("GRPCDial without credentials should error even when the server is reachable")
	}
	if !strings.Contains(err.Error(), "transport") &&
		!strings.Contains(err.Error(), "security") &&
		!strings.Contains(err.Error(), "credential") {
		t.Fatalf("expected transport-security error, got: %v", err)
	}

	// Reachability proof: with explicit insecure credentials the connection
	// reaches the server. An codes.Unimplemented response for an unknown method
	// confirms the transport succeeded — it is not a TLS or connection error.
	conn, err := GRPCDial(
		context.Background(),
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("GRPCDial with insecure creds unexpectedly failed: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn.Close: %v", err)
		}
	}()

	// grpc-go encodes nil as an empty proto message, which is valid.
	// The server has no registered services, so it replies Unimplemented.
	invokeErr := conn.Invoke(context.Background(), "/probe.Probe/Ping", nil, nil)
	if status.Code(invokeErr) != codes.Unimplemented {
		t.Fatalf("expected codes.Unimplemented from unknown method on plaintext server, got: %v", invokeErr)
	}
}
