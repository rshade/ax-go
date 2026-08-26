# Specification Quality Checklist: --yes no-prompt invariant (confirmation_required envelope)

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-07-28
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`

### Validation iteration 1 — 2026-07-28

Two `[NEEDS CLARIFICATION]` markers were raised, both deliberate. Each marked a
behavioral fork that changes the gate's observable contract rather than an
implementation detail:

- **FR-009** (human-interactive outcome): whether the gate exposes a third state the
  author must handle, or collapses to "approved" in human mode. This determines
  whether a human-facing CLI keeps a real confirmation prompt or silently loses it.
- **FR-010** (approval × dry-run): whether an active dry-run implies approval. This
  determines whether an agent can preview a gated command without first committing
  to the approval flag, and whether the two primitives stay orthogonal.

All other gaps in the source issue were closed with documented assumptions rather
than markers: no flag shorthand, no environment-variable fallback, no success-envelope
field, stdin left untouched as a data channel, and fail-closed behavior when mode is
unresolved. Each is recorded in the spec's Assumptions or Edge Cases section.

Prior open question from the source issue — whether to add a `hint` field to the
error envelope — was resolved before drafting and is recorded under Clarifications
rather than left as a marker.

### Validation iteration 2 — 2026-07-28

Both markers resolved and recorded under Clarifications:

- **FR-009** → a distinct third outcome. The gate reports approved, blocked, or
  prompt-required, and the author owns any actual prompt. Split into FR-009 and
  FR-009a, with SC-012 asserting the three are distinguishable without parsing an
  error message.
- **FR-010** → orthogonal; dry-run does not imply approval. Split into FR-010 and
  FR-010a, with SC-011 requiring all four approval × dry-run combinations to be
  tested and no dry-run to report success where the real run is blocked. This keeps
  the feature consistent with the shipped dry-run faithful-preview guarantee.

All checklist items now pass. Spec is ready for `/speckit-plan`.
