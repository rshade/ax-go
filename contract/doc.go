// Package contract provides import-isolated machine contracts for ax-go consumers.
//
// It owns deterministic exit codes, output mode resolution, context metadata,
// success envelopes, strict JSON writers, and the structured error envelope.
// The package does not import the root ax facade or runtime telemetry, logging,
// transport, or execution adapters.
//
// Consequently this package provides no live tracing: TraceIDFromContext and
// SpanIDFromContext read back metadata a caller already stored with
// WithMetadata, never an active OpenTelemetry span context. Real trace IDs come
// from the root ax package, where StartTelemetry installs W3C propagation and
// Execute opens a recording root span.
package contract
