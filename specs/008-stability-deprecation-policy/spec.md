# Feature Specification: Stability + Deprecation Policy

**Feature Branch**: `008-stability-deprecation-policy`

**Created**: 2026-06-16

**Status**: Draft

**Input**: GitHub Issue #17 — "Stability + deprecation policy: ADR-0013 (SemVer + pre-v1.0) and ADR-0014 (deprecation)"

## Governance Note

The original issue requests creating `docs/adr/0013-stability-semver-policy.md` and
`docs/adr/0014-deprecation-policy.md`. The constitution (v1.1.0, Governance section) explicitly
prohibits creating or editing ADRs: *"They MUST NOT be created or edited going forward."*
New cross-cutting architectural decisions are recorded in the consuming feature's `research.md`
and, when applicable, elevated to a constitution principle by amendment.

This spec supersedes those acceptance criteria. The deliverables are:

- A constitution amendment adding a **Stability & SemVer Principle** (cross-cutting → amend
  constitution).
- A constitution amendment adding a **Deprecation Lifecycle Principle** (cross-cutting → amend
  constitution).
- `AGENTS.md` updated to reference the new constitution sections (replaces the ADR reference
  requirement).
- README `Status` section updated to reflect the documented stability tier.
- `staticcheck SA1019` verification: **already enabled** — the existing `.golangci.yml`
  uses `checks: [all]` without excluding SA1019. No config change is needed; this must be
  confirmed and documented.

## Clarifications

### Session 2026-06-16

- Q: Pre-v1.0 SemVer contract for `0.x` releases? → A: Pragmatic pre-v1.0 — a
  `0.MINOR.0` bump MAY contain breaking changes; `0.x.PATCH` releases MUST stay
  backward-compatible (bug-fixes only).
- Q: Minimum deprecation notice window before removal? → A: One minor release — a
  symbol must be marked `//Deprecated:` in at least one published `0.MINOR.0` release
  before it may be removed in a later release.
- Q: How do machine-payload (`ax.Error` / `__schema`) shape changes count as breaking?
  → A: Additive-tolerant — adding a new field is NON-breaking (consumers MUST tolerate
  unknown fields); removing, renaming, re-typing, or changing the semantics of an
  existing field IS breaking.
- Q: Should the release-please config enforce the pre-v1.0 contract? → A: Yes — while in
  `0.x`, `feat:` MUST bump the minor (not patch), so `bump-patch-for-minor-pre-major`
  is set to `false` (was `true`); `bump-minor-pre-major` stays `true` so breaking
  changes bump the minor and never auto-promote to `1.0.0`.

## User Scenarios & Testing

### User Story 1 — Consumer Understands What Is Safe to Depend On (Priority: P1)

A Go developer evaluating whether to adopt ax-go in a production CLI needs to know
what stability guarantees the module offers at its current `v0.x` version, what counts
as a breaking change, and whether a `0.x.0` bump may break their build.

**Why this priority**: Without a documented policy, every consumer guesses. The first
downstream project that upgrades and gets a broken build will create support cost and
erode trust. This is the most urgent gap.

**Independent Test**: Read the README `Status` section and the constitution —
a reader unfamiliar with ax-go can answer all three questions above (stability tier,
definition of breaking, pre-v1.0 behavior) without consulting the maintainer.

**Acceptance Scenarios**:

1. **Given** the README, **When** a developer reads the `Status` section,
   **Then** they find the current stability tier (pre-v1.0 or stable), the SemVer
   contract for that tier, and a pointer to the full policy in the constitution.
2. **Given** the constitution, **When** a developer reads the new Stability
   principle, **Then** they can determine unambiguously whether renaming an exported
   function is a breaking change and what version bump it requires.
3. **Given** a future PR that renames an exported identifier, **When** reviewers
   apply the policy, **Then** the policy resolves the question without judgment calls.

---

### User Story 2 — Contributor Knows How to Deprecate an Identifier (Priority: P2)

A maintainer wants to deprecate `ax.OldFunction` before removing it in a future
version. They need to know the required `//Deprecated:` comment format, the
minimum notice window, and what migration guidance must accompany the comment.

**Why this priority**: Without a deprecation policy, deprecated identifiers are
either removed without warning (breaking consumers) or accumulate indefinitely
(polluting the API surface). The policy makes the process predictable.

**Independent Test**: A maintainer can deprecate a symbol end-to-end — add the
comment, verify `staticcheck SA1019` reports it to callers, wait the required notice
period, then remove it — by following only the constitution's Deprecation Lifecycle
principle.

**Acceptance Scenarios**:

1. **Given** the constitution Deprecation principle, **When** a maintainer adds
   `//Deprecated: Use NewFunction instead.` above an exported symbol, **Then**
   the comment satisfies the documented format requirement.
2. **Given** a deprecated symbol, **When** a downstream project runs
   `golangci-lint run`, **Then** `staticcheck SA1019` surfaces the deprecation
   at every call site (already enabled; scenario confirms existing behavior).
3. **Given** the notice period policy, **When** a maintainer proposes removing
   a deprecated symbol before the minimum window has elapsed, **Then** the PR
   review process rejects the removal with a reference to the policy.

---

### User Story 3 — Retroactive Evaluation of the ax.Logger Interface Change (Priority: P3)

The issue specifically asks: under the new policy, was the `concrete *zerolog.Logger →
ax.Logger interface` change introduced in the structured-logger revision a breaking change
requiring a major bump? The policy must be evaluated against this concrete example
and the verdict recorded.

**Why this priority**: This is the single concrete test case the issue authors
identified. Getting an answer anchors the policy to reality and prevents it from
being too abstract to apply.

**Independent Test**: The `research.md` for this feature records the policy verdict
on the ax.Logger change with explicit reasoning, so a future reviewer can see the
policy applied to a real case.

**Acceptance Scenarios**:

1. **Given** the Stability principle's definition of "breaking change," **When**
   applied to the `concrete *zerolog.Logger → ax.Logger interface` change, **Then**
   the verdict (breaking or non-breaking) and reasoning are recorded in `research.md`.
2. **Given** the verdict, **When** the current version history is reviewed, **Then**
   the maintainer can confirm whether a retrospective version bump is or is not
   needed.

---

### Edge Cases

- What constitutes "breaking" for machine-payload output (the `ax.Error` envelope,
  `__schema` output shape) as opposed to Go API surface? **Resolved** (see
  Clarifications 2026-06-16): additive-tolerant — adding a field is NON-breaking
  (consumers MUST tolerate unknown fields), while removing, renaming, re-typing, or
  changing the semantics of an existing field IS breaking.
- How does the stability tier apply to `internal/` packages? (Expected: exempt from
  the SemVer guarantee, since the Go toolchain already blocks external import.)
- How does the policy handle experimental sub-packages (e.g., a future `ax/x/`)?
  (Assumption: treat as pre-v1.0 regardless of root package version, document
  separately in the constitution if introduced.)
- Deprecation without an alternative: is `//Deprecated:` valid when there is no
  replacement yet? (Expected: yes, but the comment must explain the removal reason.)

## Requirements

### Functional Requirements

- **FR-001**: The constitution MUST include a Stability & SemVer principle that defines
  the pre-v1.0 contract as **pragmatic pre-v1.0**: a `0.MINOR.0` bump MAY contain
  breaking changes, while `0.x.PATCH` releases MUST stay backward-compatible (bug-fixes
  only). The "breaking change" definition MUST cover the Go API surface AND
  machine-payload output shapes, where for machine-payload (`ax.Error` envelope,
  `__schema` output) adding a new field is NON-breaking (consumers MUST tolerate
  unknown fields) while removing, renaming, re-typing, or changing the semantics of an
  existing field IS breaking. The principle MUST also define the stability tier per
  package kind (`internal/` exempt, root package governed, experimental packages
  separately noted).
- **FR-002**: The constitution MUST include a Deprecation Lifecycle principle that
  documents the `//Deprecated:` comment format (Go convention), the minimum notice
  period before removal of **at least one published `0.MINOR.0` release** in which the
  symbol carries the `//Deprecated:` comment before it may be removed in a later
  release, the required migration-note content in the deprecation comment, and the
  tooling expectation (`staticcheck SA1019` already enabled — confirm and document).
- **FR-003**: The README `Status` section MUST be updated to reflect the documented
  stability tier for the current release series and MUST link to or reference the
  constitution's Stability principle for the full policy.
- **FR-004**: `AGENTS.md` MUST reference both new constitution principles under
  "Accepted Architecture" (replacing the original issue's request to reference
  ADR-0013 and ADR-0014).
- **FR-005**: `research.md` MUST record the retroactive verdict on whether the
  `concrete *zerolog.Logger → ax.Logger interface` change was a breaking change under
  the documented policy, with explicit reasoning.
- **FR-006**: `research.md` MUST confirm that `staticcheck SA1019` is already enabled
  in `.golangci.yml` via `checks: [all]` without exclusion of SA1019, so no linting
  config change is required.
- **FR-007**: The constitution amendment MUST apply the correct semantic version bump
  per the constitution's versioning policy: adding a new principle is MINOR; no
  backward-incompatible removal or redefinition is occurring.
- **FR-008**: `release-please-config.json` MUST be consistent with the Stability &
  SemVer principle so the documented contract is enforced by the release tooling, not
  just prose. While the module is in `0.x`: breaking changes MUST bump the minor digit
  (`bump-minor-pre-major: true`, never auto-promoting to `1.0.0`) and `feat:` commits
  MUST bump the minor digit, NOT the patch digit (`bump-patch-for-minor-pre-major:
  false`), so that patch releases remain bug-fixes-only per FR-001. The current config
  sets `bump-patch-for-minor-pre-major: true`, which routes features into patch
  releases and contradicts the policy; it MUST be flipped to `false`.

### Key Entities

- **Constitution principle (Stability & SemVer)**: A new section in
  `.specify/memory/constitution.md` defining the stability contract. Cross-cutting;
  supersedes any future ADR request on this topic.
- **Constitution principle (Deprecation Lifecycle)**: A new section defining the
  deprecation workflow. Cross-cutting.
- **research.md**: The feature-specific Phase 0 document capturing absorbed ADR
  decisions, retroactive verdicts, and findings that do not belong in the constitution
  body itself.
- **Sync Impact Report**: The mandatory amendment metadata block appended to the
  constitution whenever it is versioned, describing which principles were modified and
  why the version was bumped.
- **release-please-config.json**: The release-automation config whose pre-v1.0 bump
  flags (`bump-minor-pre-major`, `bump-patch-for-minor-pre-major`) operationalize the
  Stability & SemVer principle. The tooling enforcement of the documented policy.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A developer new to ax-go can read the README and constitution and
  answer "what is the stability guarantee?" without consulting the maintainer —
  verified by having a second person attempt it before the PR is merged.
- **SC-002**: The `//Deprecated:` comment format can be validated mechanically —
  `golangci-lint run` already reports SA1019 at every deprecated call site (confirm
  with a test case in `research.md`).
- **SC-003**: The retroactive verdict on the `ax.Logger` interface change is
  documented in `research.md` within this feature's scope (100% of identified
  test cases resolved).
- **SC-004**: No new ADR files are created — the pull request for this feature
  contains zero new files under `docs/adr/`. This is directly verifiable in CI or
  during review.
- **SC-005**: The constitution version is bumped correctly (MINOR, since two new
  principles are added) and the Sync Impact Report is present in the file.
- **SC-006**: All downstream derived documents (AGENTS.md, README) are updated in
  the same PR as the constitution amendment, with no follow-up work left open.
- **SC-007**: `release-please-config.json` matches the documented pre-v1.0 contract —
  `bump-minor-pre-major: true` and `bump-patch-for-minor-pre-major: false` — so that
  `feat:` bumps the minor, `fix:` bumps the patch, and breaking changes bump the minor
  without auto-promoting to `1.0.0`. Directly verifiable by inspecting the config flags
  in review or CI.

## Assumptions

- Source inputs: GitHub issue #17. Governing ADRs: none directly (ADR-0013 and
  ADR-0014 do not exist; the issue proposes their creation, which conflicts with the
  constitution). Decisions are absorbed into `research.md` and elevated to constitution
  principles.
- The pre-v1.0 contract decision is **resolved** (see Clarifications 2026-06-16):
  pragmatic pre-v1.0 — `0.MINOR.0` bumps MAY break; `0.x.PATCH` releases MUST stay
  backward-compatible. `research.md` Phase 0 will transcribe this decision and its
  rejected alternatives (strict SemVer, deprecate-first).
- `internal/` packages are assumed to be exempt from the SemVer stability guarantee
  (the Go toolchain already enforces this via import restrictions).
- `staticcheck SA1019` is assumed to already be enabled in `.golangci.yml` based on
  inspection; `research.md` will confirm this finding explicitly.
- The minimum notice period is **resolved** (see Clarifications 2026-06-16): one minor
  release — a symbol must be `//Deprecated:` in at least one published `0.MINOR.0`
  release before removal. `research.md` Phase 0 will transcribe this and its rejected
  alternatives (two minor releases, one major release).
- Machine-payload output stability (envelope schemas, `__schema` output) is considered
  part of the public API contract, not just the Go type surface. **Confirmed and
  resolved** (see Clarifications 2026-06-16): the Stability principle codifies an
  additive-tolerant rule — new fields are non-breaking (consumers MUST tolerate unknown
  fields); removal, rename, re-type, or semantic change of an existing field is
  breaking.
- No existing identifier in ax-go is currently deprecated; the Deprecation Lifecycle
  principle introduces the mechanism for future use only.
