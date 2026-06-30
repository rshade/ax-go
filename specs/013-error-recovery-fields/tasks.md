---
description: "Task list for error-envelope recovery & remediation fields"
---

# Tasks: Error-envelope recovery & remediation fields

**Input**: Design documents from `specs/013-error-recovery-fields/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/error-envelope.md, quickstart.md

**Tests**: INCLUDED and REQUIRED — Constitution Principle VII (Test-First Discipline) is
non-negotiable in this repo. New behavior (US1, US2) lands test-first; backward-compat
(US3) lands as green guardrail tests.

**Organization**: Tasks are grouped by user story so each story is an independently
testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1 / US2 / US3 (maps to spec.md user stories)
- Exact file paths are included in each description

## Path Conventions

Single Go module. Envelope source of truth: `contract/error.go`; root facade:
`error.go`. Tests at module root (`error_test.go`, `error_fuzz_test.go`,
`example_test.go`); golden fixtures under `testdata/`.

> **ADR retirement**: NONE. Governing ADR-0002 (`docs/adr/0002-error-envelope-schema.md`)
> was already deleted in PR #79 and its decisions are transcribed in
> `research.md` → "Decision Records Absorbed". There is no ADR file to remove, so
> this tasks list has no final ADR-retirement task (Constitution §Governance gate
> satisfied retroactively).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish a known-green baseline before any change.

- [x] T001 Record the green baseline: run `go test -race ./...` and `make cover-check`; confirm `testdata/error_envelope.golden.json` is unmodified and note its exact bytes (no source edits in this task).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: None blocking — both new fields are additive scalars on an existing struct
with no shared scaffolding to build first. US1 begins immediately.

*(No foundational tasks. The struct and option-builder mechanism already exist in
`contract/error.go`; each story appends its own field + builder.)*

---

## Phase 3: User Story 1 — Retry-safety signal (Priority: P1) 🎯 MVP

**Goal**: A producer can mark a failure as explicitly retryable, explicitly
non-retryable, or leave it unspecified, and an agent reads a tri-state machine value.

**Independent Test**: Build errors with `WithRetryable(true)`, `WithRetryable(false)`,
and no option; assert the envelope shows `"retryable":true`, `"retryable":false`, and an
absent key respectively.

- [x] T002 [US1] Write failing test in `error_test.go` (`TestErrorRetryable`): table cases for `WithRetryable(true)` → `"retryable":true`, `WithRetryable(false)` → `"retryable":false`, and no option → no `retryable` key. Confirm it fails to compile/run (builder absent).
- [x] T003 [US1] In `contract/error.go`, add field `Retryable *bool` with tag `json:"retryable,omitempty"` immediately after `Suggestions`, and add `WithRetryable(retryable bool) ErrorOption` storing `&retryable` (data-model VR-1, research D1/D7).
- [x] T004 [US1] In `error.go`, re-export `WithRetryable` as a thin delegate to `contract.WithRetryable`; add its exported doc comment. Run `go test ./...` → T002 passes.
- [x] T005 [US1] In `error_test.go`, extend `TestRootErrorEnvelopeMatchesIsolatedContractShape` with `WithRetryable(true)` on both root and contract builders to prove byte-identical facade output (FR-008).
- [x] T006 [P] [US1] In `example_test.go`, add `ExampleWithRetryable` (with `// Output:`) demonstrating the tri-state; ensure `go test` runs it and `make doc-coverage` stays green.

**Checkpoint**: US1 is independently shippable — retry-safety works end to end.

---

## Phase 4: User Story 2 — Backoff hint (Priority: P2)

**Goal**: A producer can attach a relative, deterministic backoff window
(`retry_after_seconds`) to network/timeout failures.

**Independent Test**: Build an error with `WithRetryAfterSeconds(30)`; assert
`"retry_after_seconds":30`, that a negative value yields an absent key, and that two
marshals are byte-identical.

- [x] T007 [US2] Write failing test in `error_test.go` (`TestErrorRetryAfterSeconds`): cases for `WithRetryAfterSeconds(30)` → `"retry_after_seconds":30`, `WithRetryAfterSeconds(0)` → key absent (omitempty), `WithRetryAfterSeconds(-5)` → key absent (VR-2 normalization), and a determinism check (two `WriteError` calls produce identical bytes). Confirm it fails (builder absent).
- [x] T008 [US2] In `contract/error.go`, add field `RetryAfterSeconds int64` with tag `json:"retry_after_seconds,omitempty"` immediately after `Retryable`, and add `WithRetryAfterSeconds(seconds int64) ErrorOption` that ignores `seconds < 0` (research D2/D3).
- [x] T009 [US2] In `error.go`, re-export `WithRetryAfterSeconds` as a thin delegate; add its exported doc comment. Run `go test ./...` → T007 passes.
- [x] T010 [US2] Create golden fixture `testdata/error_recovery_envelope.golden.json` with the canonical populated bytes from `contracts/error-envelope.md` (key order `…,suggestions,retryable,retry_after_seconds`), and add `TestWriteErrorRecoveryEnvelope` in `error_test.go` that builds the matching error and asserts against it via `assertGolden` (FR-011, SC-004).
- [x] T011 [US2] Extend the facade-parity test (T005) to also set `WithRetryAfterSeconds`, proving root == contract bytes for the fully-populated shape.

**Checkpoint**: US2 shippable — backoff guidance works and is golden-locked.

---

## Phase 5: User Story 3 — Backward compatibility & invariants (Priority: P3)

**Goal**: Producers/consumers not using the new fields observe no change; exit codes
and error chains are unaffected.

**Independent Test**: Re-run the pre-feature golden corpus and exit-code/error-chain
tests unchanged; all stay green.

- [x] T012 [US3] Add `TestErrorDefaultShapeUnchanged` in `error_test.go`: an error built with NO recovery options marshals with neither `retryable` nor `retry_after_seconds` present (FR-005/FR-006). Must be green pre- and post-implementation (regression guardrail).
- [x] T013 [US3] Extend `TestErrorExitCodeContextErrors` and `TestErrorCauseChain` in `error_test.go` with a case that also sets `WithRetryable`/`WithRetryAfterSeconds`, asserting `ErrorExitCode` and `errors.Is`/`errors.As` are unaffected (FR-007, SC-005).
- [x] T014 [P] [US3] In `error_fuzz_test.go`, add seed corpus entries exercising `retryable` and `retry_after_seconds` so `FuzzErrorEnvelope`/`FuzzErrorEnvelopeUnmarshal` round-trip the new fields without panic.

**Checkpoint**: All three stories complete; determinism and compatibility proven.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T015 [P] Document the two new envelope fields (`retryable`, `retry_after_seconds`) in `README.md` under the error-envelope/public-contract section, noting tri-state semantics and relative-seconds determinism.
- [x] T016 [P] Update `examples/integration/` if a failing-command path should demonstrate the recovery fields; refresh any affected example golden/test.
- [x] T017 Run the full quality gate and fix any findings: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./...`, `make doc-coverage`, `make cover-check`. All must be clean.
- [x] T018 Confirm the API Diff scope: new exported `WithRetryable`/`WithRetryAfterSeconds` are additive (no `breaking-change-approved` label needed); capture the user-facing change for the `feat:` Conventional Commit (CHANGELOG is release-please-managed — do NOT hand-edit).

---

## Dependencies & Execution Order

- **T001** (baseline) first.
- **US1 (T002–T006)** is the MVP and lands before US2 (both edit `contract/error.go` and
  `error.go`, so their implementation tasks are sequential, not parallel).
- **US2 (T007–T011)** after US1.
- **US3 (T012–T014)** after the fields exist (needs US1 + US2 for T013/T014 recovery
  cases; T012 can be written anytime).
- **Polish (T015–T018)** last; T017 gate is the final acceptance.
- `[P]` tasks (T006, T014, T015, T016) touch distinct files and may run in parallel
  within their phase.

## Implementation Strategy

- **MVP = User Story 1** (`retryable`). It delivers the core AX recovery signal on its
  own; ship/verify before adding US2.
- Each story is test-first: write the failing test (T002/T007), implement, go green.
- US3 is guardrails — its tests protect the determinism/compat contract and must never
  go red.

## Task Summary

- **Total**: 18 tasks (T001–T018)
- **Per story**: Setup 1 · Foundational 0 · US1 5 (T002–T006) · US2 5 (T007–T011) · US3 3 (T012–T014) · Polish 4 (T015–T018)
- **Parallel opportunities**: T006, T014, T015, T016
- **MVP scope**: US1 (T001 → T002–T006)
