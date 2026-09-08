# Public API Contract: Live-Resolving Metadata Accessor for Root ax

**Feature**: `025-metadata-from-context` | **Date**: 2026-09-07

## Surface delta (root package `ax`)

```go
// MetadataFromContext returns the envelope metadata ax would emit for ctx:
// the live OpenTelemetry trace and span IDs (ZeroTraceID and ZeroSpanID when
// no span is active) merged with the dry-run state and idempotency key that
// Execute stores. A trace or span ID a caller stored explicitly through
// contract.WithMetadata is superseded by the live span, exactly as NewEnvelope
// and NewError already behave.
//
// contract.MetadataFromContext cannot see the active span — this package is
// import-isolated from the OpenTelemetry SDK — and returns zero IDs for a
// root-ax context. Use this function instead when calling from root ax code.
func MetadataFromContext(ctx context.Context) Metadata
```

`Metadata`, `NewEnvelope`, `NewError`, `TraceIDFromContext`,
`SpanIDFromContext`, and every other exported root-package signature remain
unchanged. No new package, type, constant, variable, flag, environment
variable, or machine-payload field is added.

## Surface delta (public package `contract`) — doc-comment only

```go
// MetadataFromContext returns explicit metadata from ctx merged with dry-run
// and idempotency-key context helpers.
//
// This does not resolve an active OpenTelemetry span context: this package is
// import-isolated from the OpenTelemetry SDK and provides no live tracing, so
// the TraceID and SpanID fields reflect only metadata a caller already stored
// with WithMetadata, defaulting to ZeroTraceID/ZeroSpanID otherwise. Live
// tracing comes from the root ax package: use ax.MetadataFromContext instead
// when calling from root ax code.
func MetadataFromContext(ctx context.Context) Metadata
```

`contract.MetadataFromContext`'s signature and return value are unchanged for
every input. This is a documentation-only edit; it does not appear as a
`surface-check` or `apidiff` finding because no signature changes.

## Behavioral contract

| Contract ID | Requirement |
|-------------|-------------|
| MC-01 | `ax.MetadataFromContext(ctx)` equals `ax.NewEnvelope(ctx, x).Meta` field-for-field for any `ctx` and any `x`, observed at the same point in execution. |
| MC-02 | When `ctx` carries no active tracing span, `TraceID`/`SpanID` are `ZeroTraceID`/`ZeroSpanID`. |
| MC-03 | A `nil` `ctx` does not panic; behavior matches `context.Background()`. |
| MC-04 | `DryRun` and `IdempotencyKey` reflect the same context values `contract.MetadataFromContext` already surfaces for those two fields. |
| MC-05 | When `ctx` carries both an explicitly stored trace/span ID and an active live span, the live span's IDs take precedence. |
| MC-06 | `contract.MetadataFromContext`'s return value is byte-for-byte unchanged by this feature for every input. |
| MC-07 | The export and behavior are identical in all four supported build configurations and six surface profiles. |

## Stability and release classification

- **Go API**: additive, supported root-package function.
- **Existing semantics**: `contract.MetadataFromContext` behavior is
  unchanged for every caller; only its doc comment gains a warning
  paragraph.
- **Machine payloads**: unchanged — `Metadata`, `Envelope`, and `Error` keep
  their existing JSON shapes.
- **SemVer**: non-breaking `feat:`; while pre-v1, release-please increments
  the minor digit under Constitution Principle XI.
- **Deprecation**: none.

## Public-surface artifacts

The reviewed live baseline gains this universally present feature (added via
`make surface-update`, then reviewed line by line):

```json
{
  "id": "func:MetadataFromContext",
  "signature": "func(context.Context) Metadata",
  "configurations": "all",
  "profiles": "all"
}
```

The permanent audit
(`specs/023-internalize-helpers/public-surface-audit.json`) gains a matching
row: classification `supported`, disposition `keep-public`, lifecycle `live`,
rationale referencing the live-resolving envelope-metadata contract. Root
`ax` is already an allowed public package for both `surface-check` and
`apidiff`, so no package-allowlist change is needed.

## Documentation and example contract

- `ax.MetadataFromContext` carries the full contract doc comment above.
- `ExampleMetadataFromContext` is added as a standalone verified example
  (not gated by `make doc-coverage`, since the function is not in
  doccover's `requiredSymbols()`, but encouraged per AGENTS.md's "other
  exported symbols where they add clarity" guidance).
- README's section introducing `ax.NewEnvelope` gains a one- or two-sentence
  mention of `ax.MetadataFromContext` for callers who need the metadata
  without building an envelope.
- The `build-your-first-cli` tutorial gains the same mention at the point
  `NewEnvelope` is first introduced.

## Non-goals

- No change to `contract.MetadataFromContext`'s returned values.
- No change to the `Metadata`, `Envelope`, or `Error` JSON shapes.
- No pre-population of trace metadata inside `Execute`'s stored context —
  resolution stays at read time so a child span's IDs are never stale.
- No new exported type, constant, or `ExecuteOption`.
