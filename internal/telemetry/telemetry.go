package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// DefaultShutdownBudget is the canonical timeout for telemetry shutdown and
// synchronous exporter attempts. It is the single source of truth for the
// budget; the public ax package derives its default from this value.
const DefaultShutdownBudget = 2 * time.Second

// Config is the telemetry configuration resolved by the public StartTelemetry
// constructor before the provider is installed.
type Config struct {
	TraceParent    string
	TraceState     string
	OTLPEndpoint   string
	Debug          bool
	Stderr         io.Writer
	ServiceName    string
	ServiceVersion string
	ShutdownBudget time.Duration
}

// Start installs W3C trace propagation, resolves exporters from cfg, and
// returns a context carrying any inbound trace/state from cfg.TraceParent and
// cfg.TraceState, a fully configured TracerProvider, and a nil error.
//
// Exporter resolution (fail-open):
//   - cfg.OTLPEndpoint set → OTLP HTTP exporter attached via SimpleSpanProcessor;
//     construction failure emits a one-time stderr diagnostic and the exporter
//     is skipped — the provider remains usable.
//   - cfg.Debug set → pretty-print stderr debug exporter attached; same
//     fail-open behavior on construction failure.
//   - Both, neither, or either may be active simultaneously.
//
// The error return is always nil. Exporter and propagation failures degrade
// to a no-op/recording-only provider rather than surfacing an error; the error
// return exists solely for signature stability in case a future change makes
// Start fallible.
func Start(ctx context.Context, cfg Config) (context.Context, *sdktrace.TracerProvider, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}

	propagator := propagation.TraceContext{}
	otel.SetTextMapPropagator(propagator)

	carrier := propagation.MapCarrier{}
	if cfg.TraceParent != "" {
		carrier["traceparent"] = cfg.TraceParent
	}
	if cfg.TraceState != "" {
		carrier["tracestate"] = cfg.TraceState
	}
	ctx = propagator.Extract(ctx, carrier)

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(telemetryResource(cfg)),
	}
	if cfg.OTLPEndpoint != "" {
		exporter, err := newOTLPExporter(ctx, cfg)
		if err != nil {
			writeDiagnostic(cfg.Stderr, "otel exporter disabled", err)
		} else {
			options = append(options, sdktrace.WithSpanProcessor(
				sdktrace.NewSimpleSpanProcessor(exporter),
			))
		}
	}
	if cfg.Debug {
		exporter, err := stdouttrace.New(
			stdouttrace.WithWriter(NewLockedWriter(cfg.Stderr)),
			stdouttrace.WithPrettyPrint(),
			stdouttrace.WithoutTimestamps(),
		)
		if err != nil {
			writeDiagnostic(cfg.Stderr, "otel debug exporter disabled", err)
		} else {
			options = append(options, sdktrace.WithSpanProcessor(
				sdktrace.NewSimpleSpanProcessor(&diagnosticExporter{
					exporter: exporter,
					stderr:   cfg.Stderr,
				}),
			))
		}
	}

	tp := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(tp)

	return ctx, tp, nil
}

// telemetryResource builds the span resource identity. It merges the SDK
// default resource — preserving telemetry.sdk.* and the unknown_service.name
// fallback — with the configured service.name/service.version, each added only
// when set so an empty value never overwrites the default. The merge cannot hit
// a schema-URL conflict because resource.Default and the imported semconv share
// one pinned version; the fallback keeps the resource usable if that ever drifts.
func telemetryResource(cfg Config) *resource.Resource {
	var attrs []attribute.KeyValue
	if cfg.ServiceName != "" {
		attrs = append(attrs, semconv.ServiceName(cfg.ServiceName))
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	custom := resource.NewWithAttributes(semconv.SchemaURL, attrs...)
	merged, err := resource.Merge(resource.Default(), custom)
	if err != nil {
		return custom
	}
	return merged
}

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

func normalizeOTLPEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse OTEL_EXPORTER_OTLP_ENDPOINT: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT must use http or https scheme")
	}
	if u.Host == "" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT must include a host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("OTEL_EXPORTER_OTLP_ENDPOINT must not include query or fragment")
	}
	u.Path = path.Join(u.Path, "/v1/traces")
	return u.String(), nil
}

type diagnosticExporter struct {
	exporter sdktrace.SpanExporter
	stderr   io.Writer
	once     sync.Once
}

// ExportSpans is fail-open: an export failure is reported as a one-time stderr
// diagnostic and the return is always nil so a single unreachable collector
// cannot cascade into a command failure.
func (e *diagnosticExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := e.exporter.ExportSpans(ctx, spans); err != nil {
		e.once.Do(func() {
			writeDiagnostic(e.stderr, "otel export failed", err)
		})
	}
	return nil
}

func (e *diagnosticExporter) Shutdown(ctx context.Context) error {
	if err := e.exporter.Shutdown(ctx); err != nil {
		e.once.Do(func() {
			writeDiagnostic(e.stderr, "otel export failed", err)
		})
	}
	// Shutdown is fail-open like ExportSpans: a stuck or unreachable collector at
	// flush time is a stderr diagnostic, never a command failure. The once shared
	// with ExportSpans collapses a single outage into one diagnostic instead of
	// reporting it again at shutdown.
	return nil
}

// lockedWriter serializes concurrent writes to an underlying io.Writer using a
// mutex. NewLockedWriter constructs one; the type itself is unexported.
type lockedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

// NewLockedWriter returns an io.Writer that serializes all writes to w with a
// mutex. It is used at two intentional layers of defense-in-depth: Execute (ax
// package) wraps cfg.stderr before any goroutine touches it, and Start wraps
// the same writer again for the specific inner path taken by the stdouttrace
// debug exporter. Each layer cannot assume the other synchronizes its writes.
func NewLockedWriter(w io.Writer) io.Writer {
	return &lockedWriter{writer: w}
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

func writeDiagnostic(w io.Writer, message string, err error) {
	if w == nil {
		w = os.Stderr
	}
	if err == nil {
		fmt.Fprintf(w, "ax: %s\n", message)
		return
	}
	fmt.Fprintf(w, "ax: %s: %s\n", message, SanitizeDiagnostic(err.Error()))
}

// SanitizeDiagnostic replaces ASCII control characters (< 0x20 and DEL 0x7f)
// with spaces, preventing log-forging via newline injection or ANSI escapes in
// error messages that are written to stderr diagnostics.
func SanitizeDiagnostic(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, value)
}
