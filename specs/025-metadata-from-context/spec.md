# Feature Specification: Live-Resolving Metadata Accessor for Root ax

**Feature Branch**: `025-metadata-from-context`

**Created**: 2026-09-07

**Status**: Draft

**Input**: User description: "Export ax.MetadataFromContext(ctx) ax.Metadata from the root ax package with live OpenTelemetry trace and span ID resolution, matching what ax.NewEnvelope and ax.NewError already emit. Today the only exported MetadataFromContext is contract.MetadataFromContext, which is import-isolated from OpenTelemetry and always returns ZeroTraceID/ZeroSpanID when called from root-ax code (e.g. inside a command run via ax.Execute) — this is a public-API/behavior gap, tracked as GitHub issue #212."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Read the same metadata a command's own output would carry (Priority: P1)

A developer is building a command on top of ax-go and needs the same trace-correlated metadata (trace ID, span ID, dry-run state, idempotency key) that the command's own success or error envelope will carry — for example, to embed it in a custom payload shape, an NDJSON line, or a log field — without hand-rolling the composition logic themselves.

**Why this priority**: This is the entire reason the feature exists. Without it, the only exported accessor named for this purpose (`contract.MetadataFromContext`) silently returns zero-value trace/span IDs whenever it is called from root-package code, because the import-isolated `contract` package cannot see the active OpenTelemetry span. A developer who reaches for the obviously-named function gets a partially wrong answer with no error to signal it.

**Independent Test**: Can be fully tested by running a command through `ax.Execute`, calling the new accessor from inside the command handler, and asserting its result is field-for-field identical to what the command's own envelope construction would produce for the same context at the same point in execution.

**Acceptance Scenarios**:

1. **Given** a command is executing under `ax.Execute` with an active root span, `--dry-run` set, and an auto-generated idempotency key, **When** the handler calls the new metadata accessor on the command's context, **Then** it returns metadata whose trace ID and span ID are non-zero and equal to the trace ID and span ID that would appear in that same context's success envelope, and whose dry-run flag and idempotency key also match.
2. **Given** the same running command, **When** a handler stores an explicit trace or span ID on the context through the existing explicit-metadata mechanism and a live span is also active, **Then** the live span's IDs take precedence in the returned metadata, exactly as they already do for the envelope and error constructors.

---

### User Story 2 - Get warned by the isolated accessor instead of silently misled (Priority: P2)

A developer who is not yet aware of the import-isolation boundary reaches for the pre-existing isolated metadata accessor and reads its documentation before or after being surprised by zero-value IDs.

**Why this priority**: This closes the actual gap that let the bug go unnoticed — the isolated accessor's documentation does not currently disclose that it cannot see an active span, even though a sibling function in the same file already carries that exact warning. Fixing the behavior gap without fixing the documentation gap would leave the trap in place for the next developer.

**Independent Test**: Can be fully tested by reading the isolated accessor's documentation in isolation (no code execution) and confirming it discloses the same live-span limitation already documented on its sibling trace/span ID accessors.

**Acceptance Scenarios**:

1. **Given** the isolated metadata accessor's documentation, **When** a developer reads it, **Then** it states plainly that it does not resolve an active tracing span and that live-resolving metadata is available from the root package.

---

### User Story 3 - Discover the new accessor where envelopes are already documented (Priority: P3)

A developer learning the library through its reference documentation and getting-started material encounters the new accessor in the same place they learn how success envelopes are built, rather than having to already know it exists.

**Why this priority**: Discoverability matters less than correctness (P1) and eliminating the documentation trap (P2), but an accessor nobody finds does not fix the underlying problem for new adopters.

**Independent Test**: Can be fully tested by checking that the reference documentation and introductory tutorial material that already describe envelope construction also mention the new accessor at the same point.

**Acceptance Scenarios**:

1. **Given** the existing documentation describing how success envelopes are constructed, **When** a developer reads that section, **Then** they also find the new metadata accessor described there.

---

### Edge Cases

- What happens when the accessor is called with a context that carries no active tracing span (e.g., outside any command execution, or with a bare background context)? It returns the well-defined zero-value trace and span IDs rather than an error, matching today's isolated-accessor behavior for the identifiers it can supply.
- What happens when the accessor is called with a `nil` context? It must not panic; it behaves as if an empty background context had been passed, mirroring how the library's other context-reading helpers already handle `nil`.
- What happens when a handler opens its own child span before calling the accessor? The accessor resolves whatever span is active in the context passed to it at the moment of the call — it does not return metadata captured earlier in the command's lifecycle, so a child span's identifiers are correctly reflected rather than a stale root-span identifier.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The root package MUST provide a function that returns the machine-envelope metadata (trace ID, span ID, dry-run state, idempotency key) for a given context, resolved at the moment of the call.
- **FR-002**: The trace ID and span ID in the returned metadata MUST reflect the actively executing tracing span carried by the context, exactly matching the identifiers that the library's own success-envelope and error-envelope constructors would embed for that same context at that same point in execution.
- **FR-003**: When the context carries no active tracing span, the function MUST return the library's well-defined zero-value trace and span identifiers rather than erroring or panicking.
- **FR-004**: When passed a `nil` context, the function MUST NOT panic and MUST behave as though an empty background context were supplied.
- **FR-005**: The returned metadata MUST include the dry-run flag and idempotency key already readable from the context, with the same values the pre-existing isolated accessor already surfaces for those two fields.
- **FR-006**: When the context carries both an explicitly stored trace/span identifier and an actively executing span, the actively executing span's identifiers MUST take precedence in the result, consistent with how the existing envelope and error constructors already resolve that conflict.
- **FR-007**: The new function's documentation MUST state that it resolves the live tracing span and that an explicitly stored trace/span identifier is superseded by that live span.
- **FR-008**: The pre-existing isolated metadata accessor's documentation MUST be updated to disclose that it does not resolve an active tracing span and that live-resolving metadata is available from the root package, mirroring the equivalent warning already present on its sibling trace-ID and span-ID accessors.
- **FR-009**: This change MUST NOT alter the pre-existing isolated accessor's returned values for any input, nor change the shape of the success-envelope or error-envelope machine payloads.
- **FR-010**: This change MUST be recorded as an additive, non-breaking addition to the library's public surface, including whatever review artifacts the project's public-surface change process requires.
- **FR-011**: Reference documentation and introductory tutorial material that already describe how success envelopes are constructed MUST also describe the new accessor.

### Key Entities

- **Metadata**: The existing machine-envelope metadata record (trace ID, span ID, dry-run flag, idempotency key) already emitted inside every success and error payload. This feature adds a new way to read a fully-resolved instance of it directly from a context; it does not change the record's shape.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer calling the new accessor from inside a running command receives trace and span identifiers that are non-zero and identical to the identifiers embedded in that same command's own output, in 100% of executions with an active trace.
- **SC-002**: A developer calling the new accessor outside of any active trace context receives well-defined zero-value identifiers rather than an error or crash, in 100% of such calls, including when passed a `nil` context.
- **SC-003**: Every existing caller of the pre-existing isolated accessor observes byte-identical return values after this change ships — zero behavioral change to that accessor.
- **SC-004**: A developer reading the documentation for how success envelopes are built encounters the new accessor within that same reading, with no additional search required.

## Assumptions

- Source inputs: [GitHub issue #212](https://github.com/rshade/ax-go/issues/212). No governing ADR applies.
- The project's existing internal mechanism for composing trace-aware context (already proven correct by the existing envelope and error constructors' test coverage) is reused for this new read path rather than reimplemented from scratch.
- No change to the wire/JSON shape of the success-envelope or error-envelope machine payloads; this feature adds a new read path for metadata that both payload types already carry, for consumers who need it independent of constructing either payload type.
- This feature is scoped to the root package's public surface; the pre-existing isolated package's returned values for its own callers are unchanged — only its documentation gains a disclosure.
- This is not a hot-path operation and carries no performance budget of its own.
