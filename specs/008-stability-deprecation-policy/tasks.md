---
description: "Task list for Stability + Deprecation Policy"
---

# Tasks: Stability + Deprecation Policy

**Input**: Design documents from `/specs/008-stability-deprecation-policy/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md,
contracts/stability-policy.md, quickstart.md

**Tests**: None. This is a documentation/governance feature with **no Go source code**.
No test tasks are generated; verification is documentary (config inspection, file-absence,
prose-vs-contract-table consistency, second-reader comprehension).

**Organization**: Tasks are grouped by user story (P1 → P2 → P3) so each is independently
testable. Note two structural realities specific to this feature:

- **`.specify/memory/constitution.md` is a single shared file.** The two new principles
  (US1, US2) and the Sync Impact Report (Polish) all edit it, so those edits are
  **sequential** — they are NOT marked `[P]` relative to each other.
- **Governing ADR(s): N/A.** No `docs/adr/` file governs this feature, so there is **no
  ADR-retirement task** (the template's FINAL Polish task is intentionally omitted).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1 / US2 / US3 (maps to spec.md user stories)
- Exact file paths are included in every task

---

## Phase 1: Setup (Verification Baselines)

**Purpose**: Establish the pre-edit baselines the success criteria measure against.

- [X] T001 [P] Confirm zero new ADR files: verify `docs/adr/0013*` and `docs/adr/0014*` are absent (`ls docs/adr/0013* docs/adr/0014* 2>/dev/null` returns nothing) — baseline for SC-004.
- [X] T002 [P] Ensure `release-please-config.json` `packages["."]` has `bump-minor-pre-major: true` and `bump-patch-for-minor-pre-major: false` — **apply the edit if absent** (do not rely on an uncommitted working-tree change), then confirm. Idempotent: re-running lands the same state. Satisfies FR-008 / SC-007.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core prerequisites that must exist before user-story edits.

**No foundational tasks required.** This feature adds governance prose and reconciles
derived docs; there is no shared infrastructure to build. The only cross-story coupling is
that US1 and US2 edit the same file (`constitution.md`) and must be applied sequentially —
captured in Dependencies below, not as a blocking task.

**Checkpoint**: Baselines confirmed — user-story work can begin.

---

## Phase 3: User Story 1 - Consumer Understands What Is Safe to Depend On (Priority: P1) 🎯 MVP

**Goal**: A developer can determine ax-go's stability tier, what counts as a breaking
change, and whether a `0.x.0` bump may break their build — from the README + constitution
alone.

**Independent Test**: A second person, unfamiliar with ax-go, reads the README `Status`
section and the constitution's Stability principle and correctly answers (1) the stability
tier, (2) whether renaming an exported function is breaking, and (3) whether a `0.x.0` bump
may break their build — without consulting the maintainer.

### Implementation for User Story 1

- [X] T003 [US1] Add the **Stability & SemVer** principle to `.specify/memory/constitution.md` as **Principle XI** (§Core Principles, immediately after Principle X. Idiomatic Go & Dependency Minimalism): state the pragmatic pre-v1.0 contract (`0.MINOR.0` MAY break; `0.x.PATCH` is bug-fixes-only; never auto-promote to `1.0.0`), the breaking-change definition over BOTH the Go API surface AND machine-payload shapes (additive-tolerant: add = non-breaking, remove/rename/re-type/semantic-change = breaking), and the per-package-kind tier (`internal/` exempt, root `ax` governed, experimental packages noted). Prose MUST match `contracts/stability-policy.md` Contract A. Satisfies FR-001.
- [X] T004 [US1] Update the `> **Status: …**` blockquote in `README.md` (currently lines 9–15) to document the current pre-v1.0 stability tier, the SemVer contract (patch = safe upgrade; minor may break; no auto-`1.0.0`), and a pointer to the constitution's Stability principle for the full policy. Satisfies FR-003. (Depends on T003 — links the principle added there.)

**Checkpoint**: US1 independently testable — the stability guarantee is fully readable from README + constitution.

---

## Phase 4: User Story 2 - Contributor Knows How to Deprecate an Identifier (Priority: P2)

**Goal**: A maintainer can deprecate an exported symbol end-to-end — comment format, notice
window, migration note, tooling — by following the constitution's Deprecation Lifecycle
principle.

**Independent Test**: Following only the Deprecation Lifecycle principle, a maintainer can
add a `//Deprecated:` comment, confirm `staticcheck SA1019` reports it at call sites, wait
the required window, then remove it.

### Implementation for User Story 2

- [X] T005 [US2] Add the **Deprecation Lifecycle** principle to `.specify/memory/constitution.md` as **Principle XII** (§Core Principles, immediately after Principle XI from T003): document the `//Deprecated:` Go-convention comment format, the minimum notice window (≥1 published `0.MINOR.0` release carrying the comment before removal), the required migration-note content (replacement, or removal reason when none), and the tooling expectation (`staticcheck SA1019` — already enabled, confirm and document). Prose MUST match `contracts/stability-policy.md` Contract B. Satisfies FR-002. (Sequential after T003 — same file.)
- [X] T006 [P] [US2] Confirm `staticcheck SA1019` is already enabled and requires no config change: verify `.golangci.yml` `linters.settings.staticcheck.checks` is `["all", "-ST1000", "-ST1016", "-QF1008"]` (SA1019 NOT excluded) and that `research.md` Decision 6 records this finding. No edit to `.golangci.yml`. Satisfies FR-006 / SC-002.

**Checkpoint**: US2 independently testable — a maintainer can run the full deprecation lifecycle from the principle.

---

## Phase 5: User Story 3 - Retroactive Evaluation of the ax.Logger Interface Change (Priority: P3)

**Goal**: The policy is applied to the concrete `*zerolog.Logger → ax.Logger` change and the
verdict is recorded with reasoning.

**Independent Test**: `research.md` records the verdict on the `ax.Logger` change with
explicit reasoning, so a future reviewer can see the policy applied to a real case.

### Implementation for User Story 3

- [X] T007 [P] [US3] Verify `research.md` Decision 5 records the retroactive verdict on the `concrete *zerolog.Logger → ax.Logger interface` change — **breaking** (re-type of an exported symbol per Contract A) but owing only a **minor** bump pre-v1.0 (no retrospective major), with explicit reasoning — and confirm the version-history consequence (no major bump owed) is stated. Satisfies FR-005 / SC-003. (Authored during planning; this task finalizes/validates it.)

**Checkpoint**: All three user stories independently satisfied.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Version the amendment, reconcile derived docs, and validate end-to-end. These
tasks depend on the principle content from US1 (T003) and US2 (T005) being in place.

- [X] T008 Prepend/update the **Sync Impact Report** in `.specify/memory/constitution.md` and bump `**Version**: 1.1.0` → `1.2.0` with `**Last Amended**: 2026-06-16`. The report MUST name both added principles (XI. Stability & SemVer; XII. Deprecation Lifecycle), give the MINOR-bump rationale (two new principles; nothing removed/redefined), state that plan/spec/tasks templates need no change, and list the derived docs reconciled in this PR (AGENTS.md, README). Satisfies FR-007 / SC-005. (Depends on T003 and T005 — same file; must run after both principles exist.)
- [X] T009 [P] Update `AGENTS.md` "Accepted Architecture" section to reference BOTH new constitution principles (Stability & SemVer; Deprecation Lifecycle), replacing the original issue's request to reference ADR-0013/ADR-0014. Reference the constitution principles, NOT any ADR. Satisfies FR-004 / SC-006.
- [X] T010 Run `quickstart.md` validation: have a second reader answer the three stability questions from README + constitution (SC-001), and confirm the constitution prose for both principles matches the decision tables in `contracts/stability-policy.md` (Contracts A & B) and the bump rules match `release-please-config.json` (T002). Re-confirm SC-004 (zero new `docs/adr/` files in the diff).
- [X] T011 [P] Run `markdownlint` on all changed markdown (`.specify/memory/constitution.md`, `README.md`, `AGENTS.md`, and `specs/008-stability-deprecation-policy/*.md`) and fix any issues. Do NOT edit `CHANGELOG.md` (release-please-owned).

> **No ADR-retirement task**: Governing ADR(s) = N/A (no `docs/adr/` file governs this feature). The template's FINAL ADR-deletion task is intentionally omitted (SC-004).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T001, T002 are parallel.
- **Foundational (Phase 2)**: None (no blocking infra).
- **User Stories (Phase 3–5)**: Begin after Setup.
  - US1 and US2 both edit `constitution.md` → their constitution edits (T003, T005) are
    **sequential**: T005 after T003.
  - US3 (T007) is file-independent (`research.md`) → fully parallel with US1/US2.
- **Polish (Phase 6)**: T008 depends on T003 + T005 (must name both principles). T009, T011
  are file-independent and parallel. T010 is the final validation gate.

### User Story Dependencies

- **US1 (P1)**: Independent. The MVP.
- **US2 (P2)**: Its constitution edit (T005) is sequenced after T003 only because of the
  shared file — the story is otherwise independent and independently testable.
- **US3 (P3)**: Fully independent (verifies a planning artifact).

### Within Each Story

- US1: T003 (principle) → T004 (README links it).
- US2: T005 (principle); T006 (tooling confirmation) is independent and parallel.
- US3: T007 standalone.

### Parallel Opportunities

- Setup: T001 ∥ T002.
- Cross-story: T007 (US3) runs in parallel with all US1/US2 work (different file).
- Within US2: T006 ∥ T005.
- Polish: T009 ∥ T011 (different files); both before T010's final gate.
- **Not parallel**: T003, T005, T008 all touch `constitution.md` — strictly sequential in
  that order.

---

## Parallel Example: early parallel batch

```bash
# Setup baselines + the US3 verification can all run together (independent files):
Task: "T001 Confirm docs/adr/0013* and 0014* absent"
Task: "T002 Verify release-please-config.json bump flags"
Task: "T007 Verify research.md Decision 5 records the ax.Logger verdict"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup (T001, T002).
2. Phase 3: US1 — add the Stability & SemVer principle (T003), update README Status (T004).
3. **STOP and VALIDATE**: a second reader answers the three stability questions from README +
   constitution. This alone closes the most urgent gap (P1).

### Incremental Delivery

1. Setup → baselines confirmed.
2. US1 → stability guarantee documented (MVP).
3. US2 → deprecation lifecycle documented.
4. US3 → retroactive verdict confirmed.
5. Polish → version bump + Sync Impact Report (T008), reconcile AGENTS.md (T009),
   validate (T010), markdownlint (T011).

### Critical Path

`T003 → T005 → T008` (the three sequential `constitution.md` edits) is the critical path;
everything else parallelizes around it.

---

## Notes

- [P] = different files, no dependency on an incomplete task.
- The three `constitution.md` edits (T003, T005, T008) are deliberately un-parallelized.
- No Go source, no tests, no `.golangci.yml` change, no new ADR files.
- `CHANGELOG.md` is release-please-owned — never hand-edit it.
- The constitution amendment (T008) bumps 1.1.0 → 1.2.0 (MINOR) per its own versioning policy.
