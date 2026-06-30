# Feature Specification: Error-envelope recovery & remediation fields

**Feature Branch**: `013-error-recovery-fields`

**Created**: 2026-06-29

**Status**: Draft

**Input**: User description: "Add optional recovery/remediation fields to the
`ax.Error` envelope (GitHub issue #27). The envelope currently carries
`actionable_fix` and `suggestions` for remediation but cannot tell an LLM agent
whether a naive retry is safe or how long to wait before retrying. Add two
optional fields — `retryable` and `retry_after_seconds` (relative seconds, no
wall-clock) — reusing the existing `actionable_fix`/`suggestions` as the recovery
hints. Preserve byte-identical determinism, the required-field contract,
`errors.Is`/`errors.As`, and exit-code mapping. Golden tests cover the new shape.
Supersedes the stale 'amend ADR-0002' criterion (ADR-0002 is already retired)."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent decides whether to retry a failed command (Priority: P1)

An LLM agent drives an ax-go-based CLI and a command fails. Today the error
envelope tells the agent *what* failed (`error_code`, `message`) and offers
best-effort human hints (`actionable_fix`, `suggestions`), but nothing tells it
whether simply re-running the same command is safe. The agent must guess —
risking either giving up on a transient failure it could have recovered from, or
blindly re-running a non-idempotent operation. The agent wants a single
machine-readable signal it can branch on without parsing free text.

**Why this priority**: This is the core differentiator the AX source audit
(`docs/sources.md`) names most often — an error must carry a machine-readable
recovery path so an agent can self-correct instead of breaking. Retry-safety is
the smallest, highest-leverage slice and delivers value on its own.

**Independent Test**: Build an error envelope with the retry-safety signal set,
emit it, and confirm a consumer can read an explicit boolean (true / false /
absent) and branch deterministically — without any other field present.

**Acceptance Scenarios**:

1. **Given** a command fails on a transient condition the producer knows is
   safe to retry, **When** the error envelope is emitted, **Then** it carries an
   explicit retry-safety signal of `true`.
2. **Given** a command fails on a permanent condition (e.g. validation) the
   producer knows must not be retried, **When** the envelope is emitted, **Then**
   it carries an explicit retry-safety signal of `false`.
3. **Given** a producer sets no retry guidance, **When** the envelope is
   emitted, **Then** the retry-safety key is absent and the payload is
   byte-identical to the pre-feature envelope.

---

### User Story 2 - Agent learns how long to back off before retrying (Priority: P2)

For network/timeout failures (exit code `3`), an immediate retry can hammer a
service that is already struggling. When the producer knows a sensible backoff
window (e.g. a rate-limit reset), the agent wants that window surfaced so it can
wait the right amount of time rather than busy-looping or over-waiting.

**Why this priority**: Backoff guidance compounds the value of retry-safety but
is only meaningful once retry-safety exists, so it follows US1. It is the
natural companion for the `3` network/timeout exit class.

**Independent Test**: Build an error envelope with a backoff hint set, emit it
twice, and confirm both runs are byte-identical (no wall-clock drift) and a
consumer reads a non-negative whole number of seconds.

**Acceptance Scenarios**:

1. **Given** a network/timeout failure where the producer knows a backoff
   window, **When** the envelope is emitted, **Then** it carries a backoff hint
   expressed as a relative, non-negative whole number of seconds.
2. **Given** the same failing command runs twice, **When** both envelopes are
   compared, **Then** the backoff hint value is byte-identical across runs.
3. **Given** a producer supplies no backoff window, **When** the envelope is
   emitted, **Then** the backoff key is absent from the payload.

---

### User Story 3 - Existing producers and consumers see no behavior change (Priority: P3)

Teams already emitting and parsing the `ax.Error` envelope must be able to adopt
this release with zero changes. Producers that set none of the new fields, and
consumers that ignore them, must observe identical envelopes, identical exit
codes, and identical error-unwrapping behavior.

**Why this priority**: Backward compatibility is a hard constraint rather than a
new capability, so it is validated last — but it gates the release.

**Independent Test**: Run the pre-feature golden corpus unchanged against the new
build and confirm every fixture still matches.

**Acceptance Scenarios**:

1. **Given** an error built with no recovery options, **When** it is written,
   **Then** the JSON is byte-identical to the pre-feature golden fixture.
2. **Given** an error with a wrapped cause plus recovery fields set, **When**
   `errors.Is`/`errors.As` are applied, **Then** the wrapped cause is still
   reachable and the mapped exit code is unchanged.
3. **Given** an error carrying recovery fields, **When** the deterministic exit
   code is computed, **Then** it matches the exit code of the same error without
   recovery fields.

---

### Edge Cases

- **Backoff without retry-safety**: If a backoff hint is present but the
  retry-safety signal is `false` or absent, the retry-safety signal is
  authoritative — a consumer treats the backoff hint as meaningful only when
  retry-safety is `true`. The two fields are independent on the wire; their
  relationship is a documented consumer contract.
- **Negative backoff value**: A negative seconds value is nonsensical for a
  wait hint. It MUST be normalized deterministically rather than emitted
  verbatim (see FR-009).
- **Zero backoff value**: `0` seconds means "retry immediately"; it is a valid,
  distinct value from "absent" only if the producer explicitly sets it — see
  FR-009 for how zero interacts with omitempty normalization.
- **Determinism**: The backoff hint is always relative seconds; an absolute
  wall-clock instant is never emitted, because it would break byte-identical
  output across runs.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The error envelope MUST support an optional retry-safety signal
  communicating whether a naive re-run of the same command is safe, readable as
  a machine value without parsing free text.
- **FR-002**: The retry-safety signal MUST be able to express three
  distinguishable states: explicitly safe, explicitly unsafe, and unspecified
  (absent). Absence MUST be distinguishable from explicitly-unsafe so a consumer
  can tell "do not retry" apart from "no information".
- **FR-003**: The error envelope MUST support an optional backoff hint expressed
  as a relative, non-negative whole number of seconds before a retry should be
  attempted.
- **FR-004**: The backoff hint MUST be deterministic across runs — derived from
  no wall-clock value and emitting no absolute timestamp.
- **FR-005**: When a producer sets neither recovery field, the emitted envelope
  MUST be byte-identical to the pre-feature envelope (no new keys present).
- **FR-006**: Both recovery fields MUST be omitted from output when unset, so
  the default envelope shape and determinism are unchanged.
- **FR-007**: Setting either recovery field MUST NOT alter the deterministic
  exit-code mapping, the required-field set, or the reachability of any wrapped
  cause via `errors.Is` / `errors.As`.
- **FR-008**: The recovery fields MUST be settable through the same
  option-builder mechanism used by existing optional envelope fields, and MUST be
  available from both the import-isolated contract package and the root facade,
  producing byte-identical output from either entry point.
- **FR-009**: A negative backoff value supplied by a producer MUST be normalized
  deterministically to a safe value (treated as unset / omitted) rather than
  emitted as a negative number.
- **FR-010**: The existing `actionable_fix` and `suggestions` fields MUST remain
  the recovery-hint surface; no new free-text "next action" field is introduced
  by this feature.
- **FR-011**: Golden-file coverage MUST include at least one envelope that
  exercises both new recovery fields, and the root facade output MUST be proven
  byte-identical to the isolated contract output for that shape.

### Key Entities *(include if feature involves data)*

- **Error envelope**: The structured error record emitted to `stderr`
  (`ax.Error`, a facade over the isolated contract error). Existing attributes
  are unchanged. New optional attributes: a retry-safety signal (three-state:
  safe / unsafe / unspecified) and a backoff hint (relative, non-negative whole
  seconds). Both are absent unless a producer sets them.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent can determine the retry-safety of a failed command by
  reading a single machine-readable field, with zero free-text parsing, in 100%
  of envelopes where a producer set the signal.
- **SC-002**: Two runs of the same failing command produce byte-identical
  envelopes, including any recovery fields (0 non-deterministic bytes
  introduced).
- **SC-003**: 100% of pre-existing error-envelope golden fixtures pass unchanged
  when no recovery fields are set.
- **SC-004**: At least one golden fixture exercises both new fields, and the root
  facade output is proven byte-identical to the contract output for that fixture.
- **SC-005**: Exit-code mapping and `errors.Is`/`errors.As` behavior are
  unchanged for 100% of existing error test cases after the fields are added.

## Assumptions

- Source inputs: GitHub issue #27 and governing ADR-0002. ADR-0002 has **already
  been retired** from `docs/adr/` (only `0004`, `0008`, `0009` remain); the
  constitution now governs the `ax.Error` envelope and determinism contract, so
  there is no ADR file to absorb or delete as a final task — `research.md` records
  the superseded ADR-0002 decisions for provenance.
- The backoff hint is **relative delta-seconds**, chosen over an absolute
  timestamp specifically to preserve byte-identical determinism (constitution
  determinism mandate; mirrors HTTP `Retry-After` delta-seconds form).
- The retry-safety signal is the authoritative "safe to retry" indicator;
  the backoff hint is advisory and only meaningful when retry-safety is `true`.
- The three-state requirement (FR-002) implies the retry-safety signal is
  represented so that "explicitly false" and "absent" are distinguishable on the
  wire; the exact representation is an implementation/plan concern.
- This is a pre-v1.0 (`0.x`) additive change to a machine payload. New optional
  keys are additive-tolerant and ride a `0.MINOR.0` release; no consumer break is
  expected, and the Go API change (new option builders) is additive.
- "Reuse `actionable_fix` + `suggestions`" means this feature deliberately adds
  **no** new free-text remediation field; the net-new surface is exactly the
  retry-safety signal plus the backoff hint.
