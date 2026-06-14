# Contract: Fuzz Parser Surfaces

**Feature**: `005-fuzz-parser-surfaces` | **Date**: 2026-06-13

For a test-centric library feature, the "interface contract" is the set of fuzz
function signatures, the invariants each guarantees, and the committed corpus
format. These are the artifacts other contributors and CI depend on. No public
`ax` API changes; all entries below are `package ax` test functions.

---

## 1. `FuzzParseConfig` — `config_fuzz_test.go` (MODIFY)

```go
// FuzzParseConfig verifies ParseConfig never panics and always classifies its
// outcome under arbitrary byte input and arbitrary read caps, with special
// attention to the bounded-reader boundary (Principle V).
func FuzzParseConfig(f *testing.F)
```

- **Signature under fuzz**: `(data []byte, maxBytes int64)`
- **Call**: `ParseConfig(ctx, bytes.NewReader(data), &dst, WithMaxConfigBytes(maxBytes))`
  with `dst := map[string]any{}`.
- **Guarantees**: I1–I6 in data-model.md. Key boundary contract:
  - `len(data) > maxBytes` (valid cap) ⇒ `config_too_large`, exit 2
  - `maxBytes < 0 || maxBytes > MaxConfigBytesCeiling` ⇒ `config_max_bytes_invalid`, exit 2
  - otherwise ⇒ success or `config_invalid` (exit 2)
- **Corpus**: `testdata/fuzz/FuzzParseConfig/`
  - line 1 `[]byte("…")`, line 2 `int64(…)`
  - MUST include cap−1, cap, cap+1 straddle entries + an invalid-cap entry.

## 2. `FuzzIdempotencyKey` — `id_fuzz_test.go` (NEW)

```go
// FuzzIdempotencyKey verifies an arbitrary user-supplied idempotency key
// survives the context+envelope round-trip unchanged and that generated keys
// remain valid UUID v4 (ADR-0007).
func FuzzIdempotencyKey(f *testing.F)
```

- **Signature under fuzz**: `(key string)`
- **Call chain**: `WithIdempotencyKey(ctx, key)` → `IdempotencyKeyFromContext` →
  `NewEnvelope(ctx, struct{}{})` → `json.Marshal` → `json.Unmarshal`.
- **Guarantees**: I1–I3 in data-model.md. Round-trip fidelity for any string;
  generated-key UUID-v4 invariant asserted once per run.
- **Corpus**: `testdata/fuzz/FuzzIdempotencyKey/`
  - `string("…")` per entry: a real UUID v4, empty, non-UUID, Unicode, long.

## 3. `FuzzErrorEnvelope` — `error_fuzz_test.go` (NEW)

```go
// FuzzErrorEnvelope verifies ax.Error marshal/unmarshal is panic-free, exported
// fields round-trip, and the WithErrorCause chain is reachable in-process but
// never serialized (ADR-0002).
func FuzzErrorEnvelope(f *testing.F)
```

- **Signature under fuzz**: `(code, message, cause string)`
- **Call**: `NewError(ctx, code, message, WithErrorCause(errors.New(cause)),
  WithErrorExitCode(ExitValidation))` → marshal → unmarshal into fresh
  `*ax.Error`; also `WriteError(buf, e)`.
- **Guarantees**: I1–I6 in data-model.md. Cause reachable via `errors.Is`/`Unwrap`
  **before** marshal; absent from JSON and `nil` after unmarshal; `WriteError`
  emits one JSON line + `\n`, with interface-nil a no-op and typed-nil
  `(*Error)(nil)` panic-free. The arbitrary-bytes unmarshal path is a separate
  target — see §3b.
- **Corpus**: `testdata/fuzz/FuzzErrorEnvelope/`
  - three `string("…")` lines per entry: normal, empty, control-char/newline,
    long-string variants.

## 3b. `FuzzErrorEnvelopeUnmarshal` — `error_fuzz_test.go` (NEW)

```go
// FuzzErrorEnvelopeUnmarshal verifies ax.Error's json.Unmarshal path never
// panics on arbitrary bytes and that inputs which unmarshal cleanly re-serialize
// to a byte-for-byte stable fixpoint (Principle II; ADR-0002).
func FuzzErrorEnvelopeUnmarshal(f *testing.F)
```

- **Signature under fuzz**: `(data []byte)`
- **Call**: `var e ax.Error; err := json.Unmarshal(data, &e)`; on `err == nil`,
  re-marshal and assert the byte-level fixpoint.
- **Guarantees**: I1–I3 in data-model.md (no panic; marshal-safe on success;
  serialization idempotence via byte-level fixpoint — see research D7 for why
  struct `DeepEqual` is the wrong comparison given `omitempty`).
- **Corpus**: `testdata/fuzz/FuzzErrorEnvelopeUnmarshal/`
  - one `[]byte("…")` per entry: a full valid envelope, `{}`, empty/whitespace,
    truncated/garbage bytes, deeply nested `context`.

## 4. `FuzzTraceparentExtraction` — `telemetry_fuzz_test.go` (MODIFY)

```go
// FuzzTraceparentExtraction verifies trace/span-ID extraction never panics and
// always yields canonical-length IDs for arbitrary TRACEPARENT and TRACESTATE
// headers (W3C Trace Context, ADR-0004).
func FuzzTraceparentExtraction(f *testing.F)
```

- **Signature under fuzz**: `(traceparent, tracestate string)` *(extended from the
  current single `traceparent string`)*
- **Call**: `StartTelemetry(ctx, WithTelemetryEnv(func(k string) string { ... }))`
  returning `traceparent` for `"TRACEPARENT"` and `tracestate` for `"TRACESTATE"`.
- **Guarantees**: I1–I2 in data-model.md. `StartTelemetry` error-free, `Shutdown`
  clean, ID lengths == `ZeroTraceID`/`ZeroSpanID` lengths.
- **Corpus**: `testdata/fuzz/FuzzTraceparentExtraction/`
  - two `string("…")` lines per entry: valid pair, all-zero/version-00, malformed.

---

## Cross-cutting contract guarantees

- **No panic on any input** for all five functions (the central invariant).
- **No new public `ax` API**; `go test ./...` (no `-fuzz`) replays every committed
  corpus entry; `go test -race ./...` stays green (SC-002).
- **Corpus type-match**: each `testdata/fuzz/<Func>/` entry's arg types match the
  signature exactly, or `go test` fails to load it.
- **Lint/doc gates unaffected**: `golangci-lint run` and `make doc-coverage`
  remain clean (fuzz funcs are test-only; doc comments are encouraged, not gated).

## Verification commands

```bash
# Seed replay (no fuzzing), race-enabled — the CI contract:
go test -race ./...

# Targeted exploration per surface (manual / nightly):
go test -run=^$ -fuzz=FuzzParseConfig            -fuzztime=30s .
go test -run=^$ -fuzz=FuzzIdempotencyKey         -fuzztime=30s .
go test -run=^$ -fuzz=FuzzErrorEnvelope          -fuzztime=30s .
go test -run=^$ -fuzz=FuzzErrorEnvelopeUnmarshal -fuzztime=30s .
go test -run=^$ -fuzz=FuzzTraceparentExtraction  -fuzztime=30s .

# Replay only the committed corpus for one function:
go test -run=FuzzParseConfig/ .
```
