# Feature Specification: --yes no-prompt invariant (confirmation_required envelope)

**Feature Branch**: `018-yes-no-prompt-invariant`

**Created**: 2026-07-28

**Status**: Draft

**Input**: User description: "Add `--yes` as the fourth auto-mounted agent-safety
primitive and enforce a no-prompt invariant (GitHub issue #121). In agent mode an
ax-go CLI must never block on a stdin prompt, open a pager, spawn `$EDITOR`, or
animate a spinner. Any confirmation point that would otherwise block fails fast
with exit 2 and a structured `confirmation_required` envelope naming the bypass
flag. With `--yes`, the same confirmation point proceeds with no prompt."

## Clarifications

### Session 2026-07-28

- Q: The issue left open whether the bypass hint needs a new `hint` field on the
  error envelope. → A: **No new field.** The envelope already carries an
  `actionable_fix` remediation field with a supported construction option, and the
  bypass instruction is exactly that kind of remediation. The
  `confirmation_required` envelope populates `actionable_fix`. The error envelope's
  shape therefore does **not** change at all; this feature is additive only in (a)
  one new auto-mounted flag and (b) one new `error_code` string value, both of which
  are additive-tolerant under Constitution Principle XI.
- Q: An MCP `tools/call` is non-interactive by construction. Should the MCP server
  runtime force approval, ignore the flag, or thread it? → A: **Thread it as a
  per-call tool argument**, mirroring how dry-run already flows through the
  dispatcher. The calling MCP client decides per call. Rejected: forcing approval
  for every MCP call (erases the safety signal on the most agent-facing transport,
  so no MCP client could ever observe a `confirmation_required` result), and leaving
  the dispatcher untouched (every confirmation-gated command becomes permanently
  unreachable over MCP, with no bypass available, until runtime elicitation ships).
- Q: In human-interactive mode with no approval granted, does the gate report a
  distinct third outcome, or collapse to approved? → A: **A distinct third outcome.**
  The gate reports exactly one of three states — approved, blocked, or
  prompt-required — and the CLI author handles prompt-required by writing their own
  prompt. Rejected: collapsing human mode to approved, which would make the gate a
  silent no-op for human operators and quietly delete the confirmation an adopting
  CLI relied on, for exactly the users a confirmation is meant to protect. The cost
  of the chosen answer is one additional branch at each call site.
- Q: Does an active dry-run imply approval, since no side effect can occur? → A:
  **No — the two primitives are orthogonal.** Dry-run governs the side effect;
  approval governs the decision. A dry-run of a gated command still yields
  `confirmation_required`, so the rehearsal faithfully predicts the real run. The
  preview idiom for a gated command is therefore both flags together. Rejected:
  auto-approving under dry-run, which would let a rehearsal succeed where the real
  run fails — contradicting the established dry-run contract that a dry-run emits the
  same envelope as a real run and surfaces the same failures a real run would.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A non-interactive run never hangs (Priority: P1)

An LLM agent operates an ax-go CLI with no human at the keyboard. It invokes a
command that reaches a point the CLI author considers destructive enough to
confirm. Today an author who writes a confirmation prompt there produces the worst
possible agent failure: no error, no timeout, no output — just a process blocked on
a stdin read that will never be satisfied, a dead agent turn, and a burned operator.
The agent wants that point to either be pre-approved or to fail immediately with a
machine-readable explanation naming exactly what to pass to proceed.

**Why this priority**: This is the entire point of the feature and the smallest
slice that delivers it. A blocked prompt is unrecoverable from the agent's side;
converting it into a typed, exit-coded refusal is what makes an ax-go CLI safe to
hand to an agent at all. It stands alone: with only this slice, every gated
confirmation is already non-blocking and already bypassable.

**Independent Test**: Build a command that calls the confirmation gate before a
recorded side effect. Run it in machine mode without the approval flag and assert
the process exits 2, writes exactly one `confirmation_required` envelope to stderr,
writes zero bytes to stdout, records no side effect, and never reads stdin. Run the
same command with the approval flag and assert it exits 0 and records the side
effect.

**Acceptance Scenarios**:

1. **Given** a command with a confirmation gate running in machine mode without the
   approval flag, **When** it reaches the gate, **Then** it does not block on input,
   exits with code 2, and emits one structured `confirmation_required` envelope to
   stderr.
2. **Given** that same blocked run, **When** its streams are inspected, **Then**
   stdout has received zero bytes and the gated side effect did not occur.
3. **Given** the emitted `confirmation_required` envelope, **When** an agent reads
   it, **Then** its remediation field names the approval flag the agent must pass to
   proceed.
4. **Given** a command with a confirmation gate and the approval flag supplied,
   **When** it reaches the gate, **Then** the gate reports approval, no prompt is
   presented, the side effect occurs, and the command exits 0.
5. **Given** the approval flag is supplied, **When** the run is compared against a
   run of the same command with no confirmation gate at all, **Then** the stdout
   payload is byte-identical apart from fields already documented as
   non-deterministic.

---

### User Story 2 - An MCP client approves a call it initiated (Priority: P2)

An agent driving an ax-go CLI through its MCP server runtime calls a tool whose
underlying command is confirmation-gated. Because an MCP `tools/call` has no human
at the keyboard by construction, the call must not hang and must not silently
self-approve. The agent wants to make the approval decision explicitly, per call, in
the same way it already chooses dry-run per call — and to receive the same
structured refusal when it did not.

**Why this priority**: MCP is the most agent-facing transport ax-go ships, so a
confirmation gate that is unreachable or auto-approved there defeats the feature for
its primary audience. It ranks below US1 because it is a second surface over the
same gate rather than the gate itself, and it is independently testable once the
gate exists.

**Independent Test**: Register a confirmation-gated command as an MCP tool. Call it
with no approval argument and assert the result is a structured
`confirmation_required` failure with the validation exit code, not a hang and not a
success. Call it again with the approval argument set true and assert it succeeds
and performs the side effect.

**Acceptance Scenarios**:

1. **Given** a confirmation-gated command exposed as an MCP tool, **When** a client
   calls it without the approval argument, **Then** the call returns a structured
   `confirmation_required` failure carrying the validation exit code, and does not
   block.
2. **Given** the same tool, **When** a client calls it with the approval argument set
   to true, **Then** the approval reaches the command's context, the gate reports
   approval, and the call succeeds.
3. **Given** the same tool, **When** a client passes a non-boolean value for the
   approval argument, **Then** the call fails as a validation error, consistent with
   how the dry-run argument already rejects non-boolean values.
4. **Given** two sequential MCP calls where the first supplies approval and the
   second does not, **When** the second runs, **Then** it is not approved — no
   approval state leaks between calls.

---

### User Story 3 - A human-operated CLI keeps its confirmation (Priority: P3)

A developer ships one CLI used both by agents and by human operators at a terminal.
They do not want the agent-safety invariant to quietly delete the confirmation
prompt their human users rely on before a destructive action. They want the gate to
tell them, unambiguously, which of the three situations they are in: approval was
explicitly granted, approval is impossible to obtain here and the command must stop,
or a human is present and the author may ask them.

**Why this priority**: It prevents the feature from degrading human-facing CLIs and
makes the gate's contract honest in all resolved modes rather than only the agent
one. It ranks below US1 and US2 because a human at a terminal can always recover
from a bad outcome by watching it happen, whereas an agent cannot.

**Independent Test**: Call the gate with human mode resolved and no approval flag,
and assert the outcome is distinguishable from both the approved outcome and the
blocked outcome without inspecting an error message string.

**Acceptance Scenarios**:

1. **Given** human mode resolved and no approval flag, **When** the author calls the
   gate, **Then** the result is distinguishable from both "approved" and "blocked",
   and no `confirmation_required` envelope is produced.
2. **Given** human mode resolved and the approval flag supplied, **When** the author
   calls the gate, **Then** the result is "approved" and the author's prompt is
   skipped.
3. **Given** the gate itself, **When** it runs in any mode, **Then** it never reads
   stdin, never writes to stdout, never opens a pager, never spawns an editor, and
   never renders animation — any actual prompt remains the author's own code.

---

### User Story 4 - The primitive is discoverable and learnable (Priority: P3)

An agent introspecting an ax-go CLI through its schema command needs to discover the
approval flag the same way it discovers the other three agent-safety primitives, so
it can pass the flag without out-of-band knowledge. A developer adopting ax-go needs
a verified, runnable example of the gate rather than prose.

**Why this priority**: The refusal envelope names the flag, so an agent can recover
even without schema discovery; this slice removes the need for a failed round trip
first. It depends on US1 existing and is polish rather than core behavior.

**Independent Test**: Invoke the schema command on a CLI that declared no flags of
its own and assert the approval flag appears in the reflected flag set with its type
and default. Confirm the gate's runnable example compiles and executes in the test
suite.

**Acceptance Scenarios**:

1. **Given** any ax-go CLI, **When** an agent reads its schema output, **Then** the
   approval flag appears in the reflected flag set alongside the existing three
   primitives, with no special-casing in the reflection logic.
2. **Given** the published API documentation, **When** a developer looks up the
   confirmation gate, **Then** a runnable example demonstrates it and is exercised by
   the test suite.
3. **Given** the repository's integration example command, **When** a reader inspects
   it, **Then** it demonstrates a gated confirmation as the canonical usage.

---

### Edge Cases

- **Piped stdin is a data channel, not a prompt**: The invariant targets *interactive
  blocking*, not stdin itself. A command that reads a piped payload from stdin (for
  example, JSON supplied by the caller) is doing normal machine-mode work and MUST
  keep working unchanged. The feature MUST NOT close, replace, or poison stdin, and
  MUST NOT treat a stdin read as a violation.
- **The author already declared a flag of the same name**: Auto-mounting MUST NOT
  overwrite a flag the author already declared, matching the existing behavior of
  the other three primitives.
- **Approval combined with dry-run**: The two primitives govern different things — one
  the decision, one the side effect — and stay orthogonal (FR-010). A gated command
  run with dry-run alone is still blocked; the preview idiom is dry-run *and*
  approval together, which yields a faithful preview of the approved path. Approval
  without dry-run applies for real. All four combinations are meaningful and MUST be
  tested.
- **Absent or unresolved mode in the context**: When the gate cannot determine a
  resolved mode (for example, a caller that never went through the standard execution
  wrapper, or a nil context), it MUST fail closed and treat the situation as
  non-interactive rather than assuming a human is present. It MUST NOT panic.
- **The gate is called more than once in a run**: Each call is evaluated independently
  against the same context state and yields the same answer; the gate holds no
  state and does not "remember" a previous approval.
- **Approval granted but the effect fails**: The gate governs permission only. An
  error from the caller's subsequent work is returned by the caller's own code with
  its wrap chain intact; the gate neither wraps nor reclassifies it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST auto-mount an approval flag named `yes` as a persistent
  boolean flag, defaulting to false, on every ax-go CLI, alongside the existing
  `--format`, `--dry-run`, and `--idempotency-key` primitives, without the CLI author
  declaring it.
- **FR-002**: Auto-mounting MUST NOT overwrite an identically named flag the author
  already declared, matching the established behavior of the existing three
  primitives.
- **FR-003**: The resolved approval state MUST be carried in the command's context so
  that any code holding the context can read it, mirroring how dry-run state is
  carried today. Resolution MUST happen in the same pre-run step that resolves mode,
  dry-run, and idempotency key.
- **FR-004**: The system MUST provide a public confirmation gate that a CLI author
  calls at any point that would otherwise block on human input.
- **FR-005**: When machine/agent mode is resolved and approval was NOT granted, the
  gate MUST report that the operation must not proceed, and MUST yield a structured
  error whose `error_code` is exactly `confirmation_required`.
- **FR-006**: That error MUST map to exit code 2 (validation), the fixed mapping for a
  blocked-but-required confirmation.
- **FR-007**: That error MUST carry remediation in the existing `actionable_fix` field
  naming the approval flag. The system MUST NOT add a new field to the error envelope
  for this purpose, and MUST NOT otherwise change the envelope's shape.
- **FR-008**: When approval was granted, the gate MUST report approval regardless of
  resolved mode, and MUST NOT present a prompt, open a pager, spawn an editor, or
  render animation.
- **FR-009**: The gate MUST report exactly one of three mutually exclusive outcomes:
  **approved** (approval was granted), **blocked** (non-interactive and approval was
  not granted), or **prompt-required** (human-interactive mode and approval was not
  granted). The three MUST be distinguishable by the caller without inspecting an
  error message string.
- **FR-009a**: The prompt-required outcome MUST NOT produce a
  `confirmation_required` error and MUST NOT terminate the command. It signals that a
  human is present and the CLI author may now run their own prompt; the gate itself
  still performs no prompting (FR-011).
- **FR-010**: Approval and dry-run MUST remain orthogonal. An active dry-run MUST NOT
  imply approval: a gated confirmation reached under dry-run without approval MUST
  yield the same blocked outcome, the same `confirmation_required` error code, and
  the same exit code 2 as it would without dry-run.
- **FR-010a**: A gated command's confirmation outcome MUST therefore be identical
  between a dry-run and a real run given the same approval state, so a dry-run never
  predicts success where a real run would be blocked. This preserves the existing
  guarantee that a dry-run surfaces the same failures a real run would.
- **FR-011**: The gate itself MUST NOT read stdin, write to stdout, open a pager,
  spawn an editor, or render animation, in any mode. Any actual human prompt remains
  the CLI author's own code.
- **FR-012**: A blocked confirmation MUST write its envelope to stderr only. stdout
  MUST receive zero bytes from the blocked path, preserving stream separation.
- **FR-013**: The feature MUST NOT close, replace, or otherwise interfere with stdin.
  Commands that read a piped payload from stdin MUST continue to work unchanged; the
  invariant concerns interactive blocking, not the stdin stream itself.
- **FR-014**: The approval flag MUST appear in `__schema` output through the existing
  flag-reflection path, with no special-casing added to reflection.
- **FR-015**: The MCP server runtime MUST accept the approval flag as a per-call tool
  argument, validate it as a boolean, thread it into the isolated per-call argument
  vector, and carry the resolved value into that call's context — mirroring the
  existing dry-run threading. It MUST NOT grant approval on the client's behalf, and
  MUST NOT ignore a client-supplied value.
- **FR-016**: A non-boolean value supplied for the MCP approval argument MUST be
  reported as a validation error (exit code 2), consistent with the existing
  treatment of a non-boolean dry-run argument.
- **FR-017**: Approval state MUST NOT leak between MCP calls; each dispatched call
  resolves its own approval state from its own arguments.
- **FR-018**: The gate MUST tolerate a nil or state-free context without panicking,
  and in that case MUST fail closed — treating the run as non-interactive rather than
  assuming a human is present.
- **FR-019**: The feature MUST NOT change the success envelope's shape. Surfacing a
  "this command required confirmation" signal on the success envelope is deferred to
  the separate envelope-trust-signals work.
- **FR-020**: Output determinism MUST be preserved: for a given input, the stdout
  payload stays byte-identical across runs apart from fields already documented as
  non-deterministic.
- **FR-021**: The gate MUST be demonstrated by a verified, runnable example on the
  primary API surface, and the repository's integration example command MUST
  demonstrate a gated confirmation as the canonical usage.
- **FR-022**: The feature MUST NOT introduce a new public package. The gate is added
  to the already-public root package, and the approval-state carrier joins the
  existing context carriers.

### Key Entities

- **Approval state**: A per-run boolean meaning "a human has already consented to
  whatever this command is about to do." Sourced from the auto-mounted `yes` flag,
  carried in the run's context, and — over MCP — sourced instead from the per-call
  tool argument. Defaults to not-granted.
- **Confirmation gate**: The public primitive a CLI author calls at an interaction
  point. Inputs: the run's context (carrying resolved mode and approval state) and a
  description of what is being confirmed. Output: a decision about whether the caller
  may proceed, plus the structured refusal when it may not. It is a decision oracle,
  never a prompter: it performs no I/O of its own beyond the refusal envelope.
- **`confirmation_required` envelope**: An instance of the existing error envelope
  with `error_code` set to `confirmation_required`, exit code 2, and `actionable_fix`
  naming the approval flag. It introduces no new field and is the documented
  degradation target for future runtime elicitation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In machine mode with no approval granted, 100% of gated confirmation
  points terminate rather than block — proven by tests asserting the process exits 2
  and never reads stdin, with zero hangs across the suite.
- **SC-002**: A blocked confirmation writes exactly one structured envelope to stderr
  and zero bytes to stdout, verifiable by capturing both streams independently.
- **SC-003**: An agent that receives a `confirmation_required` envelope can determine
  the exact flag needed to proceed from the envelope alone, without consulting
  documentation or the schema — the remediation field names it.
- **SC-004**: With approval granted, a gated command performs its side effect and
  exits 0, and its stdout payload is byte-identical to the same command with no gate
  apart from documented non-deterministic fields.
- **SC-005**: An MCP client can approve a call it initiated and can observe a
  structured refusal when it did not, with no approval state shared between
  consecutive calls.
- **SC-006**: The approval flag is discoverable in schema output for a CLI that
  declared no flags of its own, alongside the existing three primitives.
- **SC-007**: The error envelope's field set is unchanged by this feature — the public
  surface gate reports no added, removed, or retyped envelope member.
- **SC-008**: Commands that read a piped stdin payload behave identically before and
  after the feature, proven by a test that pipes input through a gated CLI.
- **SC-009**: The gate has a verified runnable example that compiles and executes in
  the test suite, and the documentation-coverage gate stays green.
- **SC-010**: No new public package is added; the public-package allowlist is
  unchanged.
- **SC-011**: A gated command reaches the same confirmation outcome under dry-run as
  it does for real given the same approval state — proven by a test covering all four
  approval × dry-run combinations, in which no dry-run reports success where the
  corresponding real run is blocked.
- **SC-012**: A CLI author can tell the three gate outcomes apart without parsing an
  error message, proven by a test that asserts on the outcome value itself in
  machine mode without approval, human mode without approval, and with approval.

## Assumptions

- Source inputs: GitHub issue #121. No governing ADR — the agent-safety primitives are
  governed by Constitution Principle IV (Agent-Safety Primitives) and the
  stability/additivity rules of Principle XI. ADRs are frozen and none applies here.
- "Users" of this feature are Go developers building CLIs on ax-go, and transitively
  the LLM agents that operate those CLIs and rely on never being blocked on input
  they cannot supply.
- The approval flag gets no single-character shorthand. None of the existing three
  primitives has one, and claiming a short letter would risk colliding with a flag an
  adopting CLI already uses.
- The approval flag gets no environment-variable fallback. Only the output-mode flag
  has one today, because mode must be resolvable by an outer process; approval is a
  per-invocation decision that should be explicit in the command line an agent
  constructs.
- The feature provides the invariant and the gate, not universal interception.
  ax-go does not detect or refactor third-party libraries that spawn pagers or
  editors; a CLI author who calls such a library directly at an unguarded point is
  outside what this feature can enforce.
- The mode-resolution precedence (flag, then environment, then TTY detection) is
  unchanged. Approval is read independently of mode and composes with whatever mode
  resolution produces.
- Reading a piped payload from stdin remains ordinary machine-mode behavior and is
  explicitly not the target of the invariant.
- Runtime MCP elicitation is out of scope and tracked separately; the exit-2
  `confirmation_required` envelope specified here is that future work's documented
  degradation target.
- Dry-run-by-default with an explicit apply flag is out of scope and tracked
  separately; this feature does not change any command's default side-effect posture.
- Surfacing a confirmation signal on the *success* envelope is out of scope and
  belongs to the envelope-trust-signals work; this feature touches only the error
  envelope's `error_code` value space.
