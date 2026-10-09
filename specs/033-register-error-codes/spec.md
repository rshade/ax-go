# Feature Specification: Register every error code and guard the registry

**Feature Branch**: `033-register-error-codes`

**Created**: 2026-10-09

**Status**: Draft

**Input**: User description: "Issue #285: Register every runtime error_code as
an exported contract constant, build KnownErrorCodes() from that registry with
invalid_schema_declaration in a separate commented authoring-time exclusion
list, replace every string-literal error code at ax-go emit sites with
constants, delete the duplicate contractErrorWarnings constant in execute.go,
and add a drift-guard test that walks the module's non-test Go sources and
fails when any NewError code argument (contract.NewError or root ax.NewError)
is a string literal or an unregistered identifier, proven by a fixture.
__schema output stays byte-identical; no code spellings, exit codes, or
known_codes membership change."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A new error path cannot skip the registry (Priority: P1)

A maintainer adds an error path to ax-go and writes a new `error_code`
inline. Before this feature nothing connects that code to the agent-facing
`known_codes` list, which is how five config codes went unlisted (#284). After
it, the repository's test suite fails and names the file, line, and offending
expression, so the code has to be registered before the change can merge.

**Why this priority**: This is the whole point of the issue. Two open issues
(#130, #125) are about to add runtime codes; without an automated check each
must remember to update the list by hand.

**Independent Test**: Point the drift guard at a fixture that emits a code
through a string literal and through an identifier the registry does not
hold. The guard reports both, with their locations. Point it at this
repository's own sources and it reports nothing.

**Acceptance Scenarios**:

1. **Given** library code that constructs an error envelope with a string
   literal as its code, **When** the test suite runs, **Then** the drift guard
   fails and names the file, line, and literal.
2. **Given** library code that constructs an error envelope with an
   identifier that is not a registered code constant (a local constant, a
   variable, a computed string, or a code constant whose value is in neither
   registry list), **When** the test suite runs, **Then** the drift guard
   fails and names the file, line, and expression.
3. **Given** this repository after the feature lands, **When** the test suite
   runs, **Then** the drift guard reports zero violations.
4. **Given** an emit site that sits behind a build constraint or a platform
   file suffix, **When** the drift guard runs, **Then** that site is checked
   too, regardless of the configuration the test itself is built under.

---

### User Story 2 - Adopters match every ax-go code by name (Priority: P1)

A Go adopter, or a thin consumer that links only the import-isolated
`contract` package, wants to compare an error's code against a named value
instead of retyping the spelling, for every code ax-go can return, not only the
config and `--strict` codes published so far.

**Why this priority**: A named value removes typo risk at every adopter match
site, and it is the registry the drift guard enforces.

**Independent Test**: Trigger each ax-go runtime failure (mode validation,
confirmation blocked, internal fallback, `--strict` escalation, each config
failure) and compare the envelope's `error_code` with the matching published
constant.

**Acceptance Scenarios**:

1. **Given** any runtime `error_code` ax-go emits, **When** an adopter looks
   for its name in `contract`, **Then** exactly one exported constant holds
   that spelling.
2. **Given** the authoring-time code `invalid_schema_declaration`, **When** an
   adopter looks for its name in `contract`, **Then** it is published as its
   own exported constant, and it is not in the known-code list.

---

### User Story 3 - The agent-facing contract does not move (Priority: P1)

An agent driving an ax-go CLI reads `__schema` and matches envelopes by their
`error_code` and exit code. This feature is pure enforcement, so the agent sees
no difference.

**Why this priority**: Code spellings, exit codes, and `known_codes` are frozen
public contract (Principle XI). A refactor that moves any of them breaks every
agent that matches on them.

**Independent Test**: Run every existing `__schema`, `ax.Error`, and
envelope golden test, unchanged, and run each failure path's exit-code test.

**Acceptance Scenarios**:

1. **Given** any command tree, **When** `__schema` is emitted, **Then** the
   output is byte-identical to the output before this feature, including the
   nine-entry `known_codes` array.
2. **Given** each existing failure path, **When** it is triggered, **Then**
   the envelope's `error_code` spelling and the process exit code are
   unchanged.

---

### User Story 4 - The registry and its exclusion are self-explaining (Priority: P2)

A maintainer reading the code registry wants to know why
`invalid_schema_declaration` is not in `known_codes` and where a new code
belongs, without reading specs.

**Why this priority**: The exclusion is deliberate (spec 032 FR-001). A
reader who does not see why will "fix" it by moving the code into the list.

**Independent Test**: Read the registry source. The authoring-time list is
separate from the runtime list and carries a comment stating why its codes
are excluded.

**Acceptance Scenarios**:

1. **Given** the registry source, **When** a maintainer reads it, **Then** the
   runtime list and the authoring-time exclusion list are separate, and the
   exclusion's comment states the spec 032 FR-001 reason.
2. **Given** a code constant in the registry, **When** the registry's own
   consistency test runs, **Then** that constant is in exactly one of the two
   lists, and every list entry is a published constant.

---

### Edge Cases

- **Pass-through constructors.** The root `ax.NewError` is a public
  constructor that forwards its caller's `code` parameter to
  `contract.NewError`. It is the only pass-through site in the library, and the
  guard exempts it by an explicit, reviewed entry that names the file and the
  enclosing function. Any other site that forwards a variable fails.
- **Development gate tools.** The commands under `internal/cmd/` define their
  own codes (`surface_drift`, `size_build_failed`, ...) and route them through
  a local helper that takes a `code` parameter. Spec 032 FR-001 excludes
  gate-tool codes from `known_codes`, so the guard excludes `internal/cmd/`
  wholesale.
- **Examples.** `examples/` is adopter code and its codes are adopter-defined
  (spec 032 FR-001). The guard excludes it.
- **Test files and fixtures.** `_test.go` files, `testdata/` directories, and
  nested Go modules are not library code. The guard skips them; test
  expectations keep their string literals so they pin the frozen spellings.
- **Codes set without the constructor.** An envelope can also get its code
  from a composite literal that sets the `ErrorCode` field or from an
  assignment to that field. The guard checks both forms with the same rule,
  so the constructor is not the only door.
- **The constructor taken as a value.** A reference to `NewError` that is not
  a direct call (a function value, a method expression) hides its code
  argument from a syntactic check. The guard reports it.
- **Dot imports.** A dot import of `contract` or the root package would let an
  unqualified call escape the guard. The guard reports it.
- **Pre-serialized fallback.** The MCP dispatcher writes a hand-built JSON
  envelope when marshaling the real envelope fails. Its code comes from the
  registry constant, so the spelling cannot drift from the constant.
- **A constant registered twice, or a duplicate value.** The registry
  consistency test fails when two constants share a spelling or a code
  appears in both lists.
- **A runtime code missing from `known_codes`.** If the guard finds a
  runtime emit site whose code is not registered, that is a real gap and is
  fixed by registering the code, not by exempting the site. The survey for
  this feature found none.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every `error_code` that ax-go library code emits MUST have
  exactly one exported constant in the import-isolated `contract` package
  whose value is that spelling. This adds constants for
  `confirmation_required`, `internal_error`, `validation_error`, and
  `invalid_schema_declaration`; the five config codes and
  `warnings_as_errors` already have one.
- **FR-002**: The runtime registry MUST hold exactly the nine codes spec 032
  FR-002 lists, each named by its constant and never by a literal.
  `KnownErrorCodes()` MUST return that registry, byte-wise sorted, without
  duplicates, as a new slice on every call.
- **FR-003**: `invalid_schema_declaration` MUST be held in a separate
  authoring-time exclusion list in the same registry source, with a comment
  stating why it is excluded (it is returned while the command tree is built,
  before command dispatch, per spec 032 FR-001). The exclusion list MUST be
  disjoint from the runtime registry.
- **FR-004**: Every site in library code that sets an envelope's code MUST
  name a registered constant directly at the site. In particular:
  `execute.go`, `confirm.go`, `contract/error.go`'s non-envelope fallback,
  `internal/mcpserver` (`dispatch.go`, `server.go`, `transport.go`),
  `internal/schema` (`declare.go`, `conflict_error.go`), `mcp/command.go`,
  `schema/schema.go`, and `schema/declarations.go`.
- **FR-005**: The duplicate unexported `contractErrorWarnings` constant in
  the root package MUST be removed; its use sites MUST name
  `contract.ErrorCodeWarningsAsErrors`. The unexported
  `invalidDeclarationCode` constant in `schema` MUST likewise be replaced by
  the published constant.
- **FR-006**: The repository MUST contain an automated drift guard, run by
  the normal test suite, that parses every non-test Go source file in this
  module (ignoring build constraints, so every configuration and platform is
  covered) and reports each of the following as a violation, with its file,
  line, and expression:
  - a call to `contract.NewError` or the root `NewError` whose code argument
    is not a registered constant;
  - a composite literal or assignment that sets an `ErrorCode` field to
    anything other than a registered constant;
  - a reference to either `NewError` that is not a direct call;
  - a dot import of `contract` or the root package.
- **FR-007**: A "registered constant" MUST mean an exported constant declared
  in `contract` whose value is in the runtime registry or the authoring-time
  exclusion list. It is referenced as `contract.<Name>` (under any import
  alias) or, inside `contract`, as `<Name>`. A string literal, a computed
  string, a variable, a parameter, or any other identifier is not registered.
- **FR-008**: The guard MUST exclude, and document why it excludes:
  `_test.go` files, `testdata/` directories, directories holding a nested
  `go.mod`, hidden directories, `node_modules`, `internal/cmd/` (gate tools,
  excluded by spec 032 FR-001), and `examples/` (adopter code). Its only
  pass-through exemption MUST be the root `NewError` constructor's forwarding
  of its own `code` parameter, recorded with a reason.
- **FR-009**: The guard MUST prove it can fail: a committed fixture MUST
  contain at least a string-literal code, an unregistered identifier, a
  constant whose value is not registered, a non-call reference to
  `NewError`, and an `ErrorCode` field set from a literal, and a test MUST
  assert the guard reports each one at its expected location.
- **FR-010**: A registry consistency test MUST assert that every exported
  `ErrorCode*` constant in `contract` is in exactly one of the two lists,
  that every list entry is the value of exactly one such constant, and that
  the two lists are disjoint.
- **FR-011**: `__schema` output, every `ax.Error` envelope, every
  `error_code` spelling, and every exit code MUST be unchanged. No golden file
  changes.
- **FR-012**: The `contract` package MUST stay import-isolated: the registry
  and constants add no new dependency to it.

### Constitution alignment

- Additive under Principle XI: the exported surface gains four untyped string
  constants in `contract`. Nothing is removed, renamed, or retyped, and no
  machine payload changes. It ships as `feat:` with no
  `breaking-change-approved` label.
- The root `ax` package gains no exported identifier, so the root surface
  audit does not change; the surface baseline records the four `contract`
  constants.
- Test first (Principle VII): the registry consistency test and the drift
  guard's fixture test land before the registry and emit-site changes, and
  the guard is seen failing on this repository's literal sites before they
  are converted.
- No new dependency: the guard uses the standard library's Go parser.

### Key Entities

- **Runtime registry**: the set of codes ax-go library code can emit during
  a command run (spec 032 FR-001); published as `known_codes`.
- **Authoring-time exclusion list**: codes ax-go returns only while the
  command tree is built; registered so emit sites can name them, excluded from
  `known_codes`.
- **Code constant**: an exported `contract` constant whose value is a code in
  one of the two lists.
- **Emit site**: a place in library code that sets an envelope's
  `error_code`: a constructor call, an `ErrorCode` composite-literal field, or
  an assignment to that field.
- **Violation**: an emit site whose code expression is not a registered
  constant, or a construct that hides an emit site from the guard.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 0 emit sites in library code pass a string literal or an
  unregistered identifier as a code, down from 21 today (19 string literals
  and 2 call sites that name an unexported local constant).
- **SC-002**: The drift guard reports 100% of the violation kinds in its
  fixture (at least 5 kinds) at their exact file and line, and 0 violations on
  this repository.
- **SC-003**: 0 golden files change, `known_codes` still lists exactly 9
  codes, and 0 code spellings or exit codes change.
- **SC-004**: All 10 codes ax-go library code returns (9 runtime, 1
  authoring-time) have a published constant, up from 6.
- **SC-005**: Adding a new runtime code later takes exactly one registry
  edit (the constant plus its list entry); forgetting either fails the test
  suite.

## Assumptions

- Source inputs: GitHub issue #285 and no governing ADR. Spec 032
  (`specs/032-known-config-codes/`) supplies the FR-001 definition of a
  runtime code that this feature enforces.
- The issue's emit-site list was re-derived from the source. It missed
  `internal/schema/conflict_error.go` (two sites), two of the five
  `internal/mcpserver/server.go` sites, and the hand-built JSON fallback in
  `internal/mcpserver/dispatch.go`. Its line numbers were off for
  `schema/schema.go` and `schema/declarations.go`. FR-004 lists the files
  found by the survey.
- The survey found no runtime code missing from `known_codes`: the only
  non-config codes emitted by library code are `confirmation_required`,
  `internal_error`, `validation_error`, `warnings_as_errors`, and the
  authoring-time `invalid_schema_declaration`.
- The new codes proposed by #130 and #125 are out of scope; they register
  through this registry when they land.
- An adopter-facing API for registering custom codes is out of scope.
- The surface audit `specs/023-internalize-helpers/public-surface-audit.json`
  covers the root package only, so it does not change (as in spec 032 R4).

### Agent decisions for operator review

The operator was not available during this run. Each decision below takes the
smallest additive option; the alternatives are recorded so the reviewer can
overturn one before merge.

1. **Constant names.** The new constants transliterate the spelling exactly:
   `ErrorCodeConfirmationRequired`, `ErrorCodeInternalError`,
   `ErrorCodeValidationError`, `ErrorCodeInvalidSchemaDeclaration`, matching
   `ErrorCodeWarningsAsErrors` and the config constants, so a name can be
   derived from the spelling and back. Alternative: drop the repeated word
   (`ErrorCodeInternal`, `ErrorCodeValidation`), which reads better but breaks
   the derivation rule and sits next to the `ExitInternal` and
   `ExitValidation` exit-code constants with a different meaning.
2. **Authoring-time list is unexported.** The exclusion list is an
   unexported function in `contract`, read by the registry tests, and only
   `ErrorCodeInvalidSchemaDeclaration` is exported. Alternative: an exported
   `AuthoringTimeErrorCodes()` for adopters. Rejected for now because no
   consumer needs it and it would be a new exported function; it can be added
   later without a break.
3. **No root re-exports.** The root `ax` package does not re-export the code
   constants, as in spec 032 R3. Alternative: `ax.ErrorCode*` aliases, which
   would add root audit rows.
4. **Guard covers more than the constructor.** The guard also checks
   `ErrorCode` composite-literal fields and assignments, non-call references
   to `NewError`, and dot imports (FR-006), because the issue's first
   acceptance criterion says every code ax-go emits comes from a constant.
   Alternative: check only `NewError` call sites, as the issue's third
   acceptance criterion literally says. The extra checks find no violations in
   the repository today and live in test code, so they add no surface.
5. **Gate tools and examples are excluded wholesale** rather than checked
   against their own registries, per spec 032 FR-001. Alternative: give each
   gate tool its own registry check, which is a separate feature.
