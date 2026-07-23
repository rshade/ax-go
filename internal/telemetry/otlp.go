//go:build !ax_no_otlp

// This file holds the real OTLP HTTP exporter construction and is the only
// place otlptracehttp — and therefore the gRPC, protobuf, OTLP-proto, and
// grpc-gateway trees — enters the module's dependency graph.
//
// Its sibling otlp_disabled.go supplies the same unexported function under the
// ax_no_otlp build constraint. Because the seam is unexported, Start's call site
// is identical in both configurations and the public API does not vary.

package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newOTLPExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	endpoint, err := normalizeOTLPEndpoint(cfg.OTLPEndpoint)
	if err != nil {
		return nil, err
	}
	budget := cfg.ShutdownBudget
	if budget <= 0 {
		budget = DefaultShutdownBudget
	}

	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithTimeout(budget),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP HTTP exporter: %w", err)
	}
	return &diagnosticExporter{
		exporter: exporter,
		stderr:   cfg.Stderr,
	}, nil
}
