# Phase 0 Research: Fuzz Tests for Every Parser Surface

**Feature**: `005-fuzz-parser-surfaces` | **Date**: 2026-06-13

This document resolves the feature's open decisions and records the invariants
absorbed from the **referenced** governing ADRs (0002, 0004, 0007). No
`NEEDS CLARIFICATION` markers remain.

## Current-state diagnosis

The issue states "None exist." That is not strictly accurate, and the plan is
scoped to the gap:

| Parser surface | Current state | Action |
|----------------|---------------|--------|
| Hujson AST patch (`PatchConfig`) | `FuzzPatchConfig` exists (`config_fuzz_test.go`) | None — already covered |
| Hujson bounded read (`ParseConfig`) | **No fuzz** | **Add `FuzzParseConfig`** |
| Idempotency-key round-trip | **No fuzz** | **Add `FuzzIdempotencyKey`** |
| Error-envelope build/round-trip | **No fuzz** | **Add `FuzzErrorEnvelope`** |
| Error-envelope arbitrary-bytes unmarshal | **No fuzz** | **Add `FuzzErrorEnvelopeUnmarshal`** |
| `TRACEPARENT` extraction | `FuzzTraceparentExtraction` exists, **TRACEPARENT only, no committed corpus** | **Extend to `TRACESTATE`; commit corpus** |
| Committed seed corpora | **None** for any function | **Add `testdata/fuzz/<Func>/` for all five** |

`FuzzPatchConfig` and `FuzzParseConfig` are deliberately *separate*: `ParseConfig`
exercises the bounded reader (`internalconfig.ReadBounded`) plus Hujson→struct
decode, while `PatchConfig` exercises the AST-preserving RFC-6902 patch path.
Both flow through the same cap logic but diverge after the read.

## Decisions

### D1 — `FuzzParseConfig` fuzzes both the byte payload and the cap

- **Decision**: Signature `func(t *testing.T, data []byte, maxBytes int64)`.
  Call `ParseConfig(ctx, bytes.NewReader(data), &dst, WithMaxConfigBytes(maxBytes))`
  where `dst` is a permissive shape (`map[string]any`) so decode reaches real
  Hujson parsing rather than failing instantly on type mismatch.
- **Invariants asserted** (any input):
  - Never panics; never hangs (the cap guarantees a bounded read — Principle V).
  - Every non-nil error is a `*ax.Error` reachable via `errors.As` (no raw,
    unclassified errors escape) — *except* the documented `*json.InvalidUnmarshalError`
    caller-misuse case, which cannot occur here because `dst` is always a valid
    non-nil pointer.
  - **Boundary check** (the DoS guarantee): when `0 <= maxBytes <= MaxConfigBytesCeiling`,
    if `int64(len(data)) > maxBytes` the error MUST classify as `config_too_large`
    with exit code 2; otherwise the call succeeds or returns `config_invalid`
    (exit 2) — never `config_too_large`.
  - When `maxBytes < 0` or `maxBytes > MaxConfigBytesCeiling`, the error MUST be
    `config_max_bytes_invalid`, exit code 2.
- **Rationale**: The bounded reader is the single most safety-critical parser
  surface (oversize input must be a validation error, never an OOM). Fuzzing the
  cap *and* the payload together is the only way to exercise the cap−1 / cap /
  cap+1 straddle that `ReadBounded` keys on.
- **Alternatives rejected**: (a) Fuzz only the payload at the default cap — misses
  the invalid-cap and boundary classes. (b) Use a typed struct `dst` — narrows
  decode coverage; `map[string]any` reaches more of the Hujson grammar.

### D2 — `FuzzIdempotencyKey` fuzzes the round-trip, not a (non-existent) validator

- **Decision**: Signature `func(t *testing.T, key string)`. There is **no
  idempotency-key validation function** today: `execute.go` accepts a user-supplied
  `--idempotency-key` verbatim and only *generates* a UUID v4 when the flag is
  absent (`execute.go:174-176`). The real parser/round-trip surface is therefore:
  `WithIdempotencyKey(ctx, key)` → `IdempotencyKeyFromContext(ctx)` →
  `NewEnvelope(ctx, data)` → `json.Marshal` of the resulting `Metadata`.
- **Invariants asserted**:
  - Round-trip fidelity: a **non-empty** key stored via `WithIdempotencyKey` is
    returned byte-identical by `IdempotencyKeyFromContext` and appears unchanged
    in `Envelope.Meta.IdempotencyKey` after marshal/unmarshal — for control
    chars, Unicode, very long strings, etc. An **empty** key is treated as
    ABSENT: `IdempotencyKeyFromContext` returns `("", false)` because
    `context.go` returns `ok && key != ""` (matching `execute.go`'s
    generate-when-empty behavior), and the envelope omits the field. The fuzz
    harness asserts the envelope matches the canonical retrieved value, covering
    both branches.
  - No panic; the envelope JSON always marshals and unmarshals.
  - **Generation invariant** (separate, not fuzz-input-dependent): the harness
    also asserts once per run that `NewIdempotencyKey()` parses as a UUID and has
    `Version() == 4` (ADR-0007), guarding against regressions in the generator.
- **Rationale**: Keeps the feature test-only with no new public API (spec
  assumption). The honest "parser surface" for a user-supplied key is the
  context+envelope round-trip it actually travels through.
- **Alternatives rejected**: Introduce a `ValidateIdempotencyKey`/`ParseIdempotencyKey`
  function and fuzz it. Rejected — that is a new public API *and* a runtime
  behavior change (rejecting keys previously accepted), which must be its own
  constitution-governed feature, not smuggled in under "add fuzz tests."

### D3 — `FuzzErrorEnvelope` asserts marshal-safety + exported-field round-trip + pre-marshal cause reachability

- **Decision**: Signature `func(t *testing.T, code, message, cause string)`.
  Build `e := NewError(ctx, code, message, WithErrorCause(errors.New(cause)),
  WithErrorExitCode(...))`, marshal to JSON, unmarshal into a fresh `*ax.Error`.
- **Invariants asserted**:
  - No panic for any field values; `json.Marshal` always succeeds; the produced
    JSON is valid and re-parses into an `*ax.Error`.
  - Exported fields round-trip exactly: `SchemaVersion` always; `ErrorCode`/
    `Message` for valid-UTF-8 inputs only (`encoding/json` coerces invalid UTF-8
    to U+FFFD, so byte-exact round-trip is gated on `utf8.ValidString` — a real
    contract the fuzzer surfaced, not an ax defect).
  - **Cause-chain semantics**: `errors.As(e, &target)` reaches the attached cause
    on the *in-process* value *before* serialization; the marshalled JSON does
    **not** contain the cause text (it is intentionally non-serialized — see
    `WithErrorCause` doc). The deserialized value carries no cause. This matches
    spec US3 acceptance scenario 2.
  - `WriteError(w, e)` writes exactly one line of valid JSON + `\n`;
    `WriteError(w, nil)` (interface-nil) is a no-op; and
    `WriteError(w, (*Error)(nil))` (typed-nil) is panic-free. The last two are
    deterministic non-fuzz assertions that close the `WriteError` edge case
    without blessing the typed-nil output shape, which is pre-existing behavior
    owned by `error.go` (analysis finding U1).
  - The **arbitrary-bytes unmarshal** path (malformed/empty JSON → `*ax.Error`)
    is covered by a *separate* target, `FuzzErrorEnvelopeUnmarshal` — see D7.
- **Rationale**: The envelope is public, stable-by-contract (ADR-0002, golden
  file). The `Unwrap` path feeds adopting CLIs' `errors.Is`/`errors.As`
  exit-code classification, so silent breakage is invisible to agents.
- **Alternatives rejected**: Asserting the cause survives JSON round-trip —
  factually wrong; `ax.Error` has no custom `MarshalJSON` and `cause` is
  unexported by design. "Preserves the chain" can only mean the in-process path.

### D4 — Extend `FuzzTraceparentExtraction` to also fuzz `TRACESTATE`

- **Decision**: Change signature from `func(t, traceparent string)` to
  `func(t, traceparent, tracestate string)`; inject both via `WithTelemetryEnv`.
  Keep all existing assertions (extracted trace/span IDs have correct lengths,
  no error from `StartTelemetry`, clean `Shutdown`).
- **Rationale**: The issue acceptance criterion is explicitly
  "TRACEPARENT/TRACESTATE." `TRACESTATE` is a second untrusted header the W3C
  propagator parses; fuzzing it closes the stated gap. Test-file-only change.
- **Alternatives rejected**: Leave it TRACEPARENT-only — fails the acceptance
  criterion.

### D5 — Seed corpora are committed as `testdata/fuzz/<Func>/` files, in addition to `f.Add` seeds

- **Decision**: Each fuzz function keeps in-code `f.Add(...)` seeds (fast, local,
  reviewable) AND ships hand-authored corpus files under
  `testdata/fuzz/<FuncName>/` in the `go test fuzz v1` format, one entry per
  file, with all positional args in signature order and matching types.
- **Rationale**: Go's `f.Add` seeds are not persisted to `testdata/fuzz/`; only
  engine-discovered crashers are. The issue requires *committed* corpora, so they
  must be authored explicitly. Committed entries replay during normal
  `go test -race ./...` (no `-fuzz` flag), giving permanent regression coverage
  for the boundary/edge cases without a fuzzing run.
- **Corpus arg types** (must match exactly, or `go test` errors on load):
  - `FuzzParseConfig`: line 1 `[]byte("…")`, line 2 `int64(…)`
  - `FuzzIdempotencyKey`: `string("…")`
  - `FuzzErrorEnvelope`: three `string("…")` lines
  - `FuzzErrorEnvelopeUnmarshal`: one `[]byte("…")` line
  - `FuzzTraceparentExtraction`: two `string("…")` lines
- **Minimum coverage** (SC-003): ≥ 3 entries per function spanning valid /
  boundary / invalid categories. `FuzzParseConfig` MUST include the cap−1, cap,
  and cap+1 straddle as named entries.

### D6 — Doc comments on fuzz functions: encouraged, not gated

- **Decision**: Give each new fuzz function a contract-style doc comment (like the
  existing `FuzzPatchConfig`) describing the surface, invariants, and why
  non-panic is the success condition.
- **Rationale**: Constitution VII treats docs as contract. `godoclint`'s
  `require-doc` does **not** gate fuzz functions (the existing
  `FuzzTraceparentExtraction` ships without a doc comment and lint is clean), and
  `make doc-coverage` scans only the public `ax` API surface, not `_test.go`
  functions — so this is a quality choice, not a hard gate.

### D7 — A dedicated `FuzzErrorEnvelopeUnmarshal` target for the arbitrary-bytes parse path

- **Decision**: Add a second fuzz function `func(t *testing.T, data []byte)` in
  `error_fuzz_test.go`: `var e ax.Error; err := json.Unmarshal(data, &e)`.
- **Invariants asserted**:
  - **No panic** for any bytes (malformed, empty, whitespace, or valid JSON).
  - **Serialization idempotence** (the meaningful, non-trivial invariant): if
    `err == nil`, then `b1 := json.Marshal(&e)` succeeds and the byte-level
    fixpoint holds — `json.Marshal(unmarshal(b1)) == b1`. This exercises the
    `Context map[string]any` and `Suggestions []string` fields (arbitrary JSON
    objects land in `Context`) and directly reinforces Principle II
    (byte-identical output).
- **Why a byte-level fixpoint, not struct equality**: the naive
  "unmarshal → marshal → unmarshal must `DeepEqual` the first struct" invariant
  is **wrong** here. `omitempty` on `Suggestions`/`Context` means an
  empty-but-non-nil slice/map serializes away, so the second unmarshal yields a
  `nil` field that does not `DeepEqual` the first empty-non-nil field — a false
  positive. Comparing re-serialized **bytes** sidesteps the empty-vs-nil
  distinction and is the determinism contract that actually matters.
- **Why a separate function (not a 4th arg on `FuzzErrorEnvelope`)**: idiomatic
  Go fuzzing uses one target per input space. A 3-string *build* path and a
  raw-bytes *parse* path have disjoint mutation spaces; merging them wastes the
  engine's budget and muddies crash attribution. Separate targets also get
  independent corpora.
- **Why this is not "AI slop" (fuzzing the stdlib)**: today `ax.Error` has no
  custom `UnmarshalJSON`, so the no-panic arm is cheap insurance; but the
  idempotence arm exercises real `ax`-owned behavior (struct field tags,
  `omitempty`, the `Context` map) and becomes a true regression guard the moment
  a custom (Un)MarshalJSON is added to the envelope.
- **Rationale**: closes US3 acceptance scenario 3 and the empty/malformed-JSON
  edge case (analysis finding G1) rather than descoping them.
- **Alternatives rejected**: (a) descope US3 AS#3 — leaves a promised acceptance
  scenario uncovered; (b) bolt a raw-`[]byte` arm onto `FuzzErrorEnvelope` —
  non-idiomatic, dilutes both corpora.

## Decision Records Referenced

These ADRs are **referenced for the invariants the fuzz tests assert**. None is
retired by this feature (no implementation is superseded by adding tests; each
ADR is owned by the feature that built its code). No ADR file is deleted, so the
"MUST NOT delete until absorbed" rule is trivially honored and no retirement task
is created.

### ADR-0002 — JSON Error Envelope Schema (ACCEPTED 2026-05-28)

- **Decision**: Rich envelope `{error_code, message, trace_id, tool, version,
  schema_version, actionable_fix?, context?, suggestions?}` on `stderr`;
  `stdout` carries success payload or nothing.
- **Invariant the fuzz test asserts**: marshal/unmarshal of the envelope is
  panic-free and the documented exported fields round-trip; the cause attached
  via `WithErrorCause` is reachable only in-process (never serialized).
- **Consequence for this feature**: drives `FuzzErrorEnvelope` (D3) and
  `FuzzErrorEnvelopeUnmarshal` (D7). Envelope shape itself remains
  golden-file-guarded by `error_envelope.golden.json` (unchanged).

### ADR-0004 — Trace ID Format (ACCEPTED; already absorbed-file-retained by feature 004)

- **Decision**: W3C Trace Context — 32-hex-char trace IDs, 16-hex-char span IDs;
  zero values (`ZeroTraceID`/`ZeroSpanID`) for the no-active-span case.
- **Invariant the fuzz test asserts**: for any `TRACEPARENT`/`TRACESTATE` input,
  `TraceIDFromContext`/`SpanIDFromContext` return strings of the canonical
  lengths and never panic.
- **Consequence for this feature**: drives the extended
  `FuzzTraceparentExtraction` (D4). File already retained per feature 004; this
  feature adds no new claim over it.

### ADR-0007 — ID Strategy (ACCEPTED 2026-05-28)

- **Decision**: OTel IDs for observability; **UUID v4 for idempotency keys**;
  UUID v7 for entity/resource IDs; the three categories MUST NEVER be
  interchanged.
- **Invariant the fuzz test asserts**: `NewIdempotencyKey()` output is a valid
  UUID with `Version() == 4`; an arbitrary user-supplied key survives the
  context+envelope round-trip unchanged (the library does not silently mutate or
  reinterpret it).
- **Consequence for this feature**: drives `FuzzIdempotencyKey` (D2).

## Open questions

None. All decisions resolved; no `NEEDS CLARIFICATION` remain.
