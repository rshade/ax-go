//go:build ax_no_otlp

package telemetry

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// These tests compile only under ax_no_otlp. They pin the behaviour of the
// declined-export build: a configured endpoint is a diagnostic, never a failure,
// and the provider that comes back is fully usable for local tracing.

// TestStartDeclinesExportWithoutFailing asserts the fail-open contract holds
// when export is declined at build time (FR-008, FR-009): Start returns a nil
// error and a recording provider even though an endpoint is configured, and it
// says so exactly once on the configured stderr.
func TestStartDeclinesExportWithoutFailing(t *testing.T) {
	var stderr bytes.Buffer

	ctx, tp, err := Start(context.Background(), Config{
		OTLPEndpoint: "http://127.0.0.1:4318",
		Stderr:       &stderr,
		ServiceName:  "app",
	})
	if err != nil {
		t.Fatalf("Start returned error: %v, want nil — declining export must never fail", err)
	}
	if tp == nil {
		t.Fatal("Start returned a nil TracerProvider")
	}
	t.Cleanup(func() {
		if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
			t.Fatalf("TracerProvider.Shutdown returned error: %v", shutdownErr)
		}
	})

	_, span := tp.Tracer("test").Start(ctx, "root")
	defer span.End()
	if !span.IsRecording() {
		t.Fatal("span is not recording — declining export degraded tracing to no tracing")
	}
	if !span.SpanContext().IsValid() {
		t.Fatal("span context is not valid under ax_no_otlp")
	}

	if got := countDiagnostics(stderr.String(), "otel exporter disabled"); got != 1 {
		t.Fatalf("diagnostic count = %d, want exactly 1; stderr=%q", got, stderr.String())
	}
	// The prefix is load-bearing beyond tidiness: the root package's fail-open
	// test asserts on "ax: otel", so reusing the existing message is what keeps
	// that assertion true in both configurations.
	if !strings.Contains(stderr.String(), "ax: otel exporter disabled") {
		t.Fatalf("stderr = %q, want the canonical 'ax: otel exporter disabled' diagnostic", stderr.String())
	}
}

// TestStartEmitsOneDiagnosticPerStart pins the per-telemetry-start semantics
// recorded in research.md D4 (SC-006). Deliberately not once-per-process: a
// package-level sync.Once would be mutable package state, which the
// constitution forbids outright.
func TestStartEmitsOneDiagnosticPerStart(t *testing.T) {
	var first, second bytes.Buffer

	for _, target := range []*bytes.Buffer{&first, &second} {
		_, tp, err := Start(context.Background(), Config{
			OTLPEndpoint: "http://127.0.0.1:4318",
			Stderr:       target,
		})
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
		if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
			t.Fatalf("TracerProvider.Shutdown returned error: %v", shutdownErr)
		}
	}

	for i, got := range []string{first.String(), second.String()} {
		if count := countDiagnostics(got, "otel exporter disabled"); count != 1 {
			t.Fatalf("Start call %d emitted %d diagnostics, want exactly 1; stderr=%q", i+1, count, got)
		}
	}
}

// TestStartWithoutEndpointIsSilent asserts the decline is only reported when a
// consumer actually asked for export. A build that never configures an endpoint
// must produce no diagnostic at all.
func TestStartWithoutEndpointIsSilent(t *testing.T) {
	var stderr bytes.Buffer

	_, tp, err := Start(context.Background(), Config{Stderr: &stderr})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatalf("TracerProvider.Shutdown returned error: %v", shutdownErr)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty — no endpoint was configured, so nothing was declined", stderr.String())
	}
}

// TestStartPreservesInboundTraceContext asserts W3C extraction is untouched by
// the export decline — the half of tracing that survives (FR-009).
func TestStartPreservesInboundTraceContext(t *testing.T) {
	const (
		traceID    = "4bf92f3577b34da6a3ce929d0e0e4736"
		traceState = "rojo=00f067aa0ba902b7"
	)

	var stderr bytes.Buffer
	ctx, tp, err := Start(context.Background(), Config{
		TraceParent:  "00-" + traceID + "-00f067aa0ba902b7-01",
		TraceState:   traceState,
		OTLPEndpoint: "http://127.0.0.1:4318",
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() {
		if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
			t.Fatalf("TracerProvider.Shutdown returned error: %v", shutdownErr)
		}
	})

	sc := trace.SpanContextFromContext(ctx)
	if got := sc.TraceID().String(); got != traceID {
		t.Fatalf("TraceID = %q, want inbound %q", got, traceID)
	}
	if got := sc.TraceState().String(); got != traceState {
		t.Fatalf("TraceState = %q, want %q", got, traceState)
	}
}

// TestShutdownDoesNotBlockOnAbsentExporter asserts shutdown returns promptly
// rather than waiting on an exporter that was never constructed (FR-014). The
// endpoint points at a closed port precisely so a real exporter would stall.
func TestShutdownDoesNotBlockOnAbsentExporter(t *testing.T) {
	var stderr bytes.Buffer

	_, tp, err := Start(context.Background(), Config{
		OTLPEndpoint: "http://127.0.0.1:1",
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	start := time.Now()
	if shutdownErr := tp.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatalf("TracerProvider.Shutdown returned error: %v", shutdownErr)
	}
	elapsed := time.Since(start)

	if elapsed > DefaultShutdownBudget {
		t.Fatalf("Shutdown took %s, want under the %s budget — it is blocking on an absent exporter",
			elapsed, DefaultShutdownBudget)
	}
}

func countDiagnostics(stderr, message string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if strings.Contains(line, message) {
			count++
		}
	}
	return count
}
