// Package logging is the import-isolated public logging surface of ax-go. It
// provides a trace-correlated, zerolog-backed structured logger without linking
// the ax-go runtime facade, so a logging-only consumer ships a substantially
// smaller binary than one importing root ax.
//
// # What this package does not link
//
// The transitive dependency graph of this package excludes, and is asserted to
// exclude, all of the following:
//
//   - github.com/rshade/ax-go (the root runtime facade)
//   - github.com/rshade/ax-go/internal/telemetry (OTLP exporter setup)
//   - go.opentelemetry.io/otel/sdk and go.opentelemetry.io/otel/exporters/...
//   - go.opentelemetry.io/contrib/instrumentation/... (otelhttp, otelgrpc)
//   - google.golang.org/grpc and google.golang.org/protobuf
//   - github.com/spf13/cobra
//   - net/http and crypto/tls
//
// It does link, and requires, github.com/rs/zerolog (the backend, which appears
// in Logger's method set) and go.opentelemetry.io/otel/trace — the trace API
// only, never the SDK — for reading an active span context.
//
// The exclusion holds identically under all four supported build configurations
// (default, ax_no_grpc, ax_no_otlp, and both), because this package never links
// the trees those constraints decline. No build tag is required to obtain it.
//
// # When to use root ax instead
//
// Two capabilities are deliberately unavailable here and remain reachable only
// through root ax, because both require dependencies this package exists to
// exclude:
//
//   - Loki direct push (ax.WithLokiFromEnv), which needs net/http.
//   - ax.Execute and OpenTelemetry export setup, which need Cobra and the
//     OTel SDK.
//
// Mixing surfaces is safe and supported. The types here are identity-preserving
// aliases of the same declarations root ax exposes, so an ax.Logger is a
// logging.Logger, and an option manufactured by root ax — including
// ax.WithLokiFromEnv — is accepted by NewLogger. A consumer that imports both
// gets one logger type, not two.
package logging
