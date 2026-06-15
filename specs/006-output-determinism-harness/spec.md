# Feature Specification: Output-Determinism Test Harness

**Feature Branch**: `006-output-determinism-harness`

**Created**: 2026-06-14

**Status**: Draft

**Input**: GitHub issue #5 — "Output-determinism test harness"

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Duplicate-run comparison on the success path (Priority: P1)

A library developer runs the test suite and wants assurance that the standard
success envelope emitted to `stdout` is byte-identical across two executions
given the same deterministic inputs. The only fields allowed to differ are the
three documented non-deterministic fields (`trace_id`, `span_id`,
`idempotency_key`); after those are masked to a stable sentinel, both outputs
must match exactly.

**Why this priority**: Determinism is the machine-trust contract defined in
Constitution Principle II. Without a test that actually compares two runs, the
guarantee exists only in documentation. This is the core deliverable.

**Independent Test**: Can be fully tested by invoking the integration example's
success command twice with a pinned idempotency key, masking the three
non-deterministic fields in each output, and asserting the masked bytes are
equal. Delivers standalone regression coverage with no dependency on Stories 2
or 3.

**Acceptance Scenarios**:

1. **Given** the integration example's default success command is run twice
   with identical arguments and a fixed idempotency key, **When** the
   non-deterministic fields (`trace_id`, `span_id`, `idempotency_key`) in
   both `stdout` outputs are replaced with a stable sentinel value, **Then**
   the two masked outputs are byte-identical.

2. **Given** the same two-run comparison, **When** a developer deliberately
   introduces a non-deterministic value into the `data` payload (e.g., a
   timestamp or random ID in the business payload), **Then** the comparison
   fails with a diff that identifies the differing bytes, making the regression
   visible.

---

### User Story 2 — Duplicate-run comparison on the NDJSON streaming path (Priority: P2)

A library developer wants the same determinism guarantee for commands that emit
multiple NDJSON lines to `stdout`, not just a single bounded envelope.

**Why this priority**: NDJSON streaming (Constitution Principle I) is
a first-class output mode. An agent consuming a stream expects each line to be
structurally stable across retries. Without a streaming-path assertion, the
harness would give a false sense of completeness.

**Independent Test**: Can be fully tested by invoking the integration example's
`stream` command twice, splitting `stdout` into individual NDJSON lines,
masking non-deterministic fields per-line, and asserting line-count and
line-by-line equality.

**Acceptance Scenarios**:

1. **Given** the streaming command is run twice with identical arguments,
   **When** each NDJSON line's non-deterministic fields are masked, **Then**
   the line count and every masked line are byte-identical between the two
   runs.

2. **Given** the streaming command is run twice, **When** one run emits a
   different number of lines than the other (simulating a broken implementation),
   **Then** the comparison fails with a clear report of the line-count mismatch.

---

### User Story 3 — Envelope structure and timestamp format verification (Priority: P3)

A library developer wants the test harness to also confirm that: (a) `stdout`
envelopes are modeled with structs (not freeform maps, which allow
non-deterministic field ordering), and (b) any timestamp fields present in the
envelope conform to the RFC 3339 UTC format required by Constitution Principle
II.

**Why this priority**: These are supporting invariants of determinism. Struct
modeling enforces field-order stability across `encoding/json` calls; RFC 3339
UTC timestamps prevent locale- or timezone-dependent drift. Both can be verified
statically as part of the same test suite without requiring a second run.

**Independent Test**: Can be fully tested by deserializing the envelope from a
single run, checking that the deserialized representation contains only
known-typed fields (no raw `interface{}` or `map[string]any` holding the root
envelope), and validating that any timestamp string values parse as valid RFC
3339 UTC.

**Acceptance Scenarios**:

1. **Given** a success-path `stdout` envelope, **When** the timestamp fields
   are extracted, **Then** each parses as a valid RFC 3339 UTC string (timezone
   designator `Z` or `+00:00`, no local-time offsets).

2. **Given** a success-path `stdout` envelope, **When** it is deserialized into
   a strongly-typed struct, **Then** no fields require a freeform
   `map[string]any` or untyped `interface{}` to represent the envelope shape
   (i.e., the envelope is fully typed).

---

### Edge Cases

- What happens when the two runs produce different `stdout` byte lengths even
  after masking? The harness must report the lengths and print a human-readable
  diff of the first divergence.
- What if `stdout` is empty for a given run (command errored)? The harness must
  fail clearly, distinguishing a comparison failure from an execution failure.
- What if a timestamp in the envelope has a non-UTC timezone offset (e.g.,
  `+05:30`)? The format check must treat this as a failure, not a pass.
- What if the NDJSON stream contains a blank trailing newline? The harness must
  normalize line splitting consistently between runs to avoid false negatives.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The test harness MUST execute a given command twice with identical
  inputs and compare the `stdout` of both runs after masking documented
  non-deterministic fields.
- **FR-002**: The harness MUST mask exactly the three documented non-deterministic
  fields — `trace_id`, `span_id`, and `idempotency_key` — replacing each with a
  stable sentinel value before comparison.
- **FR-003**: The harness MUST support single-envelope (bounded JSON) `stdout`
  and multi-line NDJSON `stdout` as distinct comparison modes.
- **FR-004**: When a comparison fails, the harness MUST emit a human-readable
  report identifying the first point of divergence (line number for NDJSON;
  byte offset for single-envelope).
- **FR-005**: The harness MUST verify that any timestamp string fields in the
  `stdout` envelope conform to RFC 3339 UTC (timezone `Z` or `+00:00`).
- **FR-006**: The harness MUST verify that the `stdout` envelope is fully typed
  (no freeform map or untyped interface holding the root envelope shape).
- **FR-007**: The harness MUST be applied to at least the integration example's
  success path and its NDJSON streaming path.
- **FR-008**: The harness MUST be callable as a reusable helper from any test in
  the repository without duplicating masking or comparison logic.
- **FR-009**: The harness MUST fail clearly and immediately if either of the two
  command executions produces a non-zero exit code or empty `stdout`, rather than
  comparing empty or partial output.
- **FR-010**: The harness MUST NOT modify `stderr` or exit codes as part of its
  operation; it is a read-only observer of `stdout`.

### Key Entities *(include if feature involves data)*

- **Deterministic run pair**: Two executions of the same command with identical
  arguments; the unit of comparison the harness operates on.
- **Non-deterministic field set**: The fixed, documented set of envelope fields
  whose values legitimately differ across runs: `trace_id`, `span_id`,
  `idempotency_key`.
- **Stable sentinel**: A constant placeholder value substituted for each
  non-deterministic field before comparison; its exact value is an
  implementation detail, but it must be consistent across both masked outputs.
- **Masked output**: The `stdout` bytes of one run after all non-deterministic
  fields are replaced with the stable sentinel; the artifact that is compared.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The determinism harness catches a regression introduced by adding
  a non-deterministic value to the `data` payload within one test run (verified
  by a deliberate break-and-detect test).
- **SC-002**: The harness covers both the single-envelope success path and the
  NDJSON streaming path of the integration example, with zero additional test
  setup beyond providing the command arguments.
- **SC-003**: Timestamp format checks reject any envelope timestamp that is not
  RFC 3339 UTC, and the check passes for all envelopes emitted by the current
  integration example.
- **SC-004**: The entire test suite, including the new harness tests, completes
  in under 30 seconds on the CI runner used by the repository (no new long-
  running or flaky tests introduced).
- **SC-005**: Running the test suite twice in sequence on an unmodified codebase
  produces identical pass/fail outcomes (the harness itself is not flaky).

## Assumptions

- Source inputs: GitHub issue #5 and the governing decision from ADR-0011
  (output format) is already absorbed into specs/006-output-determinism-harness/research.md.
- The integration example's `run()` function signature (accepting writers for
  `stdout` and `stderr`) is stable and can be called directly from test code
  without spawning a subprocess.
- The non-deterministic field set is fixed at exactly `trace_id`, `span_id`,
  and `idempotency_key`; if future work adds documented non-deterministic fields,
  the harness will need a corresponding update (out of scope here).
- Timestamp fields in the envelope's `data` payload are application-specific;
  the harness validates timestamps it finds, but does not require any particular
  field to be present — absence is not a failure.
- The harness helper is scoped to in-process test invocations (via `bytes.Buffer`
  and the `run()` function), not subprocess-level `os/exec` testing, to keep
  setup minimal and execution fast.
- Mobile or browser environments are out of scope; the harness targets the
  standard Go test runner on Linux/macOS CI.
