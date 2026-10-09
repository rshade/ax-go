# Feature Specification: Known codes include the config package's runtime codes

**Feature Branch**: `032-known-config-codes`

**Created**: 2026-10-08

**Status**: Draft

**Input**: User description: "Issue #284: `__schema` `error_envelope.known_codes`
omits the config package's runtime error codes. Amend spec 029's definition so
`known_codes` lists every `error_code` ax-go itself can emit while a command
runs, including from public helper packages such as `config`; adopter-defined
and authoring-time codes stay excluded."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Plan recovery for config failures before running (Priority: P1)

An agent reads `__schema` before it drives a CLI built on ax-go. The CLI reads
a config file inside a command. The agent wants to know, before the first
call, every `error_code` ax-go can hand back, so it can plan a recovery for an
oversized or malformed config instead of meeting an undocumented code
mid-run.

**Why this priority**: `known_codes` is the one machine-readable list of
ax-go's codes. Today it omits five codes that reach the agent in a normal
stderr envelope with exit `2`, including `config_too_large`, which the README
calls frozen. A list that is silently incomplete is worse than none, because
an agent trusts it.

**Independent Test**: Emit `__schema` for any ax-go CLI and read
`error_envelope.known_codes`. It holds nine codes, sorted, and includes all
five `config_*` codes.

**Acceptance Scenarios**:

1. **Given** any command tree, **When** `__schema` is emitted, **Then**
   `error_envelope.known_codes` is exactly `config_invalid`,
   `config_max_bytes_invalid`, `config_option_invalid`,
   `config_patch_invalid`, `config_too_large`, `confirmation_required`,
   `internal_error`, `validation_error`, `warnings_as_errors`, in that
   (byte-wise sorted) order.
2. **Given** a command whose run fails a config read, **When** the agent
   reads the stderr envelope, **Then** its `error_code` is one of the
   CLI's `known_codes`.
3. **Given** two `__schema` runs of the same binary, **When** the agent
   compares `known_codes`, **Then** the arrays are byte-identical.

---

### User Story 2 - Existing code matchers keep working (Priority: P1)

An adopter's CLI and the agents that drive it already match on the config
codes by their exact spelling and exit code.

**Why this priority**: The spellings are frozen public contract. Listing the
codes must not move them.

**Independent Test**: Trigger each config failure (nil option, out-of-range
byte cap, oversized input, invalid Hujson on read, invalid Hujson on patch,
invalid patch document) and compare the envelope's `error_code` and exit code
against the values shipped before this feature.

**Acceptance Scenarios**:

1. **Given** each config failure, **When** it is triggered, **Then** the
   `error_code` spelling and exit code `2` are unchanged.
2. **Given** a Go adopter, **When** it compares an error's code against the
   newly published name for that code, **Then** the comparison matches the
   envelope the config package emits.

---

### User Story 3 - The field states what it covers (Priority: P2)

A maintainer or agent reading the `__schema` type documentation wants to know
what `known_codes` promises, without hunting through specs.

**Why this priority**: The definition is what the follow-up drift guard
(#285) will enforce. It has to be stated where readers look, and precisely
enough to test.

**Independent Test**: Read the documentation on the `known_codes` field. It
states the scope rule in FR-001, including both exclusions.

**Acceptance Scenarios**:

1. **Given** the published documentation for the `known_codes` field,
   **When** a reader looks it up, **Then** it states that the list covers
   every code ax-go itself can emit while a command runs, and that
   adopter-defined and authoring-time codes are excluded.

---

### Edge Cases

- `invalid_schema_declaration` is returned by the declaration functions
  while the adopter builds its command tree, before `ax.Execute` dispatches
  any command. It is authoring-time and stays out of `known_codes`.
- The MCP server's startup failures happen inside the `mcp-server`
  subcommand's run, so they are runtime codes under FR-001. All of them are
  already `validation_error`, so they add nothing to the list.
- Codes emitted by this repository's own gate tools (`surfacecheck`,
  `sizecheck`, `deadcheck`, `slopcheck`) are not part of any adopting CLI and
  are not ax-go library codes. They stay out.
- Codes in the integration example (`integration_failure`,
  `upstream_unreachable`, `permission_denied`, `sample_warning`) are
  adopter-defined. They stay out.
- A config error that the config package does not normalize (for example a
  file that cannot be opened) passes through as the underlying error and is
  reported by the runtime under an already-listed code. No new code arises.
- The list is a fresh value on every call; a caller that modifies the result
  does not change what the next caller or `__schema` sees.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `known_codes` MUST list every `error_code` that ax-go library
  code can place in an error envelope delivered as the result of a command
  run, meaning any error returned between the start of command dispatch and
  the return of `ax.Execute` or an MCP tool call. This includes codes from
  ax-go's public helper packages that an adopter calls during a run, such as
  `config`. It MUST NOT list adopter-defined codes, codes returned only while
  the command tree is being built (authoring-time), or codes emitted only by
  this repository's own development gate tools.
- **FR-002**: `known_codes` MUST include `config_option_invalid`,
  `config_max_bytes_invalid`, `config_too_large`, `config_invalid`, and
  `config_patch_invalid`, alongside the four codes it lists today, nine in
  total.
- **FR-003**: `known_codes` MUST be sorted byte-wise ascending and MUST NOT
  contain duplicates.
- **FR-004**: `known_codes` MUST NOT include `invalid_schema_declaration`.
- **FR-005**: Each of the five config codes MUST be published as a named,
  exported constant in the import-isolated `contract` package, matching the
  existing `warnings_as_errors` constant, and the config package MUST emit
  its codes through those constants rather than repeated string literals.
- **FR-006**: Every existing `error_code` spelling and its exit code MUST
  stay unchanged.
- **FR-007**: The documentation on the `known_codes` field of the published
  `__schema` type MUST state the FR-001 scope rule.
- **FR-008**: Spec 029's definition of known codes MUST be amended to the
  FR-001 rule, with a pointer to this feature, so the two specs do not
  disagree.
- **FR-009**: The README's config error-code section MUST name all five
  config codes (today it omits `config_patch_invalid`) and point readers at
  `__schema`'s `known_codes` as the machine-readable list.
- **FR-010**: Each call that returns the known-code list MUST return a value
  the caller may modify without affecting later calls.

### Constitution alignment

- Additive under Principle XI: `known_codes` gains entries, no field is
  removed or retyped, and the exported surface only gains constants. No
  `breaking-change-approved` label is needed, and the change ships as `feat:`.
- The new constants live in `contract`, which stays import-isolated from the
  runtime facade, so a thin consumer can match codes without linking the
  runtime.
- Output stays deterministic: the list is a fixed, sorted value.
- Every `__schema` golden changes. The diff is reviewed as a machine-contract
  change, and the surface baseline and audit record the new constants in the
  same change.

### Key Entities

- **Known code**: an `error_code` string that satisfies FR-001, with a fixed
  spelling and exit code.
- **Known-code list**: the sorted set of known codes, published in
  `__schema` under `error_envelope.known_codes`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `__schema` lists 9 known codes, up from 4, and all 5 config
  codes an agent can receive during a run are among them.
- **SC-002**: For every config failure an adopter's command can trigger
  through ax-go's config helpers, the envelope's `error_code` appears in that
  CLI's `known_codes`: 6 of 6 failure paths covered, 0 undocumented.
- **SC-003**: 0 existing `error_code` spellings or exit codes change.
- **SC-004**: Two `__schema` emissions of the same binary produce
  byte-identical `known_codes`.

## Assumptions

- Source inputs: GitHub issue #284 and no governing ADR.
- Clarification of the MCP startup question raised in the issue is settled
  here rather than deferred: the `mcp-server` subcommand is a command, so its
  startup failures are runtime codes (FR-001). The list is identical either
  way.
- Converting the other emit sites (`execute.go`, `confirm.go`, the MCP
  server, `mcp`, `schema`) to constants, and the drift-guard test that keeps
  the list complete, are the follow-up #285. This feature fixes the
  definition and the data; it does not add automated drift detection.
- A survey of the library at specification time found no runtime codes
  beyond the four already listed and the five config codes, so FR-001 and
  FR-002 describe the same nine codes.
- New error codes (issues #130, #125) and renames are out of scope.
