# Phase 1 Data Model: Fuzz Tests for Every Parser Surface

**Feature**: `005-fuzz-parser-surfaces` | **Date**: 2026-06-13

This feature persists no runtime data (Principle VI). The "entities" are the
fuzz *input tuples* fed to each surface, the *invariants* each must uphold, and
the *committed corpus* fixture shape. They map to existing public API; no new
types are introduced.

## Entities

### Fuzz input tuple (per surface)

| Fuzz function | Input signature | Maps to call under test |
|---------------|-----------------|--------------------------|
| `FuzzParseConfig` | `(data []byte, maxBytes int64)` | `ParseConfig(ctx, bytes.NewReader(data), &map[string]any{}, WithMaxConfigBytes(maxBytes))` |
| `FuzzIdempotencyKey` | `(key string)` | `WithIdempotencyKey(ctx, key)` → `IdempotencyKeyFromContext` → `NewEnvelope(ctx, struct{}{})` → marshal/unmarshal |
| `FuzzErrorEnvelope` | `(code, message, cause string)` | `NewError(ctx, code, message, WithErrorCause(errors.New(cause)))` → marshal/unmarshal + `WriteError` |
| `FuzzErrorEnvelopeUnmarshal` | `(data []byte)` | `var e ax.Error; json.Unmarshal(data, &e)` → conditional re-marshal fixpoint |
| `FuzzTraceparentExtraction` | `(traceparent, tracestate string)` | `StartTelemetry(ctx, WithTelemetryEnv(envFromBoth))` → `TraceIDFromContext`/`SpanIDFromContext` |

### Invariant set (the assertions, by surface)

#### FuzzParseConfig

- **I1 (no-panic)**: the call returns; it never panics or hangs.
- **I2 (typed errors)**: every non-nil error satisfies `errors.As(err, new(*ax.Error))`.
- **I3 (oversize boundary)**: for `0 <= maxBytes <= MaxConfigBytesCeiling` and
  `int64(len(data)) > maxBytes` → error code `config_too_large`, exit 2.
- **I4 (valid-cap, in-bound size)**: for a valid cap with `len(data) <= maxBytes`
  → success OR `config_invalid` (exit 2); **never** `config_too_large`.
- **I5 (invalid cap)**: `maxBytes < 0` or `> MaxConfigBytesCeiling` →
  `config_max_bytes_invalid`, exit 2 (regardless of payload).
- **I6 (exit-code agreement)**: `ax.ErrorExitCode(err)` equals the envelope's
  `ExitCode()` for every returned `*ax.Error`.

#### FuzzIdempotencyKey

- **I1 (no-panic)**: round-trip + marshal never panic.
- **I2 (round-trip fidelity + empty-is-absent contract)**: for a **non-empty**
  `key`, `IdempotencyKeyFromContext(WithIdempotencyKey(ctx, key))` returns
  `(key, true)` and the marshalled `Metadata.IdempotencyKey` equals `key`. For an
  **empty** `key`, the retrieval returns `("", false)` — an empty key is treated
  as ABSENT by design (`context.go` returns `ok && key != ""`, matching
  `execute.go`'s "generate when none supplied"); the envelope then omits the key
  (`omitempty`). The harness compares the envelope against the canonical
  *retrieved* value, so both branches are covered without special envelope logic.
  The byte-exact guarantee *through a JSON round-trip* holds only for valid-UTF-8
  keys — `encoding/json` coerces invalid UTF-8 to U+FFFD (documented stdlib
  behavior) — so the post-JSON equality check is gated on `utf8.ValidString`. The
  context store/retrieve remains byte-exact for any string.
- **I3 (generation invariant)**: `NewIdempotencyKey()` parses as a UUID with
  `Version() == 4` (asserted once per fuzz invocation, input-independent).

#### FuzzErrorEnvelope

- **I1 (no-panic)**: `NewError` + `json.Marshal` + `json.Unmarshal` + `WriteError`
  never panic for any field values.
- **I2 (marshal-safe)**: `json.Marshal(e)` always succeeds and yields valid JSON.
- **I3 (exported round-trip)**: unmarshalling the JSON into a fresh `*ax.Error`
  reproduces `SchemaVersion` exactly, and reproduces `ErrorCode`/`Message`
  exactly for valid-UTF-8 inputs (gated on `utf8.ValidString` — `encoding/json`
  coerces invalid UTF-8 to U+FFFD).
- **I4 (cause reachability, pre-marshal)**: the attached sentinel cause is
  reachable via `errors.Is(e, sentinelCause)` on the in-process value. (Use
  `errors.Is`, not a direct `e.Unwrap() == cause` comparison — `errorlint`
  correctly flags identity comparison as wrapping-unsafe.)
- **I5 (cause not serialized)**: the marshalled JSON does not contain the cause
  text; the deserialized value's `Unwrap()` is nil.
- **I6 (WriteError shape & nil-safety)**: `WriteError(w, e)` emits exactly one
  valid JSON object terminated by a single `\n`; `WriteError(w, nil)`
  (interface-nil) is a no-op; `WriteError(w, (*Error)(nil))` (typed-nil) is
  panic-free. (Deterministic non-fuzz assertions — analysis finding U1.)

#### FuzzErrorEnvelopeUnmarshal

- **I1 (no-panic)**: `json.Unmarshal(data, &ax.Error{})` never panics for any
  bytes (malformed, empty, whitespace, valid).
- **I2 (marshal-safe on success)**: if unmarshal returns a nil error, the
  subsequent `json.Marshal` of the value succeeds.
- **I3 (serialization idempotence)**: for inputs that unmarshal cleanly, the
  re-serialized bytes reach a fixpoint —
  `marshal(unmarshal(marshal(unmarshal(data)))) == marshal(unmarshal(data))`.
  Byte-level comparison (not struct `DeepEqual`) avoids the `omitempty`
  empty-vs-nil false positive (research D7).

#### FuzzTraceparentExtraction

- **I1 (no-panic / no-error)**: `StartTelemetry` returns no error; `Shutdown` is
  clean for any `(traceparent, tracestate)` pair.
- **I2 (length invariant)**: `TraceIDFromContext` length == `len(ZeroTraceID)`
  (32); `SpanIDFromContext` length == `len(ZeroSpanID)` (16).

### Committed corpus entry (fixture)

- **What**: a single Go fuzzing corpus file under `testdata/fuzz/<FuncName>/`.
- **Format**: first line `go test fuzz v1`, then one line per positional fuzz
  argument in declared order, each a Go-typed literal.
- **Rules**:
  - Arg count and types MUST match the fuzz signature, else `go test` fails to
    load the corpus.
  - File names are arbitrary but SHOULD be descriptive (e.g.,
    `cap_boundary_exact`, `empty_input`, `non_uuid_key`).
  - ≥ 3 entries per function (SC-003), spanning valid / boundary / invalid.

## Required corpus coverage matrix

| Function | Valid | Boundary | Invalid |
|----------|-------|----------|---------|
| `FuzzParseConfig` | small valid Hujson under default cap | `len==cap`, `len==cap+1`, `len==cap-1` | `maxBytes<0`, `maxBytes>ceiling`, malformed Hujson |
| `FuzzIdempotencyKey` | a real UUID v4 string | empty string, very long string | non-UUID, control chars, Unicode |
| `FuzzErrorEnvelope` | normal code+message+cause | empty strings | control chars / newline in message, very long strings |
| `FuzzErrorEnvelopeUnmarshal` | a full valid envelope JSON | `{}`, `""`, whitespace | truncated/garbage bytes, deeply nested `context`, wrong field types |
| `FuzzTraceparentExtraction` | valid sampled traceparent + simple tracestate | all-zero IDs, version `00` | malformed traceparent, bad-hex, oversized tracestate |

## State transitions

None. Fuzzing is stateless: each input is independent, and no entity has a
lifecycle. This matches Principle VI (no persistent state in the library).
