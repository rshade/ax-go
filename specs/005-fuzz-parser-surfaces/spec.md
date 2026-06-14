# Feature Specification: Fuzz Tests for Every Parser Surface

**Feature Branch**: `005-fuzz-parser-surfaces`

**Created**: 2026-06-13

**Status**: Draft

**Input**: User description: "GitHub Issue #4 — AGENTS.md mandates fuzz tests for every parser surface. Surfaces: Hujson config input, idempotency-key validation, error-envelope round-trip, and TRACEPARENT extraction. Seed corpora must be committed."

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Config Reader Is Safe Under Arbitrary Input (Priority: P1)

A maintainer merges a dependency upgrade that changes how the bounded
Hujson reader behaves under edge-case byte sequences. The CI fuzz gate
catches a panic before the release branch is cut.

**Why this priority**: `ParseConfig` is the primary entry point for all
Hujson config reads. The bounded-reader boundary is a DoS-resistance
guarantee (Principle V of the constitution). Any panic or hang here
could take down an adopting CLI in production.

**Independent Test**: Run `go test -run=^$ -fuzz=FuzzParseConfig -fuzztime=30s ./...`
on the main module and confirm zero panics for any combination of byte
sequence and byte-cap value, including inputs that exactly straddle the
cap.

**Acceptance Scenarios**:

1. **Given** an arbitrary byte sequence and an arbitrary byte count as
   the size cap, **When** `ParseConfig` is called, **Then** it must not
   panic, must not hang, and must return either a valid decoded value or
   a typed `*ax.Error` (never a raw `error`).
2. **Given** an input whose length exactly equals the size cap,
   **When** `ParseConfig` is called, **Then** it succeeds or returns a
   parse error — not an oversize error.
3. **Given** an input whose length exceeds the cap by one byte,
   **When** `ParseConfig` is called, **Then** it returns a
   `config_too_large` error with exit code 2.
4. **Given** a cap value below zero or above `MaxConfigBytesCeiling`,
   **When** `ParseConfig` is called, **Then** it returns a
   `config_max_bytes_invalid` error with exit code 2.
5. **Given** a nil `ParseConfigOption`, **When** `ParseConfig` is called,
   **Then** it returns a `config_option_invalid` error with exit code 2.

---

### User Story 2 — Idempotency Key Round-Trip Is Fuzz-Hardened (Priority: P2)

A user supplies an arbitrary `--idempotency-key`, which the library accepts
verbatim, stores in the request context, and surfaces in the machine-readable
output envelope. The fuzz harness confirms that this round-trip never panics and
never mutates the key — for any string, including empty strings, very long
strings, and strings with control characters — and that auto-generated keys
remain valid UUID v4.

> **Note**: there is no idempotency-key *validation* function today; user keys
> are taken as-is (`execute.go`). The fuzzed parser surface is therefore the
> context store/retrieve plus envelope-marshal round-trip, not a validator.

**Why this priority**: Idempotency keys are agent-safety primitives
(Principle IV). A crash in key generation or round-trip handling would
make every agent-driven operation non-retryable.

**Independent Test**: Run `go test -run=^$ -fuzz=FuzzIdempotencyKey -fuzztime=30s ./...`
and confirm zero panics. A standalone fuzz corpus of interesting
boundary inputs (empty string, max-length UUID, non-UUID strings,
Unicode) is committed under `testdata/fuzz/FuzzIdempotencyKey/`.

**Acceptance Scenarios**:

1. **Given** an arbitrary string passed as an idempotency key,
   **When** the key is stored in context and surfaced through the output
   envelope, **Then** no panic occurs, the key is returned unchanged, and
   the result is deterministic.
2. **Given** the output of `NewIdempotencyKey()`, **When** it is
   used as input to the fuzz harness, **Then** it satisfies all UUID v4
   format invariants (length, hyphens, version nibble, variant bits).

---

### User Story 3 — Error Envelope Round-Trip Preserves the Cause Chain (Priority: P3)

A developer using `ax.NewError` attaches a cause via `WithErrorCause`.
After marshal/unmarshal, they call `errors.As` on the result. The fuzz
harness ensures that for arbitrary error-code strings, messages, and
cause types, the cause chain is never silently dropped or corrupted, and
the JSON envelope shape never triggers a panic. A second, dedicated harness
(`FuzzErrorEnvelopeUnmarshal`) drives **arbitrary bytes** into the envelope's
`json.Unmarshal` path, proving that malformed or empty JSON never panics and
that any value which unmarshals cleanly re-serializes to a byte-for-byte stable
fixpoint (Principle II — determinism).

**Why this priority**: The error envelope is a public, stable-by-contract
API (ADR-0002, guarded by golden file). Its `Unwrap` path feeds
`errors.Is` / `errors.As` which adopting CLIs use for exit-code
classification. Silent breakage is invisible to agents.

**Independent Test**: Run `go test -run=^$ -fuzz=FuzzErrorEnvelope -fuzztime=30s ./...`
and confirm: no panic, JSON always parses back into an `*ax.Error`, and the
attached cause is reachable via `errors.As` on the in-process value *before*
serialization. Separately run
`go test -run=^$ -fuzz=FuzzErrorEnvelopeUnmarshal -fuzztime=30s ./...` and confirm
no panic for arbitrary bytes and a stable re-serialization fixpoint for inputs
that unmarshal cleanly.

**Acceptance Scenarios**:

1. **Given** arbitrary strings for error code, message, and cause text,
   **When** an `*ax.Error` is marshalled and the JSON is unmarshalled
   back, **Then** no panic occurs, the JSON is valid, and the error code
   and message round-trip exactly.
2. **Given** an `*ax.Error` with a cause attached via `WithErrorCause`,
   **When** it is serialized to JSON and deserialized, **Then** the
   deserialized value may not carry the cause (it is not serialized), but
   the in-process value's cause is reachable before serialization.
3. **Given** malformed JSON fed to an `*ax.Error` unmarshal path,
   **When** unmarshal is called, **Then** it returns a typed error, never
   panics, and the original envelope value is unchanged.

---

### User Story 4 — TRACEPARENT/TRACESTATE Extraction Has a Committed Seed Corpus (Priority: P4)

The existing `FuzzTraceparentExtraction` function already runs in CI.
A reviewer notes that it has no committed seed corpus, so the fuzzer
always starts from scratch. Committing known-interesting inputs (valid
W3C traceparent strings, version mismatches, all-zero IDs, truncated
headers) accelerates coverage for future CI runs.

**Why this priority**: The fuzz function exists; this story only adds the
seed corpus that is missing. It has no implementation work beyond
committing files to `testdata/fuzz/FuzzTraceparentExtraction/`.

**Independent Test**: `ls testdata/fuzz/FuzzTraceparentExtraction/` shows
one or more seed files; each is accepted by
`go test -run=FuzzTraceparentExtraction/` without error.

**Acceptance Scenarios**:

1. **Given** the committed seed corpus directory, **When** `go test` runs
   the named seed cases, **Then** every seed passes (no failures).
2. **Given** a new run with `-fuzz=FuzzTraceparentExtraction`, **When**
   the engine starts, **Then** it loads seeds from the committed corpus
   before generating new inputs.

---

### Edge Cases

- Input exactly at the size boundary (cap - 1, cap, cap + 1) for `FuzzParseConfig`.
- Nil options slice vs. a non-nil slice containing a nil entry for `ParseConfig`.
- Error-code strings that contain control characters, newlines, or Unicode for `FuzzErrorEnvelope`.
- Idempotency keys that resemble UUIDs but carry a wrong version/variant nibble — round-tripped unchanged (no validation or rejection exists).
- TRACEPARENT strings with correct length but invalid hex characters.
- A nil `error` interface AND a typed-nil `*ax.Error` passed into `WriteError` — both MUST be panic-free (interface-nil is a documented no-op).
- Empty, all-whitespace, or malformed JSON fed to the envelope unmarshal path (covered by `FuzzErrorEnvelopeUnmarshal`).
- Strings containing invalid UTF-8 (e.g., `\xf2`) as idempotency keys or error fields — `encoding/json` coerces these to U+FFFD, so byte-exact JSON round-trip is asserted only for valid-UTF-8 inputs; the context round-trip stays byte-exact.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A `FuzzParseConfig` function MUST exist and exercise `ParseConfig` with
  arbitrary byte sequences and arbitrary cap values, asserting no panics and no
  uncategorized raw errors.
- **FR-002**: `FuzzParseConfig` MUST verify the bounded-reader boundary: inputs equal
  to the cap succeed-or-parse-error; inputs one byte over the cap return
  `config_too_large` (exit code 2); invalid cap values return
  `config_max_bytes_invalid` (exit code 2).
- **FR-003**: A `FuzzIdempotencyKey` function MUST exist and exercise the
  idempotency-key context+envelope round-trip (`WithIdempotencyKey` →
  `IdempotencyKeyFromContext` → envelope marshal) with arbitrary string inputs,
  asserting no panics, deterministic output, and that the key is never mutated.
  It MUST also assert that auto-generated keys (`NewIdempotencyKey`) are valid
  UUID v4. No idempotency-key *validation* API exists or is introduced (see
  Assumptions; research D2).
- **FR-004**: A `FuzzErrorEnvelope` function MUST exist and exercise the
  build → marshal → unmarshal round-trip of `*ax.Error` with arbitrary field
  values, asserting no panics, valid JSON output, and error-code/message
  round-trip fidelity.
- **FR-005**: `FuzzErrorEnvelope` MUST verify that cause chains set via
  `WithErrorCause` are accessible via `errors.As` before serialization and that
  they do not appear in the marshalled JSON payload.
- **FR-006**: Seed corpora MUST be committed under
  `testdata/fuzz/<FuzzFuncName>/` for all five fuzz functions:
  `FuzzParseConfig`, `FuzzIdempotencyKey`, `FuzzErrorEnvelope`,
  `FuzzErrorEnvelopeUnmarshal`, and `FuzzTraceparentExtraction`.
- **FR-007**: Every seed file in every committed corpus MUST pass when run with
  `go test -run=<FuzzFuncName>/` (i.e., named corpus entries must not be failing
  cases). This is the requirement-level statement of SC-001's aggregate
  cold-replay outcome.
- **FR-008**: All five fuzz functions added or extended by this feature MUST be
  exercised with `go test -race ./...` (seed replay runs with the race
  detector), alongside the pre-existing `FuzzPatchConfig`.
- **FR-009**: A `FuzzErrorEnvelopeUnmarshal` function MUST exist and feed
  arbitrary bytes into `json.Unmarshal` of an `*ax.Error`, asserting no panic on
  any input (malformed, empty, or valid JSON) and that any input which unmarshals
  without error re-serializes to a byte-for-byte stable fixpoint
  (`marshal(unmarshal(marshal(unmarshal(b)))) == marshal(unmarshal(b))`),
  exercising the `context`/`suggestions` envelope fields. This closes US3
  acceptance scenario 3 and the empty/malformed-JSON edge case.

### Key Entities

- **`FuzzParseConfig`**: A `func(*testing.F)` in `config_fuzz_test.go` that
  exercises `ax.ParseConfig`, the bounded Hujson reader entry point.
- **`FuzzIdempotencyKey`**: A `func(*testing.F)` in `id_fuzz_test.go` that
  exercises the context+envelope round-trip (`ax.WithIdempotencyKey` →
  `ax.IdempotencyKeyFromContext` → `ax.NewEnvelope` marshal) and the
  `ax.NewIdempotencyKey` UUID-v4 generation invariant. No validation helper
  exists.
- **`FuzzErrorEnvelope`**: A `func(*testing.F)` in `error_fuzz_test.go` that
  exercises `ax.NewError`, `ax.WriteError`, and the envelope's
  build → marshal → unmarshal round-trip.
- **`FuzzErrorEnvelopeUnmarshal`**: A `func(*testing.F)` in `error_fuzz_test.go`
  that feeds arbitrary bytes into `json.Unmarshal` of an `*ax.Error`, asserting
  no panic and serialization-idempotence for inputs that unmarshal cleanly.
- **`FuzzTraceparentExtraction`**: Already exists in `telemetry_fuzz_test.go`;
  this feature adds its committed seed corpus.
- **Seed corpus**: One or more files per fuzz function under
  `testdata/fuzz/<FuzzFuncName>/`, each a Go fuzzing corpus entry in the
  standard format.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All five fuzz functions compile and replay their seed corpora
  without failures in a cold `go test ./...` run (verifiable in CI without
  `-fuzz` flag).
- **SC-002**: `go test -race ./...` remains fully green after this feature
  lands — no new failures introduced.
- **SC-003**: Each fuzz function has at least three committed seed entries
  covering valid, boundary, and invalid input categories.
- **SC-004**: `golangci-lint run` and `make doc-coverage` remain clean —
  every new exported or unexported symbol added by this feature carries a
  doc comment.
- **SC-005**: No panics occur when any fuzz function is run for at least
  30 seconds with `-fuzz=<name> -fuzztime=30s` in a clean environment.
- **SC-006**: The issue's five acceptance-criteria checkboxes are all
  verifiably satisfied (each fuzz function exists and seed corpora are
  committed).

## Assumptions

- Source inputs: GitHub issue #4 and governing ADRs ADR-0002
  (error-envelope-schema), ADR-0004 (trace-id-format), ADR-0007
  (id-strategy). ADR decisions are absorbed into `research.md` during
  planning; ADRs that are fully superseded by this feature are retired as
  the feature's final task.
- The existing `FuzzPatchConfig` (in `config_fuzz_test.go`) and
  `FuzzTraceparentExtraction` (in `telemetry_fuzz_test.go`) already satisfy
  their respective parser-surface mandates for the Hujson patch path and
  TRACEPARENT extraction. This feature adds the missing surfaces and seed
  corpora only.
- `FuzzParseConfig` targets `ax.ParseConfig` (bounded reader + Hujson decode),
  which is a distinct code path from `FuzzPatchConfig` (AST-preserving
  JSON-patch application).
- Seed corpora are committed in the standard Go fuzzing corpus format (one
  file per entry, `go test fuzz v1` header, one value per line) under
  `testdata/fuzz/<FuzzFuncName>/`.
- No new public API surface is introduced; all five fuzz functions (including
  the added `FuzzErrorEnvelopeUnmarshal`) are package-level test functions only.
- The error-envelope surface is covered by **two** focused fuzz targets — a
  build/round-trip target (`FuzzErrorEnvelope`) and a parse/unmarshal target
  (`FuzzErrorEnvelopeUnmarshal`) — following the idiomatic "one fuzz target per
  input space" practice rather than overloading a single function (research D3).
- ADR-0010 is referenced only in the originating issue and is **not present** in
  `docs/adr/` (assumed already retired); the three listed ADRs (0002, 0004, 0007)
  are the governing references and none is retired by this feature.
