# Feature Specification: Execute Shutdown Flush Hook

**Feature Branch**: `024-execute-flush-hook`

**Created**: 2026-09-01

**Status**: Draft

**Input**: GitHub issue #119, "feat: WithFlushFunc ExecuteOption to drain
ax.Flush on Execute shutdown"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Register one lifecycle-owned flush (Priority: P1)

As a CLI author using the root ax runtime, I want to register my buffered-log
flush with `Execute` once so every ordinary command return drains pending output
without a hand-written defer in each command handler.

**Why this priority**: The feature's primary value is moving easy-to-forget
shutdown wiring to the lifecycle boundary that already owns process cleanup.

**Independent Test**: Register a callback, run one successful command and one
failing command, and verify the callback is invoked exactly once after each
command path while the original result and stream contract remain intact.

**Acceptance Scenarios**:

1. **Given** a successful command and a registered flush callback, **When**
   `Execute` returns, **Then** the callback runs exactly once with a bounded
   shutdown context and the success exit code remains `0`.
2. **Given** a command that returns a classified error and a registered flush
   callback, **When** `Execute` returns, **Then** the callback runs exactly once
   and the command's classified exit code remains unchanged.
3. **Given** a CLI that does not register a callback, **When** it runs through
   `Execute`, **Then** its shutdown behavior is unchanged from the current
   release.

---

### User Story 2 - Flush failures remain fail-open and safe (Priority: P2)

As a CLI operator, I want a failed shutdown flush to be visible without
corrupting the machine payload or replacing the command's real outcome, so an
observability outage cannot turn a successful operation into a reported
failure or hide the command's actual error category.

**Why this priority**: Lifecycle convenience is useful only if it preserves
ax-go's deterministic streams and exit-code contract under failure.

**Independent Test**: Make the registered callback return an error containing
line breaks and control characters, then verify stdout is unchanged, stderr
contains a single-line sanitized flush diagnostic, and both successful and
failing commands retain their original exit codes.

**Acceptance Scenarios**:

1. **Given** a callback that returns an error after a successful command,
   **When** shutdown completes, **Then** stderr reports a sanitized flush
   diagnostic and `Execute` still returns `0`.
2. **Given** a callback that returns an error after a command failure, **When**
   shutdown completes, **Then** the original error envelope and exit category
   remain authoritative and the flush diagnostic does not alter stdout.
3. **Given** a callback whose work reaches its shutdown deadline, **When** it
   returns the context error, **Then** the diagnostic is fail-open and the
   existing telemetry shutdown still receives its full configured budget.

---

### User Story 3 - Copy the recommended integration pattern (Priority: P3)

As an adopting CLI author, I want the integration example and quickstart to
show lifecycle-owned flushing so I can copy a complete, correct pattern rather
than reconstructing timeout and error-handling rules myself.

**Why this priority**: The public option removes boilerplate only when users can
discover and adopt it from the repository's canonical examples.

**Independent Test**: Follow the documented snippet in a small CLI and confirm
that a logger created during command execution can be flushed from the
`Execute` shutdown path without a command-local flush defer.

**Acceptance Scenarios**:

1. **Given** the runnable integration example, **When** a maintainer inspects
   its root execution path, **Then** it registers lifecycle-owned flushing and
   contains no command-local flush timeout defer.
2. **Given** the first-CLI tutorial, **When** a user adds buffered logging,
   **Then** the recommended snippet registers flushing with `Execute` and
   explains that flush errors are diagnostic-only.
3. **Given** the primary `Execute` API example, **When** documentation coverage
   runs, **Then** the option is demonstrated within that parent example rather
   than creating a standalone option example.

### Edge Cases

- A nil callback is treated as absent and is never invoked.
- If the option is supplied more than once, the last supplied callback is the
  registered callback; a final nil callback clears an earlier registration.
- The callback runs after command execution even when failure occurs during
  argument parsing, persistent pre-run setup, or the command handler.
- The guaranteed invocation paths are ordinary Cobra returns, whether successful
  or unsuccessful. `Execute` does not recover panics, and process termination
  that bypasses Go defers is outside the callback guarantee.
- The callback receives a fresh shutdown context rather than a possibly
  canceled command context.
- A flush callback may return immediately when no buffered sink was created;
  this remains a successful no-op.
- Execute's handling of a callback failure must not write to stdout, change a
  success payload, change an error envelope, or replace any exit code. Side
  effects performed directly by caller-supplied callback code remain the
  callback author's responsibility.
- Flush diagnostics escape or remove ASCII control characters so one error
  cannot forge additional diagnostic lines.
- Flush and telemetry shutdown each receive a full bounded window; a slow flush
  cannot consume the existing telemetry shutdown window.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The root runtime MUST expose an additive `ExecuteOption` named
  `WithFlushFunc` that accepts one callback receiving a cancellation-aware
  shutdown context and returning an error.
- **FR-002**: Registering the option MUST NOT require changing `Execute`'s
  existing signature or any existing call site.
- **FR-003**: When registered, the callback MUST run exactly once on every
  ordinary return from Cobra command execution, whether successful or
  unsuccessful.
- **FR-004**: The callback MUST receive a fresh bounded shutdown context that is
  independent of the command context.
- **FR-005**: The callback's shutdown window MUST be bounded by the same duration
  setting used as the telemetry shutdown timeout while remaining a separate
  window from telemetry shutdown.
- **FR-006**: A nil callback MUST behave as though the option were not supplied.
- **FR-007**: If the option is supplied repeatedly, the last supplied callback
  MUST determine shutdown behavior, including a final nil callback clearing an
  earlier registration.
- **FR-008**: A callback error MUST NOT change the exit code already determined
  by command execution.
- **FR-009**: Execute MUST handle a callback error by producing one `stderr`
  diagnostic prefixed `ax: flush failed:` and MUST NOT write to `stdout` because
  of that error.
- **FR-010**: The error text in the flush diagnostic MUST be sanitized using the
  same control-character policy as telemetry shutdown diagnostics. The public
  callback contract MUST state that this policy is not redaction and callback
  errors must not contain PII, secrets, tokens, or credentials.
- **FR-011**: Callback execution MUST leave existing telemetry shutdown behavior
  intact, including its configured timeout window and fail-open diagnostic.
- **FR-012**: Existing callers that omit `WithFlushFunc` MUST retain their
  current output, exit-code, and telemetry lifecycle behavior.
- **FR-013**: The integration example MUST replace its command-local flush defer
  with `WithFlushFunc` on the `Execute` call.
- **FR-014**: The repository's recommended first-CLI documentation and primary
  `Execute` example MUST demonstrate the lifecycle-owned flush pattern and state
  that flush failure is diagnostic-only.
- **FR-015**: The new exported option MUST be recorded in the reviewed public
  surface baseline and permanent supported-surface audit.
- **FR-016**: Tests MUST first demonstrate the missing behavior, then cover
  success, classified command failure, sanitized callback failure, nil callback,
  repeated option precedence, bounded context, stream separation, and unchanged
  exit codes.
- **FR-017**: All default and declined-dependency build configurations MUST
  expose identical callback behavior and the same public option.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An adopting CLI can replace its command-local timeout/defer flush
  sequence with one option on the existing `Execute` call.
- **SC-002**: Automated tests observe exactly one callback invocation on each of
  the ordinary successful and unsuccessful Cobra return paths, with zero
  invocations when no callback is registered.
- **SC-003**: For callbacks that do not themselves write to command streams,
  across callback-success, callback-failure, and callback-timeout cases, 100% of
  command exit codes and stdout bytes match an otherwise equivalent execution
  with the callback omitted.
- **SC-004**: Every injected ASCII control character is prevented from creating
  an additional stderr line in the flush-failure diagnostic.
- **SC-005**: The integration example, first-CLI tutorial, and primary API
  example all use or demonstrate the same lifecycle-owned shutdown pattern.
- **SC-006**: The full required quality-gate matrix completes with no test,
  race, vet, lint, documentation-coverage, public-surface, coverage, binary-size,
  benchmark-budget, or build-configuration regressions.

## Assumptions

- Source input is GitHub issue #119. No frozen ADR governs this feature:
  ADR-0008 fixes Cobra as the CLI framework but this additive shutdown callback
  neither reconsiders that choice nor changes Cobra integration semantics.
- This feature registers one lifecycle callback, not a general multi-hook
  shutdown framework; a callback may aggregate multiple flushes itself when
  needed.
- The existing telemetry shutdown duration is the established bounded-lifecycle
  setting. Reusing its duration avoids another public timeout knob, while
  separate contexts protect the existing telemetry budget.
- The callback contract is generic enough to call `ax.Flush` without coupling
  `Execute` to a specific logger value or sink implementation.
- Flush failure remains an operational diagnostic, matching the existing
  fail-open `ax.Flush` and telemetry shutdown contracts.
- Signal handling and process termination remain the adopting application's
  responsibility; `Execute` continues to return an exit code rather than
  calling `os.Exit`.
