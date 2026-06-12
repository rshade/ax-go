package ax

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
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
