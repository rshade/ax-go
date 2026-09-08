# Feature Specification: Agent-Safety Context Reaches Every Command in the Tree

**Feature Branch**: `026-persistent-hook-context`

**Created**: 2026-09-08

**Status**: Draft

**Input**: User description: "Fix ax.Execute so agent-safety context (resolved output mode, dry-run state, --yes approval, and the idempotency key) reaches every command in the tree, not just commands where no ancestor declares its own PersistentPreRun or PersistentPreRunE. Tracked as GitHub issue #218, severity P0."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A subcommand group's own setup hook no longer breaks agent safety (Priority: P1)

A developer builds a CLI on ax-go whose command tree includes a subcommand
group with its own setup step — for example, loading a shared configuration
file or opening a client connection once for every command under that group.
Declaring that setup as a persistent hook on the group is ordinary practice.
Today, doing so completely and silently disables ax's dry-run, approval, and
idempotency-key handling for every command under that group.

**Why this priority**: This is the entire defect. Without a fix, the
library's core safety promise — that `--dry-run` never performs a real side
effect and `--yes` reliably approves an operation — is false for any command
tree that uses an everyday Cobra pattern the library itself never warned
against. A confirmed real deployment is safe today only by accident of
which command happens to hold the one persistent hook it declares.

**Independent Test**: Can be fully tested by building a two-level command
tree where a subcommand group declares its own persistent hook, running a
leaf command under it with `--dry-run`, and confirming no side effect occurs
— exactly as it would with no such hook anywhere in the tree.

**Acceptance Scenarios**:

1. **Given** a subcommand group declares its own persistent hook (of either
   form Cobra supports), **When** a command under that group runs with
   `--dry-run`, **Then** the dry-run state is visible on that command's
   context and any guarded side effect is suppressed, identically to a tree
   with no group-level hook.
2. **Given** the same subcommand group, **When** a command under it runs
   with `--yes`, **Then** the explicit approval is visible on that command's
   context and a confirmation-gated operation proceeds, identically to a
   tree with no group-level hook.
3. **Given** the same subcommand group, **When** a command under it runs
   without an explicit idempotency key, **Then** an idempotency key is still
   generated and visible on that command's context, identically to a tree
   with no group-level hook.

---

### User Story 2 - An adopter's own hook keeps working exactly as they wrote it (Priority: P2)

The same developer's subcommand group's own persistent hook — the one doing
their config loading or client setup — must keep running normally after this
fix ships: exactly once per invocation, with its existing error-handling
behavior intact, requiring no changes to their code.

**Why this priority**: A fix that solves User Story 1 by breaking or
duplicating the adopter's own hook execution would trade one defect for
another, and would force every affected adopter to rewrite their setup code.
This must be a drop-in fix from the adopter's point of view.

**Independent Test**: Can be fully tested by declaring a persistent hook
that increments a counter and, separately, one that returns an error, then
confirming each still runs exactly once per invocation and error handling is
unchanged.

**Acceptance Scenarios**:

1. **Given** a subcommand group's own persistent hook that returns an error,
   **When** a command under it runs, **Then** that command fails with the
   adopter's own error, exactly as it did before this fix.
2. **Given** a subcommand group's own persistent hook with no error return,
   **When** a command under it runs successfully, **Then** the hook has
   executed exactly once.

---

### User Story 3 - The most careful invocation is never the most dangerous one (Priority: P3)

An operator or an automated agent, being maximally careful, runs a command
with both `--dry-run` and `--yes` set together — rehearsing the operation
while pre-approving it. Under the current defect, this specific combination
is the one most likely to be trusted and is also the one most likely to
silently perform a real mutation when a subcommand group has its own hook.

**Why this priority**: This is a real, previously-identified failure mode
rather than a new one to design for, but it is lower priority than Stories 1
and 2 because fixing the underlying context propagation (Story 1) already
resolves it; this story exists to make sure the fix is verified against this
specific combined-flag scenario, not just each flag in isolation.

**Independent Test**: Can be fully tested by running `--dry-run --yes`
together against a command under a subcommand group with its own hook and
confirming no side effect occurs and the command's own idempotency key is
still generated.

**Acceptance Scenarios**:

1. **Given** a subcommand group's own persistent hook, **When** a command
   under it runs with `--dry-run --yes` together, **Then** no side effect
   occurs and the outcome matches the same invocation against a tree with no
   group-level hook.

---

### Edge Cases

- What happens when only the deepest command in a multi-level tree (a
  grandchild, with neither the root nor the intermediate group) declares its
  own persistent hook? Agent-safety context must still reach that command
  and everything beneath it.
- What happens when both the root and a subcommand group each declare their
  own persistent hook? Both hooks must still run, in their existing relative
  order, exactly once each, and agent-safety context must still be correct
  for commands under the group.
- What happens when a subcommand group declares the error-returning hook
  form, the non-error form, or both simultaneously? All three shapes must
  produce identical agent-safety context outcomes.
- What happens on a second or later invocation of the same command tree
  (for example, in a long-running host process that calls `Execute` more
  than once)? A hook that was already made safe on a prior invocation must
  not be wrapped or invoked twice.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST make the resolved output mode, dry-run state,
  approval state, and idempotency key available on every command's context
  identically, regardless of whether that command or any of its ancestors
  declares its own persistent pre-run hook.
- **FR-002**: The system MUST perform its own agent-safety context setup
  before invoking any adopter-declared persistent hook, at every level of
  the command tree, not only at the root.
- **FR-003**: The system MUST invoke each adopter-declared persistent hook
  exactly once per command execution, preserving its existing error-handling
  behavior (an error returned by the adopter's hook still stops execution
  with that same error).
- **FR-004**: The system MUST NOT change the documented behavior of the
  existing dry-run guard helper or the existing confirmation helper — both
  already behave correctly for a context that carries the right state; the
  defect being fixed is strictly in how that state reaches the context.
- **FR-005**: The system MUST NOT alter Cobra's own multi-hook traversal
  behavior process-wide; enabling that behavior is a decision that belongs
  to the adopter, not to this library.
- **FR-006**: The system MUST apply this fix idempotently: repeating the
  setup step against a command tree that has already been prepared MUST NOT
  wrap or invoke any hook more than the intended one time.
- **FR-007**: The success envelope's metadata (dry-run flag and idempotency
  key) MUST remain correct for every hook shape described in the acceptance
  scenarios above.
- **FR-008**: The change MUST be covered by tests proving that each hook
  shape in the acceptance scenarios produces context values identical to the
  no-hook baseline, and those tests MUST be shown failing against the
  current, unfixed behavior before the fix lands.

### Key Entities

- **Command tree**: The hierarchy of commands and subcommand groups an
  adopter builds on top of this library. This feature changes how
  agent-safety context propagates through that hierarchy; it does not
  change the hierarchy's shape or the commands' own behavior.
- **Agent-safety context**: The resolved output mode, dry-run state,
  approval state, and idempotency key already defined by this library's
  existing agent-safety primitives. This feature does not add a new field to
  that set; it fixes how reliably the existing fields propagate.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A command reached through a subcommand group that declares its
  own persistent hook produces dry-run, approval, and idempotency-key
  behavior identical to the same command in a tree with no such hook, across
  100% of the hook-shape combinations exercised by tests.
- **SC-002**: A dry-run-guarded side effect is never performed under
  `--dry-run`, regardless of where in the command tree a persistent hook is
  declared.
- **SC-003**: An operation explicitly approved with `--yes` is never blocked
  as unapproved, regardless of where in the command tree a persistent hook
  is declared.
- **SC-004**: Adopters whose only persistent hook is already on their root
  command — the previously-safe configuration — observe no behavior change
  after this fix ships.
- **SC-005**: No exported Go signature changes as a result of this fix.

## Assumptions

- Source inputs: [GitHub issue #218](https://github.com/rshade/ax-go/issues/218). No governing ADR applies.
- "Every command in the tree" includes the root command, any command with no
  hook of its own, and any command carrying its own persistent hook at any
  depth.
- This is a runtime-behavior fix to a public entry point, not a new public
  option or flag; no new adopter-facing configuration is introduced.
- An adopter relying on the current shadowing defect (a subcommand hook
  silently disabling agent-safety context) is not a scenario this library
  supports or must remain compatible with — no spec, ADR, or documentation
  ever described that behavior as intentional.
- Idempotent application of the fix (Edge Cases, FR-006) requires some form
  of per-command marker; the exact mechanism is a planning-stage decision,
  not a specification-stage one.
