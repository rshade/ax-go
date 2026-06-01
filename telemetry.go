package ax

import (
	"context"
	"os"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	internaltelemetry "github.com/rshade/ax-go/internal/telemetry"
)

// Telemetry owns the OTel provider lifecycle for a short-lived CLI process.
type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
}

// TelemetryOption configures StartTelemetry.
type TelemetryOption func(*telemetryConfig)

type telemetryConfig struct {
	env func(string) string
}

// WithTelemetryEnv sets the environment lookup used for trace extraction.
func WithTelemetryEnv(env func(string) string) TelemetryOption {
	return func(cfg *telemetryConfig) {
		cfg.env = env
	}
}

// StartTelemetry installs W3C trace propagation and extracts TRACEPARENT.
//
// The initial scaffold uses the OTel SDK no-op exporter path. OTLP and debug
// exporters can be layered onto this lifecycle without changing callers.
func StartTelemetry(ctx context.Context, opts ...TelemetryOption) (context.Context, *Telemetry, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg := telemetryConfig{env: os.Getenv}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, tp, err := internaltelemetry.Start(ctx, cfg.env)
	if err != nil {
		return nil, nil, err
	}

	return ctx, &Telemetry{TracerProvider: tp}, nil
}

// Shutdown flushes and shuts down the configured tracer provider.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.TracerProvider == nil {
		return nil
	}
	return t.TracerProvider.Shutdown(ctx)
}
