# Feature Specification: Agent-safety helpers for --dry-run side-effect suppression

**Feature Branch**: `012-dry-run-guards`

**Created**: 2026-06-28

**Status**: Draft

**Input**: User description: "Agent-safety helpers for --dry-run side-effect
suppression (GitHub issue #13). `--dry-run` already resolves into context and the
envelope already surfaces `dry_run: true`; what is missing are ergonomic, safe
helpers so commands stop hand-rolling `if DryRunFromContext(ctx) { ... } else
{ ... }` to guard side-effecting operations. Add two helpers: a skip-only guard
and a rehearse/commit pair, both exported from the import-isolated contract
package with thin root-package facades."

## Clarifications

### Session 2026-06-28

- Q: When Guard/Perform suppress a side effect under dry-run, should the helper emit
  an observability signal or stay silent? → A: Emit a single diagnostic log line on
  stderr via the canonical logger noting the suppression; stdout payload and envelope
  determinism are unaffected (logs are stderr, never stdout).
- Q: Logging requires the canonical logger, which the import-isolated `contract`
  package is forbidden to import — where should the helpers live? → A: In the root
  `ax` package only. They are NOT added to `contract`; the root package owns the
  implementation directly rather than re-exporting a `contract` facade, preserving
  `contract` import-isolation. (Supersedes the "live in contract" framing in the
  original Input above.)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Guard a side effect against dry-run in one call (Priority: P1)

A developer building a CLI on ax-go has a command that performs a side effect
(writing a file, calling a remote API, mutating state). Today they must hand-write
a conditional that branches on the dry-run context state, and every command repeats
that boilerplate — a place where a slip silently lets a side effect escape under
`--dry-run`. They want a single guard call that runs the side effect normally but
suppresses it entirely when dry-run is active, and tells them whether it ran so they
can shape the rest of their output.

**Why this priority**: This is the smallest slice that delivers the issue's core
value — make side-effect suppression the easy, safe default instead of repeated
hand-rolled conditionals. It stands alone: a developer can guard one operation and
immediately get correct dry-run behavior with no other part of the feature present.

**Independent Test**: Call the guard with a side effect that records whether it ran,
once with dry-run inactive in the context and once with dry-run active. Assert the
effect ran (and the call reported it ran) in the first case, and did not run (and the
call reported it was skipped, with no error) in the second.

**Acceptance Scenarios**:

1. **Given** a context with dry-run inactive, **When** a developer guards a side
   effect, **Then** the effect executes and the call reports that it executed.
2. **Given** a context with dry-run active, **When** a developer guards a side
   effect, **Then** the effect does not execute, the call reports it was skipped, the
   call returns no error, and a single suppression log line is written to stderr.
3. **Given** a context with dry-run inactive and an effect that fails, **When** a
   developer guards it, **Then** the effect's error is returned unchanged (its
   wrap chain preserved) and the call reports that it executed.

---

### User Story 2 - Faithful dry-run preview via rehearse/commit (Priority: P2)

A developer has a command whose dry-run should be a faithful preview, not a silent
skip: under `--dry-run` it must still surface the same validation errors a real run
would (missing file, malformed input, rejected change) without performing the
mutation. They want one helper that runs the real commit when dry-run is inactive,
or a read-only rehearsal when dry-run is active, so a dry-run that would have failed
fails identically and a dry-run that would have succeeded returns the same envelope
minus the side effect.

**Why this priority**: It builds on US1's suppression but adds the rehearsal contract
that makes dry-run a trustworthy preview for agents. It is independently valuable and
testable, but ranks below the skip-only guard because faithful preview is a richer
guarantee layered on top of basic suppression.

**Independent Test**: Provide a commit function that mutates and a rehearse function
that performs the same read-only validation without mutating. With dry-run inactive,
assert the commit ran and mutated. With dry-run active, assert the rehearse ran, the
commit did not, no mutation occurred, and an invalid input produces the same error in
both modes.

**Acceptance Scenarios**:

1. **Given** a context with dry-run inactive, **When** a developer uses the
   rehearse/commit helper, **Then** the commit runs, the rehearse does not, and the
   commit's result/error is returned.
2. **Given** a context with dry-run active and valid input, **When** a developer uses
   the rehearse/commit helper, **Then** the rehearse runs, the commit does not, no
   mutation occurs, and the call returns no error.
3. **Given** a context with dry-run active and input a real run would reject, **When**
   a developer uses the rehearse/commit helper, **Then** the rehearse surfaces the
   same validation error a real run would, with no mutation.
4. **Given** a developer who supplies no rehearsal action, **When** dry-run is active,
   **Then** the helper performs a pure skip (no commit, no error), behaving like the
   skip-only guard.

---

### User Story 3 - Consistent envelope and a discoverable canonical example (Priority: P3)

A developer (and the agent running their CLI) needs the machine envelope to keep
surfacing `dry_run: true` whenever dry-run is active, regardless of which helper was
used, and needs a verified, runnable example to learn the pattern from. The
repository's own integration command should demonstrate the rehearse/commit helper as
the canonical usage rather than a hand-rolled conditional.

**Why this priority**: It protects the existing automatic `dry_run: true` guarantee
from regression and makes the new primitives learnable through verified examples — the
highest-leverage documentation artifact for agents — but it is a polish/consistency
slice that depends on US1 and US2 existing first.

**Independent Test**: Run a guarded command end to end under dry-run and assert the
emitted success envelope carries `dry_run: true` and is byte-identical to a real run
apart from documented non-deterministic fields; confirm a runnable example for each
helper compiles and runs in the test suite.

**Acceptance Scenarios**:

1. **Given** any command that uses either helper under dry-run, **When** it emits its
   success envelope, **Then** the envelope carries `dry_run: true`.
2. **Given** the same command run under dry-run and for real, **When** both envelopes
   are compared, **Then** they are byte-identical except for fields documented as
   non-deterministic (timestamps, trace/span IDs, auto-generated idempotency key) and
   the `dry_run` flag.
3. **Given** the published API documentation, **When** a developer looks up either
   helper, **Then** a runnable example demonstrates its use and is exercised by the
   test suite.

---

### Edge Cases

- **No commit / no effect supplied**: A guard given no effect, or a rehearse/commit
  given no commit, performs nothing and returns no error rather than failing — the
  helpers never panic on a missing function.
- **No rehearsal supplied under dry-run**: Treated as an intentional pure skip (see
  US2 acceptance #4), not an error.
- **Absent or nil context**: When dry-run state cannot be read from the context, the
  helpers treat dry-run as inactive and run the real path (no suppression, no skip
  log); they MUST NOT panic. The suppression log line (FR-013) is built only on the
  dry-run-active path, which by construction carries a usable context.
- **Effect/commit/rehearse panics**: The helpers do not install recovery; a panic in a
  caller-supplied function propagates as it would for a direct call (helpers add no
  hidden control flow).
- **Concurrency**: The helpers invoke the supplied function synchronously and add no
  goroutines; any concurrency is the caller's responsibility.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a skip-only guard primitive that executes a
  caller-supplied effect when dry-run is inactive in the context and skips it entirely
  when dry-run is active.
- **FR-002**: The guard primitive MUST report to the caller whether the effect
  executed — true when run, false when skipped under dry-run.
- **FR-003**: When the guard runs the effect, it MUST return the effect's error
  unchanged, preserving the wrap chain so `errors.Is`/`errors.As` keep working; when
  it skips, it MUST return a nil error.
- **FR-004**: The system MUST provide a rehearse/commit primitive that executes the
  commit operation when dry-run is inactive and the rehearse operation when dry-run is
  active.
- **FR-005**: The rehearse/commit primitive MUST return the error of whichever branch
  ran (commit in a real run, rehearse under dry-run) with its wrap chain preserved, so
  a dry-run surfaces the same validation failures a real run would.
- **FR-006**: The rehearse/commit primitive MUST accept "no rehearsal action" to mean
  a pure skip under dry-run (no commit, no error), making it behaviorally equivalent
  to the guard primitive for that branch.
- **FR-007**: Neither primitive may produce a state-mutating side effect of its own.
  The only state changes are those the caller's functions perform, and under dry-run
  the commit/effect path MUST NOT be invoked. The one output the helpers themselves
  produce is the stderr suppression log line of FR-013, which is an observability
  signal, not a state change.
- **FR-008**: Both primitives MUST live in the public root `ax` package and MUST NOT
  be added to the import-isolated `contract` package. Because they emit a log line on
  suppression (FR-013) they depend on the canonical logger, which `contract` is
  forbidden to import; the root package therefore owns the implementation directly
  rather than re-exporting a `contract` facade. This is a deliberate departure from the
  context, error, and JSON facades, made to preserve `contract` import-isolation.
- **FR-009**: The machine envelope MUST continue to surface `dry_run: true` whenever
  dry-run is active, independent of whether a guard or rehearse/commit primitive was
  used — no regression to the existing automatic stamping.
- **FR-010**: Both primitives MUST resolve dry-run state solely from the existing
  context state. They MUST NOT introduce new flags, environment variables, or envelope
  fields, nor change how dry-run is resolved into the context.
- **FR-011**: The helpers MUST treat a missing caller function defensively: a nil
  effect (guard) or nil commit (rehearse/commit) results in no action and a nil error
  rather than a panic.
- **FR-012**: Each primitive MUST be demonstrated by a verified, runnable example on
  the primary API surface so that documentation cannot silently drift, and the
  repository's integration command SHOULD adopt the rehearse/commit primitive as the
  canonical demonstration in place of its current hand-rolled dry-run conditional.
- **FR-013**: When a primitive suppresses a side effect under dry-run (the guard skips
  its effect, or the rehearse/commit skips its commit), it MUST emit a single
  structured log line via the canonical logger indicating the suppression. That line
  MUST go to stderr only, MUST NOT be written to stdout, and MUST NOT alter the machine
  envelope or its byte-for-byte determinism. When dry-run is inactive (the real path
  runs), the primitives MUST NOT emit this line. For the rehearse/commit primitive the
  line is emitted only when the dry-run preview succeeds (or no rehearsal was supplied):
  a failed rehearsal returns its error and emits no suppression line, because the command
  already surfaces that error and nothing would have proceeded.

### Key Entities

- **Guard primitive**: The skip-only helper, exported from the root `ax` package.
  Inputs: the context (carrying dry-run state) and one effect function. Output: whether
  the effect executed, plus any error the effect returned; on a dry-run skip it also
  writes one suppression log line to stderr. Represents "run this side effect unless we
  are rehearsing."
- **Rehearse/commit primitive**: The faithful-preview helper, exported from the root
  `ax` package. Inputs: the context, an optional read-only rehearse function, and a
  commit function. Output: the error of whichever branch ran; on a dry-run skip of the
  commit it also writes one suppression log line to stderr. Represents "do the real
  thing, or preview it without side
  effects."

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can guard a side effect against dry-run with a single helper
  call, replacing the multi-line hand-rolled `if dry-run … else …` conditional at the
  call site.
- **SC-002**: Under dry-run, 100% of guarded side effects are suppressed — tests prove
  the effect/commit function is never invoked when dry-run is active.
- **SC-003**: A dry-run rehearsal surfaces the same validation error a real run would,
  demonstrated by a test where one invalid input fails identically in dry-run and real
  mode while only the real mode performs the mutation.
- **SC-004**: A command's success envelope is byte-identical between a dry-run and a
  real run except for the `dry_run` flag and fields documented as non-deterministic
  (timestamps, trace/span IDs, auto-generated idempotency key).
- **SC-005**: Both primitives are reachable from the public root `ax` package (and are
  intentionally absent from the `contract` package), and the public-API surface check
  passes with no new public package added to the allowlist — the helpers are added to
  the already-allowlisted root package.
- **SC-006**: Every new primitive has a verified runnable example that compiles and
  executes in the test suite, and the documentation-coverage gate stays green.
- **SC-007**: When a side effect is suppressed under dry-run, exactly one diagnostic
  log line appears on stderr and zero bytes are added to stdout — verifiable by
  capturing both streams in a test — while a real run emits no such line.

## Assumptions

- Source inputs: GitHub issue #13. No governing ADR — the agent-safety primitives are
  governed by Constitution Principle IV (Agent-Safety Primitives), which mandates that
  `--dry-run` emit the same envelope with `dry_run: true` and cause no side effects.
- Dry-run state is already plumbed into `context.Context` (set when resolving the
  `--dry-run` flag) and the machine envelope already stamps `dry_run: true`
  automatically from that state. This feature is purely additive ergonomic helpers over
  the existing state; it changes neither flag resolution nor envelope shape.
- "Users" of this feature are Go developers building CLIs on ax-go, and transitively
  the LLM agents that run those CLIs and rely on dry-run being a safe rehearsal.
- Caller-supplied functions are synchronous; the helpers add no goroutines, timeouts,
  or recovery. Concurrency, cancellation, and panic handling remain the caller's
  responsibility and the context's existing semantics.
- The helpers do not touch the envelope; developers continue to build and emit the
  envelope as they do today, and `dry_run: true` continues to flow from context.
- The helpers depend on the canonical logger to emit the suppression line (FR-013) on
  stderr, which is why they live in the root `ax` package and not in the import-isolated
  `contract` package (see Clarifications). Stream separation is preserved: the line is
  stderr-only and never reaches stdout.
