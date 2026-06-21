# Tasks: Import-Isolated Contracts

**Input**: Design documents from `/specs/010-import-isolated-contracts/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Required by the ax-go constitution. Write each test task first and verify it fails for the expected reason before implementing the corresponding behavior.

**Organization**: Tasks are grouped by user story so each increment can be implemented and tested independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it touches different files and has no dependency on incomplete tasks.
- **[Story]**: User-story label for story phases only.
- Every task includes exact file paths or repository-relative command scope.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the public package skeletons and shared testing location used by later phases.

- [X] T001 Create public package skeletons with package documentation in `contract/doc.go`, `config/doc.go`, `schema/doc.go`, and `id/doc.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Provide the shared import-dependency assertion helper used by contract-surface tests.

**Critical**: No user story implementation should begin until this helper exists, because every contract surface must prove import isolation.

- [X] T002 [P] Write failing dependency-graph helper tests in `internal/testutil/imports_test.go`
- [X] T003 Implement dependency-graph assertion helpers in `internal/testutil/imports.go`

**Checkpoint**: `go test -race ./internal/testutil/...` should pass, and user story test work can start.

---

## Phase 3: User Story 1 - Thin Consumer Imports Contracts (Priority: P1)

**Goal**: Thin consumers can import `contract`, `config`, `schema`, and `id` without importing the root runtime facade or forbidden runtime adapters.

**Independent Test**: A minimal consumer can compile against the new public surfaces, and package tests prove forbidden runtime dependencies are absent.

### Tests for User Story 1

- [X] T004 [P] [US1] Write failing contract package tests for exit codes, mode resolution, context metadata, error envelopes, and JSON writers in `contract/exit_test.go`, `contract/mode_test.go`, `contract/context_test.go`, `contract/error_test.go`, and `contract/json_test.go`
- [X] T005 [P] [US1] Write verified contract package examples for primary exported symbols in `contract/example_test.go`
- [X] T006 [P] [US1] Write failing ID package tests, fuzz tests, and examples for UUID v4 idempotency keys and UUID v7 entity IDs in `id/id_test.go`, `id/fuzz_test.go`, and `id/example_test.go`
- [X] T007 [P] [US1] Write failing config package tests, fuzz tests, and examples for bounded Hujson parse and patch behavior in `config/config_test.go`, `config/fuzz_test.go`, and `config/example_test.go`
- [X] T008 [P] [US1] Write failing schema package tests and examples for ax-native and MCP-compatible schema output in `schema/schema_test.go` and `schema/example_test.go`
- [X] T009 [P] [US1] Write failing import-isolation tests for every public contract surface in `contract/import_isolation_test.go`, `config/import_isolation_test.go`, `schema/import_isolation_test.go`, and `id/import_isolation_test.go`

### Implementation for User Story 1

- [X] T010 [US1] Implement exit-code, mode, and context contracts in `contract/exit.go`, `contract/mode.go`, and `contract/context.go`
- [X] T011 [US1] Implement success envelope, metadata, strict JSON writers, and error-envelope contracts in `contract/json.go` and `contract/error.go`
- [X] T012 [P] [US1] Implement UUID v4 idempotency-key and UUID v7 entity-ID helpers in `id/id.go`
- [X] T013 [US1] Implement isolated config parse and patch APIs backed by internal mechanics in `config/config.go`
- [X] T014 [US1] Implement isolated ax-native and MCP-compatible schema APIs backed by internal reflection/adapters in `schema/schema.go`
- [X] T015 [US1] Run the focused MVP gate `go test -race ./contract ./config ./schema ./id` from repository root `.` and fix failures in `contract/`, `config/`, `schema/`, and `id/`

**Checkpoint**: User Story 1 is complete when the focused MVP gate passes and the new packages satisfy the import-isolation contract.

---

## Phase 4: User Story 2 - Existing Root Package Users Remain Compatible (Priority: P2)

**Goal**: Existing root `ax` imports continue to work with unchanged public behavior while delegating to the new contract surfaces where practical.

**Independent Test**: Existing root-package tests, examples, and golden fixtures continue to pass, and new compatibility tests prove root wrappers match the isolated package behavior.

### Tests for User Story 2

- [X] T016 [P] [US2] Write failing root facade compatibility tests for contract symbols in `contract_facade_test.go`
- [X] T017 [P] [US2] Write failing root facade compatibility tests for config symbols and error codes in `config_facade_test.go`
- [X] T018 [P] [US2] Write failing root facade compatibility tests for schema builders and schema golden output in `schema_facade_test.go`
- [X] T019 [P] [US2] Write failing root facade compatibility tests for ID helpers in `id_facade_test.go`
- [X] T020 [P] [US2] Strengthen root golden-preservation assertions for error envelopes and schema output in `error_test.go` and `schema_test.go`

### Implementation for User Story 2

- [X] T021 [US2] Convert root exit-code, mode, context, JSON, and error symbols to delegate or alias isolated contracts in `exit.go`, `mode.go`, `context.go`, `json.go`, and `error.go`
- [X] T022 [US2] Convert root config symbols to delegate to isolated config APIs while preserving trace-aware error behavior in `config.go`
- [X] T023 [US2] Convert root schema symbols and `__schema` command wiring to delegate to isolated schema APIs in `schema.go`
- [X] T024 [US2] Convert root ID helpers to delegate to isolated ID APIs in `id.go`
- [X] T025 [US2] Verify and update root API examples and integration usage in `example_test.go`, `examples/integration/main.go`, and `examples/integration/main_test.go`
- [X] T026 [US2] Run the compatibility gate `go test -race . ./examples/integration/...` from repository root `.` and fix failures in root `*.go`, root `*_test.go`, and `examples/integration/`

**Checkpoint**: User Story 2 is complete when existing root tests, examples, and golden contracts pass without public deprecations or removals.

---

## Phase 5: User Story 3 - Maintainers Can Enforce Package Boundaries (Priority: P3)

**Goal**: Maintainers can understand and continuously enforce which package to import for thin contracts versus full runtime behavior.

**Independent Test**: Boundary checks fail on forbidden imports, and documentation clearly directs maintainers to isolated packages or root `ax` by use case.

### Tests for User Story 3

- [X] T027 [P] [US3] Add negative boundary-helper tests that prove forbidden imports are reported clearly in `internal/testutil/imports_test.go`
- [X] T028 [P] [US3] Add documentation snippet or example validation for isolated import examples in `README.md` and `examples/integration/README.md`

### Implementation for User Story 3

- [X] T029 [US3] Update public import guidance and package selection examples in `README.md`
- [X] T030 [P] [US3] Reconcile derived agent and context guidance with the new public package boundaries in `AGENTS.md`, `CONTEXT.md`, `GEMINI.md`, `CLAUDE.md`, and `ROADMAP.md`
- [X] T031 [US3] Ensure import-isolation failure messages and forbidden dependency lists stay documented with the helper and tests in `internal/testutil/imports.go` and `internal/testutil/imports_test.go`

**Checkpoint**: User Story 3 is complete when docs explain the import choice and boundary tests enforce the forbidden dependency set.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final formatting, full repository validation, and ADR retirement.

- [X] T032 [P] Run `gofmt` on Go files in `contract/`, `config/`, `schema/`, `id/`, `internal/testutil/`, root `*.go`, and `examples/integration/`
- [X] T033 [P] Update doc-coverage baseline if required by new verified examples in `internal/cmd/doccover/baseline.txt`
- [X] T034 [P] Validate quickstart instructions against implemented package names and commands in `specs/010-import-isolated-contracts/quickstart.md`
- [X] T035 Run the full race test gate `go test -race ./...` from repository root `.`
- [X] T036 Run static analysis `go vet ./...` from repository root `.`
- [X] T037 Run lint gate `golangci-lint run` from repository root `.`
- [X] T038 Run documentation coverage gate `make doc-coverage` from repository root `.`
- [X] T039 Confirm `CHANGELOG.md` was not manually edited and capture release notes through the eventual Conventional Commit message in `CHANGELOG.md`
- [X] T040 [FINAL] Retire absorbed ADRs only after all previous tasks pass: delete `docs/adr/0001-agent-mode-trigger.md`, `docs/adr/0002-error-envelope-schema.md`, `docs/adr/0003-schema-output-format.md`, `docs/adr/0007-id-strategy.md`, and `docs/adr/0012-directory-layout.md`; update references in `README.md`, `CONTEXT.md`, `AGENTS.md`, `GEMINI.md`, `CLAUDE.md`, `ROADMAP.md`, and relevant Go doc comments in root `*.go`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Phase 1 and blocks all user-story implementation.
- **User Story 1 (Phase 3)**: Depends on Phase 2 and is the MVP.
- **User Story 2 (Phase 4)**: Depends on User Story 1 contract packages existing; compatibility tests can be drafted after Phase 2.
- **User Story 3 (Phase 5)**: Depends on Phase 2 for helper work and on User Story 1 for final boundary/doc verification.
- **Polish (Phase 6)**: Depends on the desired user stories being complete.
- **ADR retirement**: T040 is last and depends on all tests, docs, and research absorption being complete.

### User Story Dependencies

- **US1 Thin Consumer Imports Contracts**: No dependency on other user stories after Phase 2.
- **US2 Existing Root Package Users Remain Compatible**: Depends on US1 implementation because root wrappers delegate to the new surfaces.
- **US3 Maintainers Can Enforce Package Boundaries**: Can draft documentation after Phase 2, but final verification depends on US1 and US2.

### Within Each User Story

- Write tests first and verify they fail for the expected reason.
- Implement the smallest package surface required to make those tests pass.
- Run the story checkpoint command before moving to the next story.

## Parallel Opportunities

- T002 can run after T001 and before T003.
- T004, T005, T006, T007, T008, and T009 can be drafted in parallel after T003.
- T012 can run in parallel with T010/T011 because `id/` is independent.
- T016, T017, T018, T019, and T020 can be drafted in parallel after User Story 1 tests clarify package behavior.
- T027 and T028 can run in parallel because they touch test helper behavior and documentation.
- T032, T033, and T034 can run in parallel once implementation tasks are complete.

## Parallel Example: User Story 1

```text
Task: "T004 Write failing contract package tests in contract/*.go test files"
Task: "T006 Write failing ID package tests in id/"
Task: "T007 Write failing config package tests in config/"
Task: "T008 Write failing schema package tests in schema/"
Task: "T009 Write failing import-isolation tests in contract/, config/, schema/, and id/"
```

## Parallel Example: User Story 2

```text
Task: "T016 Write root contract facade compatibility tests in contract_facade_test.go"
Task: "T017 Write root config facade compatibility tests in config_facade_test.go"
Task: "T018 Write root schema facade compatibility tests in schema_facade_test.go"
Task: "T019 Write root ID facade compatibility tests in id_facade_test.go"
```

## Parallel Example: User Story 3

```text
Task: "T027 Add negative boundary-helper tests in internal/testutil/imports_test.go"
Task: "T028 Add documentation snippet validation in README.md and examples/integration/README.md"
Task: "T030 Reconcile derived guidance in AGENTS.md, CONTEXT.md, GEMINI.md, CLAUDE.md, and ROADMAP.md"
```

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 and Phase 2.
2. Complete User Story 1 tests and implementation.
3. Run `go test -race ./contract ./config ./schema ./id`.
4. Stop and verify thin consumers can import the new packages without forbidden runtime adapters.

### Incremental Delivery

1. Deliver isolated packages for thin consumers (US1).
2. Preserve and verify root `ax` compatibility (US2).
3. Harden maintainer documentation and boundary enforcement (US3).
4. Run full repository gates and retire absorbed ADRs last.

### Notes

- Do not edit `CHANGELOG.md` manually.
- Do not add new dependencies unless a task discovers an unavoidable need and records the justification in the PR description.
- Do not remove or deprecate root `ax` symbols in this feature.
- Keep `stdout` and `stderr` contracts unchanged in all examples and golden fixtures.
