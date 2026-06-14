package ax

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	internaltelemetry "github.com/rshade/ax-go/internal/telemetry"
)

// defaultTelemetryShutdownTimeout derives from the canonical budget defined in
// internal/telemetry so the two packages cannot drift.
const defaultTelemetryShutdownTimeout = internaltelemetry.DefaultShutdownBudget

// Telemetry owns the OTel provider lifecycle for a short-lived CLI process.
type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
}

// TelemetryOption configures StartTelemetry.
type TelemetryOption func(*telemetryConfig)

type telemetryConfig struct {
	env            func(string) string
	stderr         io.Writer
	serviceName    string
	serviceVersion string
	shutdownBudget time.Duration
}

// WithTelemetryEnv sets the environment lookup used for telemetry configuration
// and trace extraction. StartTelemetry resolves OTEL_EXPORTER_OTLP_ENDPOINT and
// AX_OTEL_DEBUG (exporter and debug selection) as well as TRACEPARENT and
// TRACESTATE (inbound trace continuation) through this lookup, so a custom
// function controls all of them in tests and embedding scenarios.
func WithTelemetryEnv(env func(string) string) TelemetryOption {
	return func(cfg *telemetryConfig) {
		cfg.env = env
	}
}

// WithTelemetryStderr sets the operational telemetry writer.
//
// StartTelemetry writes fail-open diagnostics and AX_OTEL_DEBUG span output to
// this writer. It defaults to os.Stderr and must not be the command stdout.
func WithTelemetryStderr(w io.Writer) TelemetryOption {
	return func(cfg *telemetryConfig) {
		cfg.stderr = w
	}
}

// WithTelemetryServiceName sets the OTel resource service.name value.
//
// The value should be a low-cardinality CLI name and must not include PII,
// credentials, resource IDs, or user-controlled command input.
func WithTelemetryServiceName(name string) TelemetryOption {
	return func(cfg *telemetryConfig) {
		cfg.serviceName = name
	}
}

// WithTelemetryServiceVersion sets the OTel resource service.version value.
//
// The value should be the deterministic build-injected version reported by the
// adopting CLI and must not include PII or high-cardinality runtime data.
func WithTelemetryServiceVersion(version string) TelemetryOption {
	return func(cfg *telemetryConfig) {
		cfg.serviceVersion = version
	}
}

// WithTelemetryShutdownBudget sets the timeout budget for telemetry shutdown
// and synchronous exporter attempts.
//
// Non-positive durations are ignored by StartTelemetry, which falls back to the
// default Execute telemetry shutdown timeout. A stuck collector maps to a
// fail-open stderr diagnostic rather than a command failure.
func WithTelemetryShutdownBudget(d time.Duration) TelemetryOption {
	return func(cfg *telemetryConfig) {
		cfg.shutdownBudget = d
	}
}

// StartTelemetry installs W3C trace propagation and extracts TRACEPARENT.
//
// It configures telemetry from the supplied environment lookup. An
// OTEL_EXPORTER_OTLP_ENDPOINT value enables OTLP HTTP export with synchronous,
// bounded attempts; AX_OTEL_DEBUG enables human-readable span output on the
// configured stderr writer. With neither set, telemetry is no-op for export
// while still allowing Execute to create a recording root span for log
// correlation.
//
// StartTelemetry is fail-open: telemetry setup failures are reported to stderr
// and the returned error is reserved for signature compatibility and currently
// always nil.
func StartTelemetry(ctx context.Context, opts ...TelemetryOption) (context.Context, *Telemetry, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg := telemetryConfig{
		env:            os.Getenv,
		stderr:         os.Stderr,
		shutdownBudget: defaultTelemetryShutdownTimeout,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.env == nil {
		cfg.env = os.Getenv
	}
	if cfg.stderr == nil {
		cfg.stderr = os.Stderr
	}
	if cfg.shutdownBudget <= 0 {
		cfg.shutdownBudget = defaultTelemetryShutdownTimeout
	}

	ctx, tp, err := internaltelemetry.Start(ctx, internaltelemetry.Config{
		TraceParent:    cfg.env("TRACEPARENT"),
		TraceState:     cfg.env("TRACESTATE"),
		OTLPEndpoint:   cfg.env("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Debug:          telemetryDebugEnabled(cfg.env("AX_OTEL_DEBUG")),
		Stderr:         cfg.stderr,
		ServiceName:    cfg.serviceName,
		ServiceVersion: cfg.serviceVersion,
		ShutdownBudget: cfg.shutdownBudget,
	})
	if err != nil {
		// This branch is intentionally unreachable today: internaltelemetry.Start
		// handles all failure modes fail-open internally and always returns nil.
		// The branch exists solely for signature stability — if a future change to
		// internal.Start ever makes it fallible, the public API contract (fail-open
		// with a no-op provider) is already enforced here without a breaking change.
		fmt.Fprintf(cfg.stderr, "ax: otel disabled: %s\n", internaltelemetry.SanitizeDiagnostic(err.Error()))
		tp = sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
		otel.SetTracerProvider(tp)
	}

	return ctx, &Telemetry{TracerProvider: tp}, nil
}

func telemetryDebugEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// Shutdown flushes and shuts down the configured tracer provider.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.TracerProvider == nil {
		return nil
	}
	return t.TracerProvider.Shutdown(ctx)
}
