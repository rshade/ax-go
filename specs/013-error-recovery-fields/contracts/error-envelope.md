# Contract: `ax.Error` envelope — recovery fields delta

**Feature**: `013-error-recovery-fields` | **Schema version**: `1.0.0` (unchanged)

This contract specifies the additive delta to the public `ax.Error` JSON
envelope. The envelope is public API (Constitution III) and golden-guarded.

## JSON shape (new optional keys)

```jsonc
{
  // ...all existing keys unchanged, in existing order...
  "actionable_fix": "…",   // existing, optional
  "context": { },          // existing, optional
  "suggestions": ["…"],    // existing, optional

  "retryable": true,            // NEW optional — bool. Omitted when unspecified.
  "retry_after_seconds": 30     // NEW optional — non-negative integer seconds. Omitted when unset.
}
```

### Key: `retryable`

- **Type**: boolean.
- **Semantics**: `true` = a naive re-run of the same command is safe;
  `false` = do NOT retry (permanent failure); **absent** = no information.
- **Determinism**: producer-supplied; no wall-clock.
- **Consumer contract**: treat absent as "unknown" — do not assume `false`.

### Key: `retry_after_seconds`

- **Type**: integer (JSON number, no fractional part), `≥ 0`.
- **Unit**: seconds, **relative** to the moment of the error (delta-seconds, à la
  HTTP `Retry-After`). Never an absolute timestamp.
- **Semantics**: advisory minimum wait before retrying. Meaningful only when
  `retryable` is `true`; if `retryable` is `false`/absent, a consumer ignores it.
- **Absence**: omitted when the producer supplies no backoff or supplies a
  negative value (normalized to unset).

## Backward compatibility

- Adding optional keys is additive; `schema_version` stays `1.0.0`.
- A producer that sets neither key emits an envelope **byte-identical** to the
  pre-feature output. Existing consumers ignore unknown keys.

## Golden fixtures

| Fixture | Asserts |
|---------|---------|
| `testdata/error_envelope.golden.json` (existing, **unchanged**) | Default shape with no recovery fields stays byte-identical → FR-005/FR-006. |
| `testdata/error_recovery_envelope.golden.json` (**new**) | Populated `retryable:true` + `retry_after_seconds:N` shape and key order → FR-011. |

### New fixture content (canonical)

```json
{"error_code":"network_timeout","message":"upstream timed out","trace_id":"00000000000000000000000000000000","tool":"app","version":"v0.1.0","schema_version":"1.0.0","actionable_fix":"retry the request","suggestions":["check upstream status"],"retryable":true,"retry_after_seconds":30}
```

> Exact bytes are finalized by the test that produces them; this is the intended
> shape and key order (`…,suggestions,retryable,retry_after_seconds`).

## Exit-code & error-chain contract (unchanged, re-asserted)

- `retryable` / `retry_after_seconds` do **not** influence `ErrorExitCode`.
- A `network_timeout` envelope still maps to exit `3` via `WithErrorExitCode`
  / context-sentinel rules, independent of `retryable`.
- `errors.Is`/`errors.As` against any `WithErrorCause` chain are unaffected.
