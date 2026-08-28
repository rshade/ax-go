# Feature Specification: Default-On Audited Guard/Perform

**Feature Branch**: `020-guard-audit-logging`

**Created**: 2026-08-26

**Status**: Draft

**Input**: User description: "GitHub issue #179: Guard/Perform variant with built-in structured 'about to / did / failed' logging"

## Clarifications

### Session 2026-08-26

- Q: Should audited logging be opt-in only (separate `GuardWithAudit`/`PerformWithAudit` functions, `Guard`/`Perform` unchanged), or should it become `Guard`/`Perform`'s default behavior? → A: Default-on hybrid. `Guard`/`Perform` keep their existing signatures but, on every real (non-dry-run) run, emit generic (description-less) audit lines by default — no code change required by existing callers. A context-based opt-out signal, propagating through the context subtree (mirroring how dry-run state is already threaded via context), lets a caller suppress this for effects where it's inappropriate (very-high-frequency internal effects, non-consequential effects). Named entry points (the audited variants) remain available for callers who want a richer, human-meaningful description in the audit trail instead of the generic default. Because this changes `Guard`/`Perform`'s real-run stderr output for every existing caller — even though no Go call signature changes — it is a deliberate breaking **behavior** change, not a purely additive one, and MUST ship via this project's breaking-change process (`feat!:`/`BREAKING CHANGE:` commit, `breaking-change-approved` label, migration note), per Constitution Principle XI. This entire feature (default-on behavior, opt-out, and the named rich-description variants) ships together in one release, not staged across releases.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Every existing Guard/Perform call gets baseline audit visibility with no code change (Priority: P1)

A developer has already wrapped a consequential mutation (destroy,
create, deploy) in plain `Guard` or `Perform`, exactly as ax-go
recommends today. After upgrading to this feature, the very next real
run of that command automatically logs "about to run" and then
"succeeded"/"failed" around the effect — without the developer writing
any logging code or switching to a different function.

**Why this priority**: This closes the actual failure mode that
motivated the feature: an opt-in mechanism that a busy developer never
adopts provides no safety net at all. Production evidence (a downstream
CLI wrapping every mutation in plain `Guard` with zero audit logging,
despite the logger being available the whole time) shows the opt-in-only
shape doesn't get used. Default-on is the only version of this feature
that guarantees the visibility exists.

**Independent Test**: Take an existing call site using plain `Guard`
(no code changes), run it for real (not dry-run), and confirm stderr
shows a structured "about to run" line before the effect executes and a
structured outcome line after — with zero changes to the call site
itself.

**Acceptance Scenarios**:

1. **Given** dry-run is not active and a non-nil effect is passed to
   `Guard`, **When** the command runs and the effect succeeds, **Then**
   stderr contains a structured "about to run" line emitted before the
   effect executes, followed by a structured "succeeded" line, with no
   code changes required at the call site.
2. **Given** dry-run is not active and a non-nil effect is passed to
   `Guard`, **When** the effect returns an error, **Then** stderr
   contains the "about to run" line followed by a structured "failed"
   line carrying the error as a structured field, and the original
   error (with its wrap chain intact) is returned to the caller
   unchanged.
3. **Given** a call site that needs to suppress this default behavior
   (e.g. a very-high-frequency internal effect), **When** the caller
   applies the documented context-based opt-out, **Then** no audit lines
   are emitted for that call or any nested Guard/Perform calls it makes,
   identical to today's fully-silent behavior.

---

### User Story 2 - Developer upgrades to a rich, described audit trail (Priority: P2)

The same developer wants the audit lines to carry a human-meaningful
description ("destroy stack prod-east") rather than the generic
default. They switch that call site to the named audited entry point
and supply a description, which appears in every audit line for that
call as a structured field.

**Why this priority**: This is a strict enhancement over the P1
baseline — valuable, but the feature already delivers its core value
(visibility exists by default) without it.

**Independent Test**: Wrap an effect with the named audited entry
point, supplying a description, run outside dry-run, and confirm the
"about to run" and outcome lines carry that description as a structured
field rather than the generic default label.

**Acceptance Scenarios**:

1. **Given** dry-run is not active and a description is supplied to the
   named audited entry point, **When** the effect runs, **Then** the
   "about to run" and outcome log lines carry that description as a
   structured field instead of the generic default label.

---

### User Story 3 - Dry-run behavior is unchanged (Priority: P3)

The same developer runs their CLI with `--dry-run` set. They rely on
the existing single suppression line (already shipped) to confirm the
effect was skipped, and do not want that behavior altered or duplicated
by this feature — regardless of whether the call site uses plain
`Guard`/`Perform` or the named audited variants.

**Why this priority**: Regressing dry-run behavior would break every
existing caller of `Guard`/`Perform` and erode trust in output
determinism — a non-negotiable invariant this feature must preserve even
though it isn't new user-facing value on its own.

**Independent Test**: Run any call site (plain or named-audited) with
dry-run active and confirm stderr contains only the existing single
suppression line — no "about to run" line, and no duplicate suppression
line.

**Acceptance Scenarios**:

1. **Given** dry-run is active and a non-nil effect is passed to
   `Guard` (plain or the named audited variant), **When** the command
   runs, **Then** stderr contains exactly the existing suppression line
   and nothing else from the default/audited behavior, and the effect
   does not execute.
2. **Given** dry-run is active and a non-nil `rehearse` is passed to
   `Perform`, **When** `rehearse` fails, **Then** stderr contains no
   suppression line and no "about to run"/"failed" line beyond what
   `rehearse`'s own caller already surfaces, matching today's `Perform`
   behavior.

---

### User Story 4 - Every description is safe to log (Priority: P4)

The developer supplies a human-readable description of the effect to
the named audited entry point. That description may originate from
user-controlled or resource-derived data (a stack name, a resource ID)
and must never let a caller forge or corrupt a log line.

**Why this priority**: A safety property of the P1/P2 behavior rather
than new user-visible behavior, but a hard constraint carried over from
the existing suppression-line discipline that must hold from day one.

**Independent Test**: Call the named audited entry point with a
description string containing newlines or control characters and
confirm the resulting log lines remain well-formed structured records
with the description confined to its own field.

**Acceptance Scenarios**:

1. **Given** a description string containing a newline or other control
   character, **When** the audit lines are logged, **Then** the
   description appears only as a structured field value (never
   concatenated into the message text), and the log output is not
   corrupted or split into forged extra lines.

### Edge Cases

- What happens when the wrapped effect is `nil`? No "about to run" or
  outcome line is emitted for an effect that will not run (mirrors
  today's `Guard`/`Perform`, which already special-case `nil`
  effects/commits to skip the suppression line).
- What happens when the description string is empty (named audited
  variants)? The log lines still emit with an empty field value rather
  than being suppressed or erroring.
- What happens when the context carries no active dry-run state (e.g. a
  `nil` context)? The default and named-audited behavior both follow
  the same dry-run-inactive-by-default behavior as `Guard`/`Perform`
  today.
- How does `Perform` behave when `rehearse` is `nil` and dry-run is
  active? It behaves like a pure skip — same as today's `Perform` —
  with no audit lines beyond the existing suppression line.
- What happens at a call site that opts out of the default audit
  behavior? No audit lines are emitted for that call or any nested
  Guard/Perform calls reached through the same context; behavior is
  identical to `Guard`/`Perform` before this feature shipped.
- What happens for an existing consumer whose tests assert exact or
  empty stderr output? Those assertions will observe new output after
  upgrading and must be updated or the call site opted out — this is
  the expected, documented consequence of the breaking-change migration
  (see FR-010).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: On a real (non-dry-run) run, `Guard` and `Perform` MUST,
  by default, emit a structured "about to run" log line to stderr
  before invoking a non-nil effect/commit — with no code change
  required by existing callers.
- **FR-002**: After the wrapped effect/commit completes on a real run,
  `Guard`/`Perform` MUST emit exactly one structured outcome log line to
  stderr by default: "succeeded" when the effect returns a nil error, or
  "failed" (carrying the error as a structured field) when it returns a
  non-nil error.
- **FR-003**: The system MUST provide a context-based mechanism to
  suppress the default audit lines for an entire subtree of Guard/Perform
  invocations (following standard context-value propagation), for effects
  where audit logging is inappropriate (e.g. very-high-frequency internal
  effects, or effects that are not consequential mutations).
- **FR-004**: The system MUST provide named entry points (the audited
  variants) that accept a caller-supplied description of the effect and
  include it as a structured field in the audit log lines, for callers
  who want a more meaningful audit trail than the generic default.
- **FR-005**: The default and named-audited log lines MUST preserve the
  wrapped effect/commit's return value verbatim, including the error and
  its wrap chain (`errors.Is`/`errors.As` continue to work), identically
  to today's `Guard`/`Perform`.
- **FR-006**: When dry-run is active, neither the default behavior nor
  the named audited variants MUST change dry-run behavior: only the
  existing single suppression line is emitted; no "about to run" or
  outcome line, and no duplicate suppression line.
- **FR-007**: No value passed through the default or named-audited path
  (description, error text, or any other caller-supplied or
  effect-derived string) MUST ever be formatted into the log message
  string itself — every value MUST go through a structured field method,
  matching the no-log-forging discipline the existing suppression line
  already follows.
- **FR-008**: All audit log output MUST be written to stderr only;
  stdout payload content and determinism MUST be unaffected by this
  feature.
- **FR-009**: A `nil` effect (`Guard`) or `nil` commit (`Perform`) MUST
  NOT produce an "about to run" or outcome log line, matching today's
  `nil`-effect handling.
- **FR-010**: This change to `Guard`/`Perform`'s default real-run
  behavior is a deliberate, documented breaking behavior change
  (existing callers' stderr output changes even though Go call
  signatures do not) and MUST be released following this project's
  breaking-change process — an explicit `feat!:`/`BREAKING CHANGE:`
  commit and the `breaking-change-approved` label — with a migration
  note covering how to suppress the new default output.
- **FR-011**: The system MUST document both the default behavior and
  the named audited variants as the recommended pattern for
  consequential/destructive mutations, and MUST document the
  context-based opt-out mechanism for callers who need to opt out.

### Key Entities

- **Audit log line**: A structured stderr record emitted around a real
  (non-dry-run) `Guard`/`Perform` invocation, carrying at minimum: which
  phase it represents (about to run / succeeded / failed), a
  description (the generic default label, or a caller-supplied
  description when using the named audited entry points), and (for the
  failed phase) the error. Trace correlation fields are attached
  automatically by the canonical logger, consistent with all other
  ax-go log output.
- **Audit opt-out signal**: A context-based signal that suppresses the
  default audit lines for the entire subtree of Guard/Perform invocations
  reached through that context (following standard context-value propagation),
  analogous to how dry-run state is already threaded — without changing
  `Guard`/`Perform`'s Go signatures.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every existing `Guard`/`Perform` call site produces
  before/after audit visibility on its very next real run after
  upgrading, with zero code changes at the call site.
- **SC-002**: Every existing `Guard`/`Perform` call site continues to
  compile and preserves its functional return-value contract unchanged
  after upgrading (errors and wrap chains identical); only stderr output
  changes, and that change is fully covered by the migration note
  required by FR-010.
- **SC-003**: A developer who wants a richer audit trail gets a
  human-meaningful description in every audit line with zero additional
  logging code of their own, by switching to the named audited entry
  points.
- **SC-004**: 100% of audit log lines produced by the default or
  named-audited paths carry caller-supplied or error-derived text
  exclusively as structured fields, never interpolated into the message
  string (verified by tests with adversarial description/error
  content).
- **SC-005**: Dry-run runs produce byte-for-byte the same stderr
  suppression-line output as today's `Guard`/`Perform` dry-run runs (no
  new or duplicated lines), regardless of whether the call site uses the
  default behavior or the named audited variants.

## Assumptions

- Source inputs: GitHub issue #179, refined through `/speckit-clarify`
  on 2026-08-26 (see Clarifications). No governing ADR — this behavior
  postdates the ADR log freeze and is specified directly through the
  Spec Kit workflow.
- This feature is **not** purely additive: making audit logging
  default-on is an intentional breaking behavior change to
  `Guard`/`Perform`'s real-run stderr output, accepted under
  Constitution Principle XI's pre-v1.0 `0.MINOR.0`-may-break allowance,
  and gated by the existing `breaking-change-approved` process. The Go
  call signatures of `Guard`/`Perform` themselves are assumed to stay
  unchanged; the opt-out is a context-based signal rather than a new
  parameter, so this is a behavior break, not a compile-time break.
- The default behavior, the context-based opt-out, and the named
  rich-description entry points all ship together in a single release —
  this is not staged across multiple releases.
- "About to run" is emitted only on the real-run path; the dry-run path
  keeps relying entirely on the existing suppression line rather than
  gaining a second, parallel "about to skip" line.
- This feature is observability-only: it never changes whether an
  effect executes, and is explicitly not a confirmation/approval gate
  (that is separate, tracked work referenced by the source issue).
- Naming, exact field names, log level choices, and the precise
  mechanism used for the context-based opt-out signal are implementation
  details resolved during planning, not user-facing scope decisions.
