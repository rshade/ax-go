# Research: Output-Determinism Test Harness

**Feature branch**: `006-output-determinism-harness`
**Date**: 2026-06-14

---

## Decision Records Absorbed

### ADR-0011: Output Payload Format — Strict JSON (and NDJSON for streams)

**ADR file**: `docs/adr/0011-output-payload-json.md` (retired in this feature; see T018)
**ADR status at absorption**: ACCEPTED — 2026-05-28

#### Decision

Both bounded and streaming output modes are supported, command-declared:

- Single-shot results: strict minified JSON envelope on `stdout`.
- Unbounded / large result sets: NDJSON (one JSON object per line on `stdout`).
- Each command declares its output mode in `__schema` (ADR-0003).
- Default serializer: `encoding/json` from the standard library.

#### Considered Alternatives

| Option | Outcome |
|--------|---------|
| A. Strict minified JSON only | Rejected for unbounded streams: forces downstream to buffer entire output before parsing. |
| B. NDJSON only | Rejected: single-record and small-set results are better expressed as one self-describing document. |
| C. Pretty-printed JSON | Rejected for machine path; `--format=human` is the only gate. |
| D. YAML / TOML | Rejected: non-standard for agents. |
| E. Protocol Buffers | Rejected: agent-unfriendly without schema distribution for the CLI surface. |

#### Consequences for this feature

- The harness MUST support both comparison modes (bounded JSON and NDJSON) to cover the full output contract.
- NDJSON lines are tested line-by-line; trailing blank lines after the final `\n` are normalized.
- The error envelope (`ax.Error`) is always strict JSON on `stderr` and is NOT part of the harness's
  success-path comparison — the harness reads `stdout` only (FR-010).
- The integration example's `stream` subcommand is the primary NDJSON test target (US-2, FR-007).

---

## Research Decisions

### 1. Masking Strategy

**Decision**: Regex replacement on raw `[]byte`, applied before any comparison.

**Rationale**:
- Regex on raw bytes is O(n) and preserves the exact byte sequence everywhere except the three masked fields.
- JSON deserialization → re-serialization is tempting but risky: if the `data` payload ever contains a
  `map[string]any` at some level, Go's `encoding/json` does not guarantee key-ordering on maps, which
  would produce spurious diff failures. The harness should not amplify or introduce non-determinism.
- The three non-deterministic fields (`trace_id`, `span_id`, `idempotency_key`) always appear as
  JSON string values in the `meta` object, so simple regex patterns are reliable:
  - `"trace_id":"<anything>"` → `"trace_id":"MASKED"`
  - `"span_id":"<anything>"` → `"span_id":"MASKED"`
  - `"idempotency_key":"<anything>"` → `"idempotency_key":"MASKED"`
- The patterns use `"([^"]*)"`  to match the value (no escaped-quote handling needed for UUIDs and hex IDs).

**Alternatives considered**:
- *JSON round-trip masking*: Unmarshal → zero out fields → re-marshal. Rejected: map ordering hazard.
- *String split on known separators*: brittle; breaks with field reordering.

---

### 2. Stable Sentinel Value

**Decision**: `"MASKED"` (the JSON string `"MASKED"`).

**Rationale**:
- Visually unmistakable in a diff output.
- Short: keeps the masked output a fixed, compact length regardless of original value length. Wait —
  actually, because the original values vary in length, the masked output will vary slightly in length
  too. The comparison is after both runs are masked; both masked outputs replace the same field with
  the same sentinel, so the resulting byte sequence from both runs is byte-identical. Length variation
  between the original and masked form is irrelevant; what matters is the two masked forms match.
- `"MASKED"` is already a valid JSON string, so the masked output is still valid JSON.

**Alternatives considered**:
- `"<REDACTED>"`: also valid but slightly longer; similarly fine.
- UUID-shaped placeholder `"00000000-0000-0000-0000-000000000000"`: might cause a future test to
  inadvertently accept it as a real UUID.

---

### 3. Helper Package Location

**Decision**: `internal/testutil/determinism.go` — package `testutil`, non-test file.

**Rationale**:
- Go allows importing `testing` in non-test files; the build system does not include test
  packages in production binaries when they are only imported from `_test.go` files.
- A non-test file in `internal/testutil/` is importable from any `_test.go` file in the module
  (as long as the caller is within `github.com/rshade/ax-go`). This satisfies FR-008.
- A `_test.go` file in `internal/testutil/` would be package-internal only and not importable
  elsewhere — violates FR-008.
- `examples/integration/determinism_test.go` can import `internal/testutil` and also call the
  unexported `run()` function from `package main`, satisfying the spec assumption about in-process
  invocation.

**Alternatives considered**:
- *Public `axtest/` package at root*: exposes test utilities as part of the public API — violates
  Constitution Principle VI (library scope).
- *All logic in `examples/integration/`*: violates FR-008 (not reusable).
- *Only in `examples/integration/` with a `TestMain` re-export*: fragile and non-idiomatic.

---

### 4. NDJSON Comparison Design

**Decision**: Split on `\n`, strip empty trailing element, mask each line, compare count and lines.

**Rationale**:
- `WriteJSONLine` always appends `\n` after each JSON object (via `WriteJSON`). A stream of N items
  produces a `stdout` that ends with `\n`, so `strings.Split(s, "\n")` yields N+1 elements where the
  last is empty. Stripping the trailing empty string before comparison prevents false mismatches.
- Line-by-line comparison localizes the first divergence to a specific line index (FR-004 for NDJSON).
- Line count mismatch is reported before any line-content comparison (US-2 Scenario 2).

---

### 5. Timestamp Validation Approach

**Decision**: Deserialize the raw `stdout` bytes into `map[string]any` recursively; for every
string value, attempt `time.Parse(time.RFC3339, v)` and additionally assert the zone is UTC (either
`Z` designator or `+00:00` offset). If the parse succeeds but the zone is not UTC, the check fails.

**Rationale**:
- The `Metadata` struct fields (`trace_id`, `span_id`, `idempotency_key`) are not timestamps.
  The integration example's `helloPayload`, `streamPayload`, and `patchConfigPayload` currently carry
  no timestamp fields. The check trivially passes for the current integration example — which is
  exactly the expected baseline (SC-003: "passes for all envelopes emitted by the current integration
  example").
- A recursive string scan catches any future timestamp field added to the data payload, making the
  harness a forward-looking guard (SC-003).
- Using `time.RFC3339` ensures nanosecond variants (`time.RFC3339Nano`) are also covered by the
  same parse call.

**Alternatives considered**:
- *Static field enumeration*: fragile; misses future timestamp fields.
- *Regex on "date-like" strings*: false positives on non-date strings.

---

### 6. Fully-Typed Envelope Check (FR-006)

**Decision**: The check is implemented by having the test call `json.Unmarshal` into a
strongly-typed `ax.Envelope[T]` concrete struct. If unmarshal succeeds without error, the shape is
fully typed. The harness helper accepts a target type via a callback: `func UnmarshalCheck[T any](t
testing.TB, data []byte)`.

**Rationale**:
- Go generics allow the harness to be generic over the payload type while remaining type-safe.
- If the envelope were using `map[string]any` for root-level fields, it would not unmarshal cleanly
  into `Envelope[T]` (field names would be lost or misrouted). The typed unmarshal is the natural
  Go test for FR-006.
- The harness documents the intent so reviewers understand why this is the check. For the `meta`
  fields, `Metadata` is already a concrete struct; for `data`, the caller provides the type.
- Note: `ax.Error.Context` uses `map[string]any` — but the Error envelope is on `stderr`, not
  `stdout`. The `stdout` success envelope is fully typed.

---

### 7. Break-Detection Test Design (SC-001)

**Decision**: A dedicated sub-test deliberately introduces a non-deterministic value into the
`data` payload of the second run by using a wrapper around `run()` that patches the output bytes
after the call. The sub-test then asserts that `CompareOutputs` *reports a failure* (uses
`testing.TB` spy via `tbHelper` pattern).

**Rationale**:
- We cannot introduce non-determinism upstream (that would require modifying `main.go`).
- Patching the raw stdout bytes before calling the comparison is equivalent and tests the harness's
  detection logic accurately.
- Pattern: run both invocations normally, then mutate the second run's masked output to introduce a
  deliberate divergence, then call the comparison with a `testingTB` that records whether `t.Error`
  was called.

---

### 8. No New Dependencies

**Decision**: The entire harness is implemented using Go stdlib only (`bytes`, `encoding/json`,
`regexp`, `strings`, `time`, `testing`).

**Rationale**: Constitution Principle X mandates justifying every new dependency. All required
functionality is available in the stdlib. No new entry in `go.mod` is needed.
