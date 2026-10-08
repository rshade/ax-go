# Feature Specification: Unread struct-field gate (slopcheck)

**Feature Branch**: `031-slopcheck-unread-fields`

**Created**: 2026-10-08

**Status**: Draft

**Input**: User description: "Issue #232: slopcheck — a type-aware gate that
reports struct fields assigned in a composite literal but never read. Build it
in ax-go. Harness: both a stdlib driver that owns the gate's stream and exit
contract and a standard analyzer adapter tested with analyzer fixtures, sharing
one harness-agnostic core. Blocking gate in `make ci` and the CI validate job;
fixes for real findings land in a separate follow-up PR. Exported fields of
exported types are skipped."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A field nothing reads fails the gate (Priority: P1)

A maintainer (or a coding agent) adds a table-driven test whose case struct
declares a field such as `expectError bool`. Every case sets it, and the loop
body never consults it. The suite passes, coverage rises, and today nothing in
`make ci` notices. With this feature, the gate run fails and names the field,
where it is declared, and where it is assigned.

**Why this priority**: This is the whole reason the feature exists. It is the
one machine-written-test fingerprint that no linter in the pipeline can see:
`unused` treats a write in a literal as a use, `structcheck` was retired, and
the structural `make slop` report has no type information.

**Independent Test**: Add a throwaway unread field to a real table test, run
the gate, confirm it fails and names that field, remove the field, confirm the
gate passes.

**Acceptance Scenarios**:

1. **Given** a keyed composite literal that sets a field no code ever reads,
   **When** the gate runs, **Then** it fails with exit `2` and one `ax.Error`
   envelope on stderr whose findings name the field's declaration position and
   every literal position that assigns it.
2. **Given** a field set in a literal and read in a loop body, **When** the
   gate runs, **Then** that field is not reported.
3. **Given** a field whose only read is taking its address (`&x.F`), **When**
   the gate runs, **Then** it is not reported.
4. **Given** a field whose only "read" is a self-assignment (`x.F = x.F`),
   **When** the gate runs, **Then** it is reported.
5. **Given** a field whose only use after the literal is a compound update
   (`x.F += 1`, `x.F++`), **When** the gate runs, **Then** it is not reported.
6. **Given** a positional composite literal of a type whose fields are never
   read, **When** the gate runs, **Then** each of those fields is reported,
   because a positional literal assigns every field.
7. **Given** a field of an embedded struct, read only through the outer type
   by promotion, **When** the gate runs, **Then** it is not reported.

---

### User Story 2 - The gate speaks the repository's machine contract (Priority: P1)

The maintainer and CI consume this gate exactly as they consume the seven
existing `internal/cmd` gates: one JSON document on stdout on a pass, one
`ax.Error` envelope on stderr on any failure, and a deterministic exit code.

**Why this priority**: A gate that prints free text cannot be consumed by the
agents this repository exists to serve, and an inconsistent gate is one the
maintainer has to learn separately.

**Independent Test**: Run the gate against a clean fixture tree and a failing
fixture tree, and byte-compare stdout, stderr and the exit code of each
against committed golden files.

**Acceptance Scenarios**:

1. **Given** no findings, **When** the gate runs, **Then** it writes exactly
   one minified JSON document to stdout, zero bytes to stderr, and exits `0`.
2. **Given** at least one finding, **When** the gate runs, **Then** it writes
   zero bytes to stdout, exactly one minified `ax.Error` envelope to stderr,
   and exits `2`.
3. **Given** a package that does not type-check, invalid flags, or unexpected
   positional arguments, **When** the gate runs, **Then** it fails closed with
   an envelope distinct from the findings code and never reports a pass.
4. **Given** the same tree, **When** the gate runs twice, **Then** both runs
   are byte-identical on stdout and stderr.

---

### User Story 3 - The same check runs as a standard analyzer (Priority: P2)

The check is also available as a standard Go analyzer so it can be exercised
with the analyzer ecosystem's fixture tooling (`// want` annotations) and,
later, hosted by a linter runner. Both front ends share one core, so they can
never disagree about what is a finding.

**Why this priority**: The maintainer chose both harnesses. The analyzer gives
idiomatic fixtures and a future lint-plugin path. It is not the gate; the
stdlib driver is.

**Independent Test**: Run the analyzer over the acceptance fixtures with the
analyzer test tooling, then run the stdlib driver over the same fixtures, and
confirm both report the identical set of fields.

**Acceptance Scenarios**:

1. **Given** each acceptance fixture from User Story 1, **When** the analyzer
   adapter runs, **Then** it reports exactly the fields the fixture's `// want`
   annotations name, and nothing else.
2. **Given** the same fixtures, **When** both front ends run, **Then** they
   report the same field set.

---

### User Story 4 - The gate is enforced, not optional (Priority: P2)

`make slop-check` exists, `make ci` runs it, and the CI `validate` job runs
it, so a pull request that adds an unread field cannot merge green.

**Why this priority**: The maintainer chose a blocking gate over a report.
A detector nobody is required to run decays the way the `make slop` report's
findings do.

**Independent Test**: Inspect `make ci` and the `validate` job for the target,
and confirm a run with a planted unread field fails CI.

**Acceptance Scenarios**:

1. **Given** the repository, **When** `make ci` runs, **Then** it runs the
   gate and fails if the gate fails.
2. **Given** a pull request, **When** the CI `validate` job runs, **Then** it
   runs the gate as its own step.

### Edge Cases

- **A field read only under a declined build configuration** (for example,
  only under `ax_no_grpc`): not reported. A field is reported only when it is
  unread in every one of the four build-tag configurations in which it is
  assigned.
- **A field read only from test code**: not reported. Tests are readers, as
  they are roots for `deadcheck`.
- **Exported field of an exported type**: skipped. A library's exported field
  may be written here and read only by a downstream consumer.
- **Exported field of an unexported type, or any field of an anonymous struct
  type** (the usual table-test shape): in scope.
- **A struct value consumed whole** — compared with `==`/`!=`, converted to an
  interface (including formatting, encoding, or reflection-based comparison),
  or passed to code outside the analyzed package: every field of that value
  counts as read. The gate cannot see a reflection-based reader, so it must not
  guess that there is none.
- **A type reachable from the external test package through an exported
  alias**: its fields count as readable from outside the package and are not
  reported.
- **Blank (`_`) fields and fields declared in generated files** (`// Code
  generated ... DO NOT EDIT.`): never reported.
- **A field declared in another package** (assigned here, declared there):
  not reported by this package's analysis; the declaring package owns it.
- **A field written only by assignment statements, never in a composite
  literal**: out of scope; it is not a candidate.
- **A package that fails to load or type-check**: the gate fails closed with
  the analysis-failure envelope, never a pass and never a partial findings
  list.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The gate MUST report every in-scope struct field that is
  assigned in at least one composite literal (keyed or positional) and read in
  no load position anywhere in its declaring package, including that package's
  test files.
- **FR-002**: The analysis MUST classify each field selector as a load, a
  store, or both: a plain assignment target is a store only; a compound
  assignment or increment/decrement is both; taking the address, passing as an
  argument, and any other value use are loads; a self-assignment (`x.F = x.F`)
  is not a read.
- **FR-003**: A positional composite literal MUST mark every field of its type
  as assigned.
- **FR-004**: A field read through promotion from an embedding type MUST count
  as a read of the embedded field.
- **FR-005**: Any use of a struct value, or of a value containing one (a
  pointer, slice, array, map key or element, or channel of it), outside a
  closed set of harmless positions MUST count as reading every field of that
  struct type. The harmless positions are ordinary copies and accesses:
  selector base, assignment (including to `_`), range, composite-literal
  element, an argument to a statically resolved same-package function, and the
  return of an unexported function (the full list is in the plan's research).
  Uses outside that set include equality comparison, interface conversion,
  calls to functions outside the package, dynamic calls, and returns from
  exported functions.
- **FR-006**: Exported fields of exported types, blank fields, fields declared
  in generated files, and fields declared outside the analyzed package MUST
  NOT be reported.
- **FR-007**: The gate MUST analyze all four build-tag configurations
  (default, `ax_no_grpc`, `ax_no_otlp`, both) on the host platform and report a
  field only when it is a finding in every configuration in which it is
  assigned. The configuration list MUST be a Go constant kept in sync with the
  Makefile's `BUILD_TAG_MATRIX` by a test, as `deadcheck` does. Here "Go constant" means
  constant policy data in Go source (a function returning a fresh slice, as
  `deadcheck.buildTags`), since a package-level slice would be mutable state.
- **FR-008**: A pass MUST write exactly one minified JSON document to stdout
  and nothing to stderr, exiting `0`. Every failure MUST write nothing to
  stdout and exactly one minified `ax.Error` envelope to stderr.
- **FR-009**: Each failure class MUST carry its own stable error code and exit
  code: findings (exit `2`), invalid flags or arguments (exit `2`), package
  load or type-check failure (exit `2`), analysis timeout (exit `3`),
  permission denial (exit `4`), and unexpected internal failure (exit `1`).
- **FR-010**: Findings MUST be emitted in a deterministic order and identify,
  per field, the package, the type, the field name, the declaration position,
  and each assigning literal position, using module-relative paths.
- **FR-011**: The analysis MUST live in one harness-agnostic core consumed by
  two front ends: a stdlib driver that is the gate and owns the stream and exit
  contract, and a standard analyzer adapter. Neither front end may contain
  classification logic of its own.
- **FR-012**: The analyzer adapter MUST be tested with the analyzer
  ecosystem's fixture tooling (`// want` annotations) over the acceptance
  fixtures, and a test MUST assert that both front ends report the same field
  set over those fixtures.
- **FR-013**: Every package of this feature MUST stay under `internal/`; none
  may be exported public API, and the exported-surface baseline MUST NOT change.
- **FR-014**: Any policy (configuration list, excluded packages, any allowlist)
  MUST be a Go constant, never an external configuration file. No allowlist is
  shipped by default.
- **FR-015**: `make slop-check` MUST run the gate, `make ci` MUST include it,
  and the CI `validate` job MUST run it as its own step.
- **FR-016**: The gate's package doc comment MUST follow the sibling gates'
  header: what it checks, why no existing linter does, what it cannot prove,
  the harness decision and its reasoning, the build-configuration rule, the
  exit codes, and the exact invocation from the module root.
- **FR-017**: The gate MUST bound each configuration's analysis with a
  timeout and honor cancellation.
- **FR-018**: The implementation PR MUST record, in its description, the
  finding count of a whole-repository run and a triage verdict (real defect or
  false positive, with reason) for each finding.

### Key Entities

- **Candidate field**: a struct field declared in the analyzed package and in
  scope under FR-006; identified by its declaring type and name.
- **Assignment site**: a composite-literal position that sets a candidate
  field, keyed or positional.
- **Read site**: a load-position use of a candidate field, or a whole-value
  use that counts as reading all of its fields.
- **Finding**: a candidate field with at least one assignment site and no read
  site in any build configuration in which it is assigned.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All seven acceptance shapes from User Story 1 are classified as
  specified, each by its own fixture case, through both front ends.
- **SC-002**: A throwaway unread field planted in a real table test is
  reported on the first run; removing it restores a pass.
- **SC-003**: Both front ends agree on 100% of fixture fields.
- **SC-004**: Pass and failure output match committed golden files byte for
  byte; two consecutive runs on an unchanged tree are byte-identical.
- **SC-005**: A whole-repository run completes within each configuration's
  timeout on the CI runner, and its finding count and per-finding triage are
  recorded in the PR.
- **SC-006**: `validate`, `test`, the full build-tag lint matrix,
  `go mod tidy -diff`, `doc-coverage`, `surface-check`, `size-check`,
  `dead-check`, `security`, `cover-check` and `slop-check` all pass, and the
  new packages clear the default per-package coverage floor. `bench-check` (the
  rest of `make ci`) is not triggered: no tracked hot path changes.
- **SC-007**: The isolated `logging` binary's size is unchanged, because no
  new dependency reaches a public package.

## Assumptions

- Source inputs: GitHub issue #232 and no governing ADR.
- Home repository: the maintainer settled the open disagreement with
  `HavenTrack/haventrack#529` in favor of ax-go. Closing that ticket as a
  duplicate is a maintainer action outside this feature.
- Harness: the maintainer chose both front ends. The analyzer adapter makes
  `golang.org/x/tools` a direct `go.mod` requirement; it is already in the
  module graph through the MCP SDK, so no new module enters the graph, and
  `internal/` keeps it out of every public package's import set.
- Gate versus report: the maintainer chose a blocking gate, with fixes for any
  real findings landing in a separate follow-up PR. If the whole-repository
  triage (FR-018) finds a real defect, a gate wired into `make ci` would fail
  this PR, so implementation pauses for a maintainer decision before wiring
  rather than either fixing findings here or weakening the gate.
- Platform: like `deadcheck`, the gate analyzes the host `GOOS`/`GOARCH` only;
  a field read only under another platform's build constraint can be reported.
- The analysis is package-scoped. Given the exported-field skip, a field in
  scope cannot be read from another package except through an exported alias,
  which FR-005/edge cases treat as a read.
- Out of scope: exporting the analyzer as public API, fixing findings in this
  repository, unused locals/parameters/identifiers (already covered),
  type-free structural rules (#229), whole-program reachability (#227), and
  mutation testing (#234).
