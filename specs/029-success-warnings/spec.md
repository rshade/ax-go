# Feature Specification: Success warnings and strict escalation

**Feature Branch**: `issue-123`

**Created**: 2026-10-08

**Status**: Draft

**Input**: User description: "Issue #123: structured warnings on the success envelope and --strict escalation."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Non-fatal findings stay on success (Priority: P1)

An agent runs a command that completed its work but noticed something the
agent should see. The process exits successfully and the machine payload
lists those findings in a stable structure.

**Why this priority**: Without a structured channel, findings are either
buried in prose or forced into a hard failure.

**Independent Test**: Run a command that attaches two warnings and no
`--strict` flag. The process exits 0, stdout is one JSON success payload,
and `warnings` lists the two findings in the order they were attached.

**Acceptance Scenarios**:

1. **Given** a command that attaches warnings, **When** it is run without
   `--strict`, **Then** the exit code is unchanged from success and stdout
   carries `warnings` as objects with `code` and `message`.
2. **Given** a command that attaches no warnings, **When** it succeeds,
   **Then** the success payload omits `warnings`.
3. **Given** the same warnings in the same order, **When** the command is
   run twice, **Then** the warning array is byte-identical after masking
   fields already documented as non-deterministic.

---

### User Story 2 - Strict mode turns warnings into a failure (Priority: P1)

A CI job must fail when a command reports findings. It passes `--strict`
and treats a non-zero exit as the gate.

**Why this priority**: Agents cannot gate on English text.

**Independent Test**: Run the same command with `--strict`. The process
exits 2, stdout is empty, and stderr is one error payload whose
`error_code` is `warnings_as_errors`.

**Acceptance Scenarios**:

1. **Given** one or more warnings, **When** `--strict` is set, **Then**
   the exit code is 2, stdout is empty, and stderr carries
   `warnings_as_errors`.
2. **Given** no warnings, **When** `--strict` is set, **Then** the command
   still succeeds and stdout is the normal success payload.
3. **Given** the command returns a real error, **When** `--strict` is
   set, **Then** that error wins and stdout stays empty.

---

### User Story 3 - Schema names the new failure (Priority: P2)

An agent reads `__schema` before calling the command. It can see that
`warnings_as_errors` is a known error code.

**Why this priority**: The code is part of the machine contract, so the
schema must name it.

**Independent Test**: Read the error-envelope section of `__schema` and
find `warnings_as_errors` among the known codes.

**Acceptance Scenarios**:

1. **Given** a command tree, **When** `__schema` is emitted, **Then** the
   error envelope lists `warnings_as_errors` as a known code.

---

### Edge Cases

- A warning with a blank code or a blank message is dropped and cannot
  escalate.
- Warning order is the caller's order. The runtime does not sort it.
- `--strict` is boolean. There is no severity field and no
  `--strict=<level>` form.
- Under `--strict`, stdout is held until the command returns, then either
  released (no warnings) or discarded (warnings or a returned error).
- Streaming many payloads is out of scope. Strict mode sees the last
  set of warnings the command attached in that run.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A success payload MUST be able to carry `warnings`, each
  item having `code` and `message`. The field is absent when there are
  no warnings.
- **FR-002**: Warning order MUST be the order the caller supplied.
- **FR-003**: Without `--strict`, warnings MUST NOT change the exit code.
- **FR-004**: With `--strict` and at least one kept warning, the process
  MUST exit 2, write a `warnings_as_errors` error payload to stderr, and
  write nothing to stdout.
- **FR-005**: `__schema` error-envelope metadata MUST include
  `warnings_as_errors` as a known code.
- **FR-006**: The escalation floor is presence. `Warning` has no severity.
  Any kept warning is at the floor. A severity taxonomy is out of scope.

### Constitution alignment

- stdout stays the success payload. Escalated runs write the error
  payload to stderr only.
- The success shape gains a field and does not remove or retype an
  existing field.
- The new shape lives in the contract package so a thin consumer can
  read it without the runtime.
- Output stays deterministic aside from fields already marked
  non-deterministic.

### Key Entities

- **Warning**: a code and a message.
- **Success payload**: existing data and metadata, plus optional warnings.
- **Strict flag**: a boolean persistent flag. Set means "escalate".

## Success Criteria *(mandatory)*

- **SC-001**: A success run with warnings exits 0 and a consumer can read
  each finding's code and message without parsing prose.
- **SC-002**: The same warnings produce the same warning array on two
  runs, once non-deterministic metadata is masked.
- **SC-003**: A strict run with a warning exits 2, leaves stdout empty,
  and names `warnings_as_errors` on stderr.
- **SC-004**: A strict run with no warnings still exits 0.
- **SC-005**: `__schema` names `warnings_as_errors`.

## Assumptions

- Clarification was not required. The issue left the severity floor open.
  FR-006 settles it: `--strict` is boolean and the floor is "any
  warning", because a severity taxonomy is explicitly out of scope.
- Commands attach warnings through the library helper that both fills
  the success payload and records them for the runner. Recording is what
  makes `--strict` deterministic even though the command writes its own
  stdout.
- Known error codes published in `__schema` are every `error_code` ax-go
  library code can return from a command run, including codes from public
  helper packages such as `config`. Adopter-defined codes, authoring-time
  codes (`invalid_schema_declaration`), and the repository's gate-tool codes
  are not listed. Amended by specs/032-known-config-codes (FR-001); this
  feature introduced the list with `confirmation_required`,
  `internal_error`, `validation_error`, and `warnings_as_errors`.
