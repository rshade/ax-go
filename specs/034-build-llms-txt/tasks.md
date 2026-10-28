# Tasks: Build-time static discovery document

**Input**: Design documents from `/specs/034-build-llms-txt/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/llms-txt.md

**Tests**: Required. The spec's acceptance scenarios are golden and table tests, and the constitution requires tests before the renderer.

**Organization**: Stories share `llmstxt.go`. Tests for a story land before the code that makes them pass. No governing ADR, so there is no ADR-retirement task.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: User Story 1 - Generated document (Priority: P1)

**Goal**: A fixed tree and a fixed opening produce one stable document.

**Independent Test**: `go test -run TestGenerateLLMsTxt -count=1`

- [x] T001 [US1] Add a failing golden test in `llmstxt_test.go` and `testdata/llms.golden.txt` for a nested tree, unsorted registration, inherited flags, and curated links.
- [x] T002 [US1] Add `ExampleGenerateLLMsTxt` in `example_test.go` with a checked `// Output:` block.
- [x] T003 [US1] Implement `GenerateLLMsTxt`, `WithPreamble`, `WithLinks`, `Link`, and `LinkSection` in `llmstxt.go` so T001 and T002 pass. Sort names, skip hidden commands, collapse newlines in reflected text.

**Checkpoint**: Identical inputs match the golden file.

## Phase 2: User Story 2 - Validation (Priority: P1)

**Goal**: A bad opening writes nothing and returns `validation_error`.

**Independent Test**: `go test -run TestGenerateLLMsTxtValidation -count=1`

- [x] T004 [US2] Add failing table tests in `llmstxt_test.go` for a missing opening, a title-less opening, a missing file, a file of exactly 1 MiB, a file of 1 MiB plus one byte, a nil destination, a nil command, a cancelled context, and an empty link section.
- [x] T005 [US2] Implement `WithPreambleFile` and the validation paths in `llmstxt.go`. Use `contract.ErrorCodeValidationError`. Write zero bytes on every validation failure.

**Checkpoint**: Each validation case reads back an empty buffer.

## Phase 3: User Story 3 - Runtime discovery unchanged (Priority: P1)

**Goal**: `__schema` goldens stay byte-identical and no runtime adapter exists.

**Independent Test**: existing schema golden tests, plus `go test -run TestGenerateLLMsTxtDoesNotTouchSchema -count=1`

- [x] T006 [US3] Add a test that a command tree which can generate a document still emits the same `__schema` payload before and after generation, that the generator registers no `__llms` command, and that the document is not an `llms-full.txt` body (FR-007, FR-014).
- [x] T007 [US3] Confirm `schema.go` gains no `--as=llms-txt` branch. The existing golden tests are the gate.

**Checkpoint**: Schema goldens pass without regeneration.

## Phase 4: User Story 4 - Integration example (Priority: P2)

**Goal**: The example's committed document matches a fresh generation.

**Independent Test**: `go test ./examples/integration -run TestLLMsTxt -count=1`

- [x] T008 [US4] Add `examples/integration/llms.preamble.md`, a failing `examples/integration/llms_test.go`, and the committed `examples/integration/llms.txt`.
- [x] T009 [P] [US4] Document the generate-and-commit workflow in `README.md` and `examples/integration/README.md`.

**Checkpoint**: The example test passes and tells an adopter that publication is theirs.

## Phase 5: Polish

- [x] T010 Add `ax.GenerateLLMsTxt` to `requiredSymbols` in `internal/cmd/doccover/main.go` and the matching `baseline.txt` entry.
- [x] T011 Regenerate `internal/cmd/surfacecheck/baseline.json` and append `supported` audit rows for the new root exports in `specs/023-internalize-helpers/public-surface-audit.json`.
- [x] T012 Run `gofmt` and the repository gates named in `AGENTS.md` for this change.

## Dependencies

T003 depends on T001 and T002. T005 depends on T004. T008 depends on T003 and T005. T010 and T011 depend on T003. T006 can be written with T001.

## Parallel opportunities

T009 can be written while T008's test is failing. T002 can be written beside T001.

## Implementation strategy

The MVP is T001 through T003. Validation, the schema non-change, and the example consumer follow in that order. Surface and doc-coverage gates are last because they record the exports T003 adds.
