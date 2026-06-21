// Package contract provides import-isolated machine contracts for ax-go consumers.
//
// It owns deterministic exit codes, output mode resolution, context metadata,
// success envelopes, strict JSON writers, and the structured error envelope.
// The package does not import the root ax facade or runtime telemetry, logging,
// transport, or execution adapters.
package contract
