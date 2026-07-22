# Feature Specification: __schema Non-Deterministic Field Enumeration

**Feature Branch**: `015-schema-nondeterministic-fields`

**Created**: 2026-07-08

**Status**: Draft

**Input**: User description: "GitHub issue #16 — `__schema`: enumerate non-deterministic fields per command output. The output-determinism mandate requires byte-identical envelopes for identical inputs, modulo 'documented non-deterministic fields.' Today those fields are not actually documented per command. Extend `__schema` so every command declares which output fields are non-deterministic (trace_id, span_id, auto-generated idempotency_key, timestamp fields when present, and any domain-specific fields a command author marks)."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent trusts the diff, not just the promise (Priority: P1)

An agent operator (or an automated agent) runs the same command twice with identical input and diffs the two `stdout` payloads to detect unintended drift. Today it has no reliable way to know which fields are *supposed* to differ (timestamps, trace IDs, auto-generated keys) versus which differences indicate a real bug. It either hard-codes a guess at the varying fields (fragile, breaks silently when a new command adds one) or gives up and skips diffing entirely (loses the safety net determinism is supposed to provide).

**Why this priority**: This is the entire reason the feature exists — the output-determinism mandate is only useful to consumers if they can act on it, and today they can't without out-of-band knowledge.

**Independent Test**: Can be fully tested by calling `__schema` for any command, reading its `non_deterministic_fields` list, running the command twice, and confirming that every observed difference between the two runs' output is accounted for by an entry in that list.

**Acceptance Scenarios**:

1. **Given** a command whose output includes a trace ID, a timestamp, and a domain-specific `report_id`, **When** an agent requests `__schema` for that command, **Then** the response lists all three fields in `non_deterministic_fields`, each identified so it can be matched to the corresponding field in the command's actual output.
2. **Given** a command whose output has no non-deterministic fields at all, **When** an agent requests `__schema` for that command, **Then** the response includes an explicit empty `non_deterministic_fields` list rather than omitting the field.
3. **Given** two consecutive runs of the same command with identical input, **When** an agent removes every field named in `non_deterministic_fields` from both outputs, **Then** the two remaining payloads are byte-identical.

---

### User Story 2 - Command author marks a field once (Priority: P2)

A developer building a command on top of ax-go adds a new output field that will vary between runs (for example, a generated report ID or a server-assigned batch number). They need a straightforward way to flag that field as non-deterministic at the point they define it, so the flag travels with the field and can't be forgotten when the schema is generated or drift when the field is renamed.

**Why this priority**: Without an author-facing marking mechanism, the enumeration in User Story 1 would have to be hand-maintained in a second location, which defeats the purpose (the exact problem this feature exists to solve for the *built-in* metadata fields today).

**Independent Test**: Can be fully tested by having a command author mark a single new field as non-deterministic, regenerating `__schema` output, and confirming the field appears in that command's `non_deterministic_fields` list with no other code changes required.

**Acceptance Scenarios**:

1. **Given** a command author adds a new field to their output struct and marks it as non-deterministic, **When** `__schema` is generated, **Then** the new field appears in that command's `non_deterministic_fields` list automatically.
2. **Given** a command author renames a field that was previously marked non-deterministic, **When** `__schema` is regenerated, **Then** the list reflects the new field name (the marking stays attached to the field, not a separately maintained string).

---

### User Story 3 - Regressions are caught before release (Priority: P3)

A maintainer changes a command's output shape (adds, removes, or renames a field) and wants confidence that the `non_deterministic_fields` enumeration in `__schema` still matches reality, without manually re-verifying every command by hand.

**Why this priority**: Enforcement is what keeps the enumeration trustworthy over time; without it, the list quietly rots the same way the undocumented status quo did.

**Independent Test**: Can be fully tested by intentionally dropping a previously-documented non-deterministic field from a command's output (or from its `__schema` declaration) and confirming an automated check fails before the change can be merged.

**Acceptance Scenarios**:

1. **Given** a command's `__schema` output currently lists a specific non-deterministic field, **When** a code change removes that field's marking without removing the field itself, **Then** an automated test fails, flagging the mismatch.
2. **Given** the full set of commands exposed by an ax-go-based CLI, **When** the automated test suite runs, **Then** every command's `non_deterministic_fields` list is verified against a known-good golden reference.

---

### Edge Cases

- What happens when a field is only non-deterministic under some conditions (e.g., an idempotency key that is deterministic when user-supplied but auto-generated otherwise)? The field MUST still appear in the enumeration whenever it is *capable* of being auto-generated, since an agent inspecting `__schema` ahead of time cannot know which case a given run will hit.
- What happens for a command with deeply nested output (subcommands, repeated structures)? Each command in the tree declares its own `non_deterministic_fields` list scoped to its own output shape; nested commands do not inherit a parent's list.
- What happens when a command produces an error envelope instead of its success payload? Error envelopes are output too and are subject to the same determinism mandate, so their non-deterministic fields (e.g., a trace ID surfaced in the error) MUST also be enumerated.
- What happens for reserved or operational commands that do not emit a standard success envelope (for example `__schema`, shell completion, or an MCP server command)? Their command-scoped `non_deterministic_fields` list MUST be explicit and empty unless their actual output shape contains a marked non-deterministic field; the global error-envelope schema still documents error output separately.
- What happens when two different commands happen to share a field name (e.g., both have `metadata.trace_id`)? Each command's enumeration is independent; there is no cross-command deduplication or shared registry an agent needs to consult.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `__schema` output MUST include a `non_deterministic_fields` list for every command in the tree, including an explicit empty list for commands with no non-deterministic output fields.
- **FR-002**: Each entry in `non_deterministic_fields` MUST identify its field with a stable locator that an agent can map directly to the corresponding field in that command's actual output (success payload or error envelope).
- **FR-003**: Command and library authors MUST have a declarative way to mark an output field as non-deterministic at the point the field is defined, so the marking travels with the field through renames and refactors.
- **FR-004**: The `non_deterministic_fields` list in `__schema` MUST be derived automatically from those markings; it MUST NOT require a hand-maintained list kept separately from the field definitions.
- **FR-005**: The enumeration MUST cover, at minimum, the built-in ax-go envelope metadata fields already understood to vary between runs (trace correlation ID, span ID, and auto-generated idempotency key) wherever each appears in a command's output. Timestamp fields are covered when they appear in a payload or error shape and are marked as non-deterministic by that output shape.
- **FR-006**: A field capable of being auto-generated in some cases and user-supplied in others (e.g., idempotency key) MUST be listed unconditionally in the enumeration, since the schema is generated independent of any single run's input.
- **FR-007**: The `__schema --as=mcp` adapter output MUST carry the same non-deterministic-field information (or its MCP-tool equivalent), so MCP-wrapped consumers get the same guarantee as direct `__schema` consumers.
- **FR-008**: Automated tests MUST verify that each command's declared `non_deterministic_fields` list matches a known-good reference, so an unintentional addition or removal is caught before merge.
- **FR-009**: Reference documentation MUST state that the `non_deterministic_fields` enumeration in `__schema` is the authoritative source of truth for which output fields an agent may safely ignore when diffing two runs.
- **FR-010**: Adding a field to a command's `non_deterministic_fields` list MUST be treated as a non-breaking (additive) change to the public `__schema` contract; removing a field from the list MUST be treated as a breaking change.

### Key Entities

- **Command Schema Entry**: The existing per-command node in `__schema` output (command tree, flags, types, examples), extended to also carry its `non_deterministic_fields` list.
- **Non-Deterministic Field Marking**: The declarative flag attached to an individual output field indicating that its value is expected to vary between otherwise-identical runs.
- **Non-Deterministic Field Locator**: The stable identifier (per FR-002) used within `non_deterministic_fields` to point at a specific field in a command's output shape.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For 100% of commands exposed by an ax-go-based CLI, `__schema` output includes a `non_deterministic_fields` declaration (populated or explicitly empty) — no command ships without one.
- **SC-002**: An agent that removes every field listed in a command's `non_deterministic_fields` from two consecutive runs' output observes byte-identical results, for 100% of commands.
- **SC-003**: A command author can mark a new non-deterministic field and see it reflected in generated `__schema` output through a single, localized code change — no second list to update.
- **SC-004**: A change that silently drops a previously-documented non-deterministic field is caught by automated verification before it reaches the main branch, 100% of the time.

## Assumptions

- Source inputs: [GitHub issue #16] and the output-determinism mandate already codified in the project constitution (Principle II, "Deterministic Output & Exit Codes") and Principle III ("Machine Discoverability via `__schema`"). The issue references ADR-0003 and ADR-0002; those numbers do not correspond to files currently present in `docs/adr/` (the ADR log is frozen and being retired per the Spec Kit workflow), so this feature's `research.md` will absorb any relevant prior decision directly rather than editing or creating an ADR.
- The enumeration applies to both success payloads and `ax.Error` envelopes, since both are `stdout`/`stderr` output subject to the same determinism mandate.
- Scope is the built-in metadata fields currently emitted by ax-go success envelopes (trace ID, span ID, auto-generated idempotency key), timestamp fields when present and marked on a payload/error shape, plus whatever domain-specific fields individual command authors choose to mark; this feature does not attempt to automatically detect non-determinism in a field an author never marks.
- Commands that emit the standard ax-go success envelope are expected to declare that output shape at command construction time; that declaration is the schema generator's precise signal that the built-in `meta.*` fields appear in that command's success output. Commands without this declaration are treated as raw-output, operational, or error-only commands for command-scoped enumeration and therefore get an explicit empty list unless another supported output-shape marker is added later.
- The field locator format only needs to be stable and unambiguous within a single command's output shape; it is not required to be globally unique across the whole command tree.
- Command authors are expected to mark fields at the time they add them; this feature does not include tooling that infers non-determinism from runtime behavior (e.g., diffing sample outputs to guess which fields vary).
