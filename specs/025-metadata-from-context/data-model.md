# Data Model: Live-Resolving Metadata Accessor for Root ax

**Feature**: `025-metadata-from-context` | **Date**: 2026-09-07

This feature persists no data and introduces no new type. Its model is a
composition of three existing pieces of context state read by one new pure
function.

## Existing entity read (unchanged shape)

**Metadata** (`type Metadata = contract.Metadata`, unchanged by this feature):

| Field | Type | Source when read via `ax.MetadataFromContext` |
|-------|------|-------------------------------------------------|
| `TraceID` | `string` | Active OpenTelemetry span in `ctx`, or `ZeroTraceID` if none. |
| `SpanID` | `string` | Active OpenTelemetry span in `ctx`, or `ZeroSpanID` if none. |
| `DryRun` | `bool` | `contract.DryRunFromContext(ctx)`, as stored by `ax.Execute`. |
| `IdempotencyKey` | `string` | `contract.IdempotencyKeyFromContext(ctx)`, as stored by `ax.Execute`. |

No field is added, removed, or retyped. `ax.MetadataFromContext` is a second,
live-resolving read path onto the same record `NewEnvelope`/`NewError`
already populate — it does not define a new record.

## Composition

```text
ax.MetadataFromContext(ctx)
    = contract.MetadataFromContext(withTraceMetadata(ctx))

withTraceMetadata(ctx)
    = ctx decorated with contract.Metadata{TraceID, SpanID}
      read live from the active OpenTelemetry span in ctx
      (ZeroTraceID/ZeroSpanID when no span is active)

contract.MetadataFromContext(decorated_ctx)
    = decorated_ctx's stored Metadata{TraceID, SpanID}
      merged with DryRunFromContext(ctx) and IdempotencyKeyFromContext(ctx)
```

This is the exact composition `NewEnvelope` (json.go:18) and `NewError`
(error.go:23) already perform before constructing their respective payloads;
`ax.MetadataFromContext` exposes the same read without requiring a payload to
be constructed around it.

## Precedence table

| `ctx` carries explicit `contract.WithMetadata` trace/span IDs? | `ctx` has an active live span? | Result `TraceID`/`SpanID` |
|---|---|---|
| No | No | `ZeroTraceID`/`ZeroSpanID` |
| No | Yes | Live span's IDs |
| Yes | No | The explicitly stored IDs |
| Yes | Yes | Live span's IDs (live always supersedes explicit) |

This table is not new behavior — it already governs `NewEnvelope`/`NewError`
today via the same `withTraceMetadata` seam — but was previously undocumented
at a call-site an application developer could reach directly. `research.md`
D2 covers why no source change is needed to realize this table beyond the new
function's doc comment.

## Invariants

- `ax.MetadataFromContext(ctx)` equals `ax.NewEnvelope(ctx, anyValue).Meta`
  and equals `ax.NewError(ctx, anyCode, anyMessage).Meta`-equivalent fields,
  for the same `ctx` observed at the same point in execution. This equality
  is the primary regression test (research.md D4, test 1).
- `ax.MetadataFromContext(nil)` does not panic and behaves as
  `ax.MetadataFromContext(context.Background())`.
- `contract.MetadataFromContext`'s return value for any input is
  byte-for-byte unchanged by this feature; only its doc comment changes.
- The function is pure: it performs no I/O, starts no goroutine, and mutates
  no state — calling it any number of times with the same `ctx` at the same
  point in execution returns the same result.
