---

description: "Task list for listing the config package's runtime codes in known_codes"
---

# Tasks: Known codes include the config package's runtime codes

**Input**: Design documents from `specs/032-known-config-codes/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/,
quickstart.md

**Tests**: REQUIRED. Constitution VII (test-first) is non-negotiable. Every
test task precedes the implementation task it covers and must be observed
failing for the right reason before that implementation starts.

**Organization**: by user story (spec.md), so each story is an independently
testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1 to US3 from spec.md
- All paths are repository-relative, inside the `../ax-go-284` worktree

---

## Phase 1: Setup

- [X] T001 Confirm the baseline is green before any change: `go test -race ./contract/... ./config/... ./schema/... ./examples/integration/... .` passes and `make surface-check` exits `0`. Record that `go run ./examples/integration __schema --format=json | jq -c .error_envelope.known_codes` prints the four-code list (the reproduction in quickstart.md).

---

## Phase 2: Foundational (blocks every story)

- [X] T002 Create `contract/codes.go` and move `ErrorCodeWarningsAsErrors` and `KnownErrorCodes` into it unchanged from `contract/warnings.go`, then remove them from `contract/warnings.go` and drop that file's now-unused imports if any. This is a pure move within package `contract` (research R3). `go build ./...` and `make surface-check` must both still pass with no baseline change, which proves the move is surface-neutral.

**Checkpoint**: `contract/codes.go` is the single home for code constants and the list.

---

## Phase 3: User Story 1 — Plan recovery for config failures before running (P1) 🎯 MVP

**Goal**: `__schema` `error_envelope.known_codes` lists the nine codes, sorted (FR-001 to FR-004, FR-010).

**Independent Test**: `go run ./examples/integration __schema --format=json | jq -c .error_envelope.known_codes` prints the nine codes in the order given in spec US1 scenario 1.

### Tests for User Story 1 (write first, watch them fail)

- [X] T003 [P] [US1] Add `ExampleKnownErrorCodes` to `contract/example_test.go`, printing `strings.Join(contract.KnownErrorCodes(), "\n")` with an `// Output:` block holding exactly the nine codes in byte-wise order: `config_invalid`, `config_max_bytes_invalid`, `config_option_invalid`, `config_patch_invalid`, `config_too_large`, `confirmation_required`, `internal_error`, `validation_error`, `warnings_as_errors`. This is the single exact-list assertion (research R6). It fails now because only four codes print.
- [X] T004 [P] [US1] Create `contract/codes_test.go` with `TestKnownErrorCodesProperties` checking that the result is sorted (`slices.IsSorted`), has no duplicates, does not contain `invalid_schema_declaration` (FR-004), and is a fresh slice on each call: overwrite element 0 of one result, call again, and confirm the second result is unchanged (FR-010). These properties hold today. They guard the list as it grows, and none restates the exact list.
- [X] T005 [P] [US1] In `config/config_test.go`, extend the shared `assertContractError` helper so it also fails when `wantCode` is not in `contract.KnownErrorCodes()` (`slices.Contains`). Keep every `wantCode` literal as a string, because the literals pin the frozen spellings (FR-006). All seven call sites (`TestParseClassifiesValidationErrors` ×3, `TestParseRejectsNilOption`, `TestParseFileHonorsOptions`, `TestPatchClassifiesErrors` ×2) now fail on membership. They cover the six distinct failure paths of SC-002, with `config_too_large` reached twice: once through `Parse` and once through `ParseFile`.
- [X] T006 [P] [US1] Add `TestRunConfigErrorCodesAreKnown` to `examples/integration/main_test.go`. It runs the integration CLI's `__schema` through `run(...)`, decodes `error_envelope.known_codes`, then triggers two real failures through `run(...)`: an oversized `--config=-` (reuse the `TestRunRejectsOversizedHujsonConfigFromStdin` input, giving `config_too_large`) and a `patch-config --dry-run` with a patch removing `/nonexistent` (reuse the existing patch test setup, giving `config_patch_invalid`). For each, it asserts exit `2`, empty stdout, and that the stderr envelope's `error_code` is in `known_codes` (spec SC-002, end to end). Table-driven over the two cases. It fails now because neither code is listed.

### Implementation for User Story 1

- [X] T007 [US1] In `contract/codes.go`, add the five exported untyped string constants `ErrorCodeConfigInvalid`, `ErrorCodeConfigMaxBytesInvalid`, `ErrorCodeConfigOptionInvalid`, `ErrorCodeConfigPatchInvalid`, and `ErrorCodeConfigTooLarge`, with values exactly as in `specs/032-known-config-codes/contracts/known-codes.md`. Each carries a contract-style doc comment naming the condition that produces it and exit `2`. Extend `KnownErrorCodes()` to return the nine codes in byte-wise order, built from the constants plus the three remaining literals (those convert in #285). T003, T004, and T005 now pass.
- [X] T008 [P] [US1] Update the `known_codes` array in `testdata/schema_ax.golden.json`, `testdata/schema_ax_declarations.golden.json`, `testdata/schema_ax_enriched.golden.json`, and `examples/integration/testdata/schema_ax.golden.json` to the nine-code array in `contracts/known-codes.md`. Also update the `// Output:` block of `ExampleBuildSchema` in `example_test.go`, which embeds the full `__schema` payload; the plan's `*.json`-only survey missed it. Change nothing else in those files (research R5). Verify with `go test -race . ./schema/... ./examples/integration/...`. Then `git diff --stat` must show exactly one changed line per golden.
- [X] T009 [US1] Run `go test -race ./contract/... ./config/... ./schema/... ./examples/integration/... .` and confirm T003 to T006 pass.

**Checkpoint**: an agent reading `__schema` sees all nine codes. US1 is shippable alone.

---

## Phase 4: User Story 2 — Existing code matchers keep working (P1)

**Goal**: the config package emits through the constants, and every spelling and exit code stays the same (FR-005, FR-006).

**Independent Test**: the six existing config failure tests, which still compare against string literals, pass unchanged after the swap.

- [X] T010 [US2] In `config/config.go`, replace the six `"config_*"` string literals in `applyOptions`, `normalizeReadError`, `normalizeDecodeError`, and `normalizePatchError` with the matching `contract.ErrorCodeConfig*` constants. Change no message, fix, context, or exit code. Then `grep -n '"config_' config/config.go` must print nothing.
- [X] T011 [US2] Run `go test -race ./config/... . ./examples/integration/...`. The literal-pinned tests from T005 and the root package's config tests (`config_test.go`, `config_facade_test.go`) pass without edits, which proves FR-006.

**Checkpoint**: the codes have one source of truth, and the spellings are unchanged.

---

## Phase 5: User Story 3 — The field states what it covers (P2)

**Goal**: the scope rule (FR-001) is stated where readers look (FR-007 to FR-009).

**Independent Test**: `go doc github.com/rshade/ax-go/schema.ErrorSchemaInfo` and `go doc github.com/rshade/ax-go/contract.KnownErrorCodes` both state the inclusion rule and the three exclusions.

- [X] T012 [P] [US3] In `schema/schema.go`, add a doc comment on the `KnownCodes` field of `ErrorSchemaInfo` stating the FR-001 rule. It lists every `error_code` ax-go library code can return from a command run (dispatch through `ax.Execute` or an MCP tool call), including public helper packages such as `config`. It excludes adopter-defined codes, authoring-time codes such as `invalid_schema_declaration`, and repository gate-tool codes. It is sorted and its source is `contract.KnownErrorCodes`.
- [X] T013 [P] [US3] In `contract/codes.go`, rewrite the `KnownErrorCodes` doc comment to state the same rule in contract form (sorted, fresh slice per call, the three exclusions), replacing "the runtime's own error_code values".
- [X] T014 [P] [US3] In `specs/029-success-warnings/spec.md`, amend the Assumptions bullet that defines known codes to the FR-001 rule, and add a one-line pointer: "Amended by specs/032-known-config-codes (FR-001)." (FR-008)
- [X] T015 [P] [US3] In `README.md`, under "Asymmetric JSON flow", extend the config error-code paragraph to name `config_patch_invalid` (an invalid RFC 6902 patch or a failed patch operation from `ax.PatchConfig`), and add that `__schema` lists every config code under `error_envelope.known_codes` and `contract` exports a constant for each (FR-009). Run `mise exec -- markdownlint README.md specs/029-success-warnings/spec.md`.

**Checkpoint**: the rule #285 will enforce is written down in the code, the specs, and the README.

---

## Phase 6: Polish & cross-cutting

- [X] T016 Run `make surface-update`, then `git diff internal/cmd/surfacecheck/baseline.json`. The diff must be exactly five `added` `const:ErrorCodeConfig*` entries (signature `untyped string`, `all`/`all`) under `github.com/rshade/ax-go/contract`, in sorted position. Confirm `specs/023-internalize-helpers/public-surface-audit.json` is untouched (research R4). Then `make surface-check` exits `0`.
- [X] T017 Run `gofmt -l .` (expect no output), `make validate`, and `make test` (race across the 4-tag matrix).
- [X] T018 Run `golangci-lint run` through `$(mise which golangci-lint)` for each tag set in `BUILD_TAG_MATRIX` (or `make lint`, falling back to direct golangci-lint if the actionlint step hangs), and `make doc-coverage`.
- [X] T019 Run `make cover-check`, `make size-check`, `make dead-check`, `make slop-check`, and `make security`.
- [X] T020 Walk `specs/032-known-config-codes/quickstart.md` end to end against the built integration CLI and confirm each printed value matches.
- [X] T021 In `PR_MESSAGE.md`, state that issue #284's acceptance criterion "surface baseline, and audit are updated" is met for the baseline only. `specs/023-internalize-helpers/public-surface-audit.json` is scoped to the root package, the new constants live in `contract`, and they are deliberately not recorded there (research R4).

---

## Dependencies & execution order

- T001 → T002 → (US1, US2, US3).
- US1: T003, T004, T005, and T006 are written in parallel and observed failing → T007 → T008 → T009.
- US2: T010 depends on T007, because the constants must exist. T011 follows T010.
- US3: T013 depends on T007 (same file). T012, T014, and T015 are independent of US1 and US2.
- Polish: T016 depends on T007. T017 to T021 run after all stories.

## Parallel opportunities

- T003, T004, T005, and T006 touch four different files.
- T008 (goldens) can run alongside T010 once T007 lands.
- T012, T014, and T015 are documentation-only and can run alongside anything.

## Implementation strategy

MVP is Phase 3 (US1). Once T009 passes, `__schema` is correct for agents. US2
removes the duplicated literals that would let the list and the emitter drift
apart, and US3 records the rule that #285 enforces. All three ship in one PR
because the issue's acceptance criteria require all of them.

## Format validation

All 21 tasks use `- [ ] T### [P?] [US?] description` with an explicit file
path or command. Story labels appear only in Phases 3 to 5.
