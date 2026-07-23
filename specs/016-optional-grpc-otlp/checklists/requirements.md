# Specification Quality Checklist: Independently Opt-Out OTLP Export and gRPC Dial Adapter

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-07-22
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

### Validation iteration 1 (2026-07-22)

**Passing with note — "No implementation details"**: The requirements deliberately
avoid naming the opt-out mechanism (build tags vs. package split). That choice is
the feature's core research question and is left to planning. FR-001 through FR-004
state the *outcome* ("a build-time way to decline", "default off", "no consumer
source change"), which constrains the mechanism without pre-deciding it. The
Assumptions section does cite concrete repository facts (package counts, absence of
build tags) — these are grounding evidence, not requirements.

**Passing with note — "Success criteria are technology-agnostic"**: SC-001 through
SC-003 reference binary size and dependency-package counts. For a library
whose entire user value here *is* dependency footprint, these are the user-facing
outcome, not implementation detail. They are stated as measurable results
(percentage reduction, exact count of zero) rather than as prescribed techniques.

**FAILING — "No [NEEDS CLARIFICATION] markers remain"**: Two clarifications
outstanding (surface-gate scope; fate of the dial helper under its decline),
recorded in the spec's `Outstanding Clarifications` section rather than as inline
markers. The first was blocking — issue #143 directs the feature to *extend*
surface-gate tooling that does not exist in this repository.

### Validation iteration 2 (2026-07-22)

Both clarifications resolved by the user and encoded into the spec; the
`Outstanding Clarifications` section is now `Resolved Clarifications`, carrying
decision, rationale, and consequences for each. All checklist items pass.

**Q1 → "Build the full gate"**. This feature now creates net-new public-surface
inventory tooling rather than extending anything. Encoded as FR-018, FR-019,
FR-020, SC-009, SC-010, and the gate-coverage clause in SC-012; User Story 4 gained
two acceptance scenarios.

**Q2 → "Absent + guiding explanation"**. Encoded as FR-022, FR-023, FR-024, and
SC-011, plus a revised edge case.

**Scope warning carried into planning**: the resolution of Q1 converts an
"extend existing tooling" line item into building a whole new CI gate. Combined
with introducing the repository's first production build-tag mechanism, this
feature now carries two substantial, largely independent workstreams. The
`effort/large` label on issue #143 is, if anything, an understatement.
`/speckit-plan` should sequence these so the opt-out mechanism is not blocked on
the gate — the size win in SC-001 does not depend on it.

**Requirement renumbering**: FR-019 through FR-023 in iteration 1 were renumbered
to FR-021, FR-025 through FR-028 to accommodate the inserted requirements. No
requirement was dropped.

### Discrepancy log

| Issue #143 claim | Repository reality at `741a8d4` | Impact |
|---|---|---|
| 68 gRPC / 36 protobuf / grpc-gateway packages in root facade | Confirmed: 68 / 36 / 3, 409 total | None — verified |
| Zero production `//go:build` lines | Confirmed: zero `//go:build` lines anywhere, including tests | None — verified |
| `//go:build` matches "only `internal/cmd/surfacecheck` test fixtures" | No such matches exist; no such package exists | Minor — issue text inaccurate |
| `internal/cmd/surfacecheck/inventory.go` sweeps six profiles | Package does not exist | **Resolved** — feature now builds it (FR-018) |
| `internal/cmd/surfacecheck/baseline.json` contains `func:GRPCDial` | File does not exist | **Resolved** — baseline is a new deliverable (FR-019) |
| `make surface-check` target | No such target in `Makefile` | **Resolved** — local entry point is a new deliverable (FR-020) |
| Cross-compile matrix covers six `GOOS/GOARCH` profiles | Confirmed: `{linux,darwin,windows} × {amd64,arm64}` | None — verified |
| `internal/testutil/imports.go` provides `ForbiddenImport` / `AssertNoForbiddenImports` | Confirmed, plus `ForbiddenRuntimeImports` and `AssertContractPackageIsolated` | None — verified |
