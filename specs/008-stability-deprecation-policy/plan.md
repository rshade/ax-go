# Implementation Plan: Stability + Deprecation Policy

**Branch**: `008-stability-deprecation-policy` | **Date**: 2026-06-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/008-stability-deprecation-policy/spec.md`

## Summary

Codify ax-go's stability guarantees and deprecation workflow as governance, not prose
scattered across docs. Two new constitution principles are added — a **Stability & SemVer**
principle (pragmatic pre-v1.0: `0.MINOR.0` MAY break, `0.x.PATCH` is bug-fix-only; breaking
defined over both the Go API surface and machine-payload shapes with an additive-tolerant
rule) and a **Deprecation Lifecycle** principle (`//Deprecated:` Go-convention format,
minimum one published `0.MINOR.0` notice window, mandatory migration note, `staticcheck
SA1019` tooling). The amendment bumps the constitution **1.1.0 → 1.2.0** (MINOR, two new
principles) with a Sync Impact Report. Derived docs (`AGENTS.md`, README `Status`) are
reconciled in the same change, `release-please-config.json` is aligned to enforce the
pre-v1.0 contract in tooling (already staged: `bump-patch-for-minor-pre-major: false`), and
`research.md` records the retroactive verdict on the `*zerolog.Logger → ax.Logger` change
plus confirmation that `staticcheck SA1019` is already enabled.

This is a documentation/governance feature. It ships **no Go source code** and therefore no
new tests, examples, or golden fixtures.

## Technical Context

**Language/Version**: Go 1.26.4 (module `github.com/rshade/ax-go`) — context only; no Go
source is added or changed by this feature.

**Primary Dependencies**: None added. Tooling touched: release-please (config), golangci-lint
/ staticcheck SA1019 (confirmation only — no config change).

**Storage**: N/A.

**Testing**: No code under test. Verification is documentary and tooling-level: SC-001
(second-reader comprehension check), SC-002 (`golangci-lint run` reports SA1019 at a
deprecated call site — confirmation of existing behavior, captured in research.md), SC-004
(zero new files under `docs/adr/`), SC-005 (constitution version + Sync Impact Report),
SC-007 (release-please flag inspection).

**Target Platform**: N/A (governance + repo metadata).

**Project Type**: Single Go library/CLI foundation (`ax` at module root). This feature
touches only governance and derived documents — no source tree changes.

**Performance Goals**: N/A.

**Constraints**: Must NOT create or edit any ADR (Constitution §Governance — ADRs are
frozen). Must NOT auto-promote to `1.0.0` (`bump-minor-pre-major: true`). The amendment must
apply the correct version bump (MINOR) and carry a Sync Impact Report.

**Scale/Scope**: 5 edited artifacts: `.specify/memory/constitution.md` (two new principles +
Sync Impact Report + version bump), `AGENTS.md` (Accepted Architecture reference),
`README.md` (`Status` section), `release-please-config.json` (already staged), plus the
feature's own `research.md`. Zero source files.

**Governing ADR(s)**: N/A. The issue proposed creating `docs/adr/0013-*` and
`docs/adr/0014-*`; the constitution prohibits new ADRs, so they do not and will not exist
(verified: `docs/adr/0013*`, `docs/adr/0014*` absent). ADR-0009 (`logger-zerolog`) is the
*subject* of this feature's retroactive evaluation (User Story 3), **not** a governing ADR
of this feature; it is not absorbed or retired here. Therefore the ADR-absorption gate and
the ADR-retirement final task do NOT apply.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

This feature amends the constitution rather than implementing against it, so most code-facing
principles are N/A. Each is evaluated explicitly:

| Principle | Applies? | Status |
|-----------|----------|--------|
| I. Stream Separation | N/A | No code; no stdout/stderr behavior changes. |
| II. Deterministic Output & Exit Codes | N/A | No code. |
| III. Machine Discoverability (`__schema`) | Indirect | The new Stability principle *classifies* `__schema`/`ax.Error` shape changes as breaking (additive-tolerant). No `__schema` behavior changes. PASS. |
| IV. Agent-Safety Primitives | N/A | No code. |
| V. Asymmetric JSON I/O | N/A | No code. |
| VI. ADR-Governed Scope | PASS | New decisions recorded in `research.md` + elevated to constitution principles; **no new ADRs** (SC-004). Honors the workflow. |
| VII. Test-First Discipline | N/A | No code → no tests/examples/golden files. Verification is documentary (see Testing). |
| VIII. Observability & ID Discipline | N/A | No code. |
| IX. Security & Resource Safety | N/A | No code. |
| X. Idiomatic Go & Dependency Minimalism | N/A | No code; no dependencies added. |

**ADR absorption gate (Constitution §Governance)**: Governing ADR(s) = N/A → gate does NOT
apply. `research.md` does not require a "Decision Records Absorbed" section and `tasks.md`
does not require an ADR-retirement task. (If a future reviewer disagrees and treats ADR-0009
as in-scope, that would be a separate feature; this feature only *evaluates* it.)

**Amendment-procedure gate (Constitution §Governance)**: The amendment MUST (a) edit
`.specify/memory/constitution.md`, (b) prepend/append a Sync Impact Report, (c) apply a MINOR
bump 1.1.0 → 1.2.0 (two new principles; no removal/redefinition — FR-007), (d) re-sync
dependent templates if affected, and (e) reconcile derived docs (`AGENTS.md`, README) in the
**same** change (SC-006). Templates: the plan/spec/tasks templates do not encode a stability
or deprecation contract, so no template change is implied — this is asserted in Phase 1 and
re-confirmed post-design.

**Result**: PASS. No violations; Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/008-stability-deprecation-policy/
├── plan.md              # This file (/speckit-plan output)
├── spec.md              # Feature specification (input)
├── research.md          # Phase 0 output — decisions, ax.Logger verdict, SA1019 confirmation
├── data-model.md        # Phase 1 output — governance "entities" (principles, reports, flags)
├── quickstart.md        # Phase 1 output — how a maintainer applies the policy end-to-end
├── contracts/
│   └── stability-policy.md   # Phase 1 output — the breaking-change + deprecation decision tables
├── checklists/          # (pre-existing)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

No source code changes. This feature edits governance and derived documents only:

```text
.specify/memory/constitution.md   # + Stability & SemVer principle, + Deprecation Lifecycle
                                   #   principle, + Sync Impact Report, version 1.1.0 → 1.2.0
AGENTS.md                         # Accepted Architecture: reference both new principles
README.md                         # Status section: document current pre-v1.0 stability tier
release-please-config.json        # bump-patch-for-minor-pre-major: false (ALREADY STAGED)
```

`internal/`, `cmd/`, `examples/`, `testdata/`, and the root `ax` package are **untouched**.

**Structure Decision**: Governance-only change. The ax-go single-package layout (public `ax`
at module root, mechanics under `internal/`) is unchanged. The "interface" this feature
delivers is a policy contract, captured under `contracts/stability-policy.md` as a decision
table rather than a Go API.

## Complexity Tracking

> No Constitution Check violations. Table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
