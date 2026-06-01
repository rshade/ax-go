package telemetry

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Start installs W3C trace propagation and returns a tracer provider.
func Start(ctx context.Context, env func(string) string) (context.Context, *sdktrace.TracerProvider, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if env == nil {
		env = os.Getenv
	}

	propagator := propagation.TraceContext{}
	otel.SetTextMapPropagator(propagator)

	carrier := propagation.MapCarrier{}
	if traceparent := env("TRACEPARENT"); traceparent != "" {
		carrier["traceparent"] = traceparent
	}
	if tracestate := env("TRACESTATE"); tracestate != "" {
		carrier["tracestate"] = tracestate
	}
	ctx = propagator.Extract(ctx, carrier)

	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)

	return ctx, tp, nil
}
