# Feature Specification: axtest — Full-Lifecycle Command Test Helper

**Feature Branch**: `019-axtest-package`

**Created**: 2026-08-25

**Status**: Draft

**Input**: User description: "GitHub issue #178: axtest package for
full-lifecycle command execution and envelope decoding in tests. A small
public `axtest` package, importable only from test code, that (1) runs a
command tree through the real startup lifecycle so agent-safety flags like
`--dry-run` and `--yes` are mounted the same way they are in production, and
(2) decodes a command's JSON output into a caller-supplied type without a
hand-declared wrapper struct."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Exercise the real startup lifecycle in a test (Priority: P1)

A developer is testing a command built on ax-go. They want to invoke their
command exactly the way a real user or agent would — with the agent-safety
flags mounted, the output mode resolved, and the run context populated — and
get back what the command produced: its machine-readable output, its
diagnostic output, and its exit code. Today, calling their command tree
directly bypasses all of that setup, so a flag like `--dry-run` doesn't
exist yet and the test fails with an unrelated "unknown flag" error instead
of testing what it meant to test.

**Why this priority**: This is the friction blocking every other testing
concern. Without a way to run the real lifecycle, a developer cannot
reliably test any agent-safety behavior at all, and this is the failure
mode with concrete production evidence (a downstream adopter's test broke
on exactly this). Shipped alone, this already unblocks correct testing.

**Independent Test**: Build a toy command tree with a handler, run it
through the helper with `--dry-run`, and confirm the returned output
reflects a dry run and the exit code is the documented success code — using
only the public helper, no reading of the library's internals required.

**Acceptance Scenarios**:

1. **Given** a command tree that has never been executed before, **When** a
   test runs it through the helper with `--dry-run`, **Then** the run
   succeeds, the agent-safety flags are recognized, and the returned output
   indicates no side effects occurred.
2. **Given** a command that is gated behind confirmation, **When** a test
   runs it through the helper without granting approval, **Then** the run
   is blocked and the returned exit code matches the documented
   permission/validation outcome for a blocked confirmation.
3. **Given** the same gated command, **When** a test runs it through the
   helper with approval granted, **Then** the run proceeds and the returned
   exit code matches the documented success outcome.
4. **Given** a test that does not supply an idempotency key, **When** the
   command runs, **Then** the run still succeeds exactly as it would in
   production, with a key generated automatically.
5. **Given** a command that fails validation, **When** a test runs it
   through the helper, **Then** the helper returns normally (it does not
   panic or abort the test process) so the test can assert on the failure.

---

### User Story 2 - Decode a command's result without a wrapper struct (Priority: P2)

A developer has a command's captured output and wants to check the value it
produced. Today they must declare a small struct in their test file whose
only purpose is to unwrap that value from the surrounding envelope, and
repeating this in every test file risks that struct's name colliding with
an unrelated type already in the package.

**Why this priority**: This removes a recurring, error-prone chore with
production evidence of real name collisions, but it is only valuable once
Story 1 supplies something to decode — hence P2.

**Independent Test**: Take a captured output value with a known shape,
decode it into a plain caller-defined type using only the public helper,
and confirm the decoded value matches what was captured, with no
intermediate type declared in the test.

**Acceptance Scenarios**:

1. **Given** captured output from a successful run, **When** a test decodes
   it into its own result type, **Then** the decoded value matches the data
   the command produced, without the test declaring any intermediate type.
2. **Given** captured output that does not match the shape the test
   expects, **When** the test attempts to decode it, **Then** the test
   fails immediately with a clear cause, rather than silently producing a
   zero-valued result.

---

### User Story 3 - Assert the common case in one step (Priority: P3)

A developer's test represents the ordinary, happy-path case: run the
command, expect success, and check the value it returned. They want to do
this in a single step rather than manually chaining the run and the decode
every time.

**Why this priority**: Pure convenience over Stories 1 and 2 — it changes
nothing about what is possible, only how much boilerplate a common
assertion takes. Valuable, but the lowest priority because the feature is
already complete without it.

**Independent Test**: Run a command known to succeed and assert on its
typed result using a single call, and confirm it behaves identically to
performing the run and the decode as two separate steps.

**Acceptance Scenarios**:

1. **Given** a command expected to succeed, **When** a test uses the
   combined helper, **Then** it receives the same typed result and exit
   code it would have received by calling the two underlying helpers
   separately.

---

### Edge Cases

- A command tree is reused across multiple runs within one test (e.g.,
  table-driven subtests sharing a root command). Mounting the agent-safety
  flags a second time must not register duplicate flags or otherwise break
  the second run.
- A command writes nothing to its machine-readable output (for example, a
  run that fails before producing a payload). Decoding must fail clearly
  rather than misinterpreting empty input as a valid, empty result.
- A dry run is combined with a confirmation-gated command. Dry-run and
  approval are independent outcomes in ax-go's execution model — a
  rehearsal of a gated action does not itself grant approval — and the
  helper must surface whichever outcome actually occurred rather than
  assuming a dry run always succeeds.
- Multiple subtests run concurrently, each against its own command tree.
  The helper must not rely on any shared or global state that would make
  concurrent use unsafe.
- A test only cares about the failure path and never intends to decode a
  result. The helper must not force every caller through decoding to get
  the run's output and exit code.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The test helper MUST run a command tree through the same
  startup sequence a production binary goes through, including mounting
  the agent-safety flags (dry-run, no-prompt approval, output format,
  idempotency key), resolving the output mode, and populating the run
  context — so a test never has to replicate that wiring by hand.
- **FR-002**: The test helper MUST return, from one run: the command's
  captured machine-readable output, its captured diagnostic output, and
  its exit code — as three independently inspectable results, so a test
  can assert on success and failure paths alike without discarding
  information it might need.
- **FR-003**: The test helper MUST behave correctly regardless of whether
  the command under test performs a dry run, is gated behind confirmation,
  is invoked with approval granted, or is invoked with approval withheld —
  returning whatever outcome actually occurred rather than assuming
  success.
- **FR-004**: The test helper MUST NOT require the caller to supply an
  idempotency key; when one is omitted, the same automatic generation that
  occurs in production MUST occur.
- **FR-005**: A separate decode helper MUST unwrap a command's captured
  machine-readable output into a caller-supplied result type, without the
  caller declaring an intermediate wrapper type to reach that value.
- **FR-006**: The decode helper MUST fail the calling test immediately and
  with a clear cause when the captured output does not conform to the
  expected shape, rather than returning a zero-valued result silently.
- **FR-007**: A combined helper MUST be available that performs the run and
  the decode as a single step, for the common case of a command expected to
  succeed, without requiring the caller to also use the two helpers
  separately to get the same outcome.
- **FR-008**: Mounting the agent-safety flags on a command tree that has
  already had them mounted (for example, by an earlier run in the same
  test) MUST be safe and MUST NOT duplicate flag registration.
- **FR-009**: None of the test helpers MUST be reachable from, or change
  the behavior, size, or dependency graph of, any code that ships in a
  production binary. An automated check MUST verify that no non-test
  source file anywhere in this module imports the test helpers, so this
  guarantee is enforced rather than assumed.
- **FR-010**: The helpers MUST be documented, with a runnable example, as
  the canonical way to test a command built on ax-go, replacing the
  previously undocumented pattern of calling the library's execution entry
  point directly from test code.

### Key Entities

- **Execution result**: What one full-lifecycle run of a command tree
  produces — its captured machine-readable output, its captured diagnostic
  output, and its exit code — considered together because a test may need
  any combination of the three to make its assertion.
- **Typed result**: A caller's own result type, populated by decoding the
  data an execution result's machine-readable output carries, with the
  enclosing envelope already removed.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can write a test that correctly exercises
  dry-run and confirmation-gated behavior using only the documented helper
  and its example — with no need to read the library's execution-entry-point
  source to discover the correct call pattern.
- **SC-002**: A test decoding a command's result needs zero hand-declared
  wrapper types, eliminating the class of naming collision observed in
  production use.
- **SC-003**: Existing ax-go consumers — every production binary built on
  the library today — see no change in behavior, binary size, or
  dependency graph as a result of this feature's existence, verified
  automatically rather than by inspection.
- **SC-004**: A test using the helpers against a confirmation-gated command
  can distinguish an approved run, a blocked run, and a dry run from one
  another, with each outcome matching its documented exit code.
- **SC-005**: The new package meets the same documentation, testing, and
  quality bar already required of every other officially supported ax-go
  package, at parity on first release.

## Out of Scope

- Changing the behavior of the library's existing execution entry point.
  This feature is purely additive test tooling.
- Schema- or MCP-tool-level acceptance testing for agents (a separate,
  already-tracked concern).
- A general mocking or stubbing framework for ax-go's telemetry, logging,
  or other runtime primitives. The helper exercises the real lifecycle; it
  does not fake it.
- Assertion or matcher utilities beyond decoding. A caller uses whatever
  testing or assertion library they already prefer on the values the
  helpers return.

## Assumptions

- Source inputs: GitHub issue #178. No governing ADR.
- The organizational precedent set by ax-go's existing import-isolated
  public packages (contract/config/schema/id, and logging) is followed for
  discoverability — a small, dedicated public package rather than adding
  test helpers to the root package — but the *motivation* differs. Those
  packages exist to shrink a production binary's dependency graph; this one
  exists only to be a stable, documented place to find test tooling, since
  it is never linked into a production binary regardless of what it
  depends on internally.
- A caller is expected to construct a fresh command tree per test case (the
  ordinary pattern for testing a command-line tool), so flag *values*
  persisting on a reused tree across calls is treated as ordinary,
  well-understood behavior of the underlying command framework rather than
  something this feature must guard against; FR-008 covers only safe
  re-mounting of the flags themselves, not resetting values a caller set.
- The combined run-and-decode helper is documented as intended for the
  success path; a test targeting a failure path uses the run helper alone
  and asserts on the exit code and diagnostic output directly.
