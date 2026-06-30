# Data Model: Error-envelope recovery & remediation fields

**Feature**: `013-error-recovery-fields` | **Date**: 2026-06-29

## Entity: Error envelope (`contract.Error`, aliased `ax.Error`)

The error envelope is unchanged except for two appended optional attributes. The
source of truth is `contract/error.go`; the root `ax.Error` is a type alias and
inherits the shape.

### Fields (delta only — existing fields unchanged)

| Field (Go) | JSON key | Type | Required? | JSON tag | Notes |
|------------|----------|------|-----------|----------|-------|
| `Retryable` | `retryable` | `*bool` | Optional | `retryable,omitempty` | Tri-state: `true` (safe to retry), `false` (do NOT retry), `nil`/absent (unspecified). |
| `RetryAfterSeconds` | `retry_after_seconds` | `int64` | Optional | `retry_after_seconds,omitempty` | Relative wait, in whole seconds, before a retry. Never absolute/wall-clock. `≥ 0`; negative input normalized to unset (D3). |

Both fields are **appended after** `Suggestions` in declaration order so existing
JSON key order is preserved (research D6).

### Resulting `contract.Error` shape (illustrative)

```go
type Error struct {
    ErrorCode         string         `json:"error_code"`
    Message           string         `json:"message"`
    TraceID           string         `json:"trace_id"`
    Tool              string         `json:"tool"`
    Version           string         `json:"version"`
    SchemaVersion     string         `json:"schema_version"`
    ActionableFix     string         `json:"actionable_fix,omitempty"`
    Context           map[string]any `json:"context,omitempty"`
    Suggestions       []string       `json:"suggestions,omitempty"`
    Retryable         *bool          `json:"retryable,omitempty"`           // NEW
    RetryAfterSeconds int64          `json:"retry_after_seconds,omitempty"` // NEW

    exitCode int
    cause    error
}
```

## Validation & normalization rules

| Rule | Where | Behavior |
|------|-------|----------|
| **VR-1** Retry-safety tri-state | `WithRetryable(b bool)` | Stores `&b`; absence of the option leaves `Retryable == nil` (unspecified). |
| **VR-2** Non-negative backoff | `WithRetryAfterSeconds(n int64)` | If `n < 0`, leave field unset (do not store the negative). If `n ≥ 0`, store `n`. |
| **VR-3** Determinism | struct/JSON | No wall-clock read anywhere; value is producer-supplied and relative. |
| **VR-4** Default invariance | `omitempty` | When neither option is supplied, marshaled bytes are identical to the pre-feature envelope. |
| **VR-5** No required-field change | struct | Required set stays `{error_code, message, trace_id, tool, version, schema_version}`; `schema_version` stays `1.0.0`. |

## Invariants preserved (no change)

- `Error.Error()` returns `Message`.
- `Error.ExitCode()` / `ErrorExitCode(err)` mapping is unaffected by the new
  fields.
- `Error.Unwrap()` still returns the `cause` set via `WithErrorCause`;
  `errors.Is`/`errors.As` reachability is unchanged.
- Root `ax.Error` output is byte-identical to isolated `contract.Error` output
  (guarded by `TestRootErrorEnvelopeMatchesIsolatedContractShape`).

## State / lifecycle

No state transitions — the envelope is an immutable value constructed via
`NewError(ctx, code, message, opts...)` and written once via `WriteError`.
