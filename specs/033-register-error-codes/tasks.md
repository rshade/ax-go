---

description: "Task list for registering every error code and guarding the registry"
---

# Tasks: Register every error code and guard the registry

**Input**: Design documents from `specs/033-register-error-codes/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/,
quickstart.md

**Tests**: REQUIRED. Constitution VII (test-first) is non-negotiable. Every
test task precedes the implementation task it covers and must be observed
failing for the right reason before that implementation starts.

**Organization**: by user story (spec.md). Phases run in dependency order,
not priority order: the drift guard (US1) needs the registry it enforces
(US2, US4) before it can run on this module. See Dependencies.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1 to US4 from spec.md
- All paths are repository-relative, inside the `ax-go-285` worktree

---

## Phase 1: Setup

- [ ] T001 Confirm the baseline is green before any change: `go test -race ./contract/... ./internal/testutil/... ./schema/... ./mcp/... ./internal/mcpserver/... ./internal/schema/... .` passes and `make surface-check` exits `0`. Record `go run ./examples/integration __schema --format=json | jq -c .error_envelope.known_codes` (the nine-code list) to compare against in T020.

---

## Phase 2: Foundational — the checker (blocks US1)

**Purpose**: the type-free syntactic checker and its proof that it can fail (spec FR-006 to FR-009, research R4, R5). It is mechanism only; the module policy arrives in US1.

- [ ] T002 [P] Create the fixture tree `internal/testutil/testdata/errorcodes/`. Every file is gofmt-clean Go that is parsed, never compiled. It holds:
  - `contract/codes.go` (package `contract`): `ErrorCodeKnown = "known_code"`, `ErrorCodeOrphan = "orphan_code"` (a constant whose value the test does not register), and a non-code constant `ModeLike = "json"`.
  - `contract/error.go`: `type Error struct{ ErrorCode string }` and `func NewError(ctx context.Context, code, message string) *Error { return &Error{ErrorCode: code} }` (exempted by a pass-through entry in the test).
  - `contract/inside.go`: an unqualified `NewError(ctx, ErrorCodeKnown, "")` (valid) and `NewError(ctx, "inside_literal", "")` (`call_code`).
  - `error.go` (root, package `ax`): `func NewError(ctx context.Context, code, message string) *contract.Error { return contract.NewError(ctx, code, message) }` (exempted pass-through) and `rootUnqualified()` calling `NewError(ctx, "root_unqualified", "")` (`call_code`).
  - `good/good.go`: valid sites only — `contract.NewError(ctx, contract.ErrorCodeKnown, "")`, an aliased import `c "github.com/rshade/ax-go/contract"` with `c.NewError(ctx, c.ErrorCodeKnown, "")`, `ax.NewError(ctx, contract.ErrorCodeKnown, "")`, `contract.Error{ErrorCode: contract.ErrorCodeKnown}`, and `e.ErrorCode = contract.ErrorCodeKnown`.
  - `bad/bad.go`: one of each violation — a string-literal code (`call_code`), a local constant code (`call_code`), `contract.ErrorCodeOrphan` (`call_code`), `contract.ModeLike` (`call_code`), a forwarded `code` parameter with no exemption (`call_code`), `ax.NewError(ctx, "root_literal", "")` (`call_code`), `contract.Error{ErrorCode: "field_literal"}` (`field_code`), `e.ErrorCode = "assign_literal"` (`assign_code`), and `f := contract.NewError` (`constructor_value`).
  - `dot/dot.go`: a dot import of `github.com/rshade/ax-go/contract` (`dot_import`).
  - `tagged/tagged_windows.go` with `//go:build ax_no_grpc` and a literal code (`call_code`): proves constrained and platform files are checked.
  - Excluded paths, each with a literal code that must NOT be reported: `bad/bad_test.go`, `testdata/nested.go`, `nested/go.mod` plus `nested/nested.go`, `.hidden/hidden.go`, `node_modules/pkg/pkg.go`, `examples/example.go`, `internal/cmd/tool/main.go`.
- [ ] T003 Write `internal/testutil/errorcodes_test.go`:
  - `TestFindErrorCodeViolationsFixture`: run `FindErrorCodeViolations` on the fixture with `ModulePath` `github.com/rshade/ax-go`, `RootPackageName` `ax`, `Registered` `{"known_code"}`, `ExcludedDirs` `examples` and `internal/cmd` (each with a reason), and `PassThroughs` for `error.go`/`NewError`/`code`, `contract/error.go`/`NewError`/`code`, and one entry that matches nothing (`stale_pass_through`). Assert the exact sorted list of `{File, Line, Kind, Expr}` (every kind in data-model.md appears at least once), and that `FormatErrorCodeViolations` renders `file:line: kind: expr`.
  - `TestFindErrorCodeViolationsRejectsInvalidGuard`: a table of invalid configurations (empty `ModuleDir`, `ModulePath`, `RootPackageName`, or `Registered`; an exclusion or pass-through with an empty reason; a `ModuleDir` with no `contract` directory) each returning a non-nil error.
  - `TestParseStringConstants`: on the fixture's `contract` directory it returns exactly the three exported string constants.
  It fails to compile now because the checker does not exist.
- [ ] T004 Implement `internal/testutil/errorcodes.go` per contracts/error-code-registry.md and research R4: the walk and its fixed exclusions, `ParseStringConstants`, import-name resolution, the three emit-site forms, `constructor_value`, `dot_import`, pass-through matching with `stale_pass_through`, configuration validation, deterministic sorting, and `FormatErrorCodeViolations`. Wrap every error with `%w`. Doc comments state the contract, the fail-closed meaning of an error, and the type-free limits. Run `go test -count=1 -race ./internal/testutil/` and confirm T003 passes.

**Checkpoint**: the checker reports every planted violation in the fixture and nothing from its excluded paths.

---

## Phase 3: User Story 2 — Adopters match every ax-go code by name (P1)

**Goal**: every code ax-go emits has exactly one exported `contract` constant (FR-001, FR-002, FR-012).

**Independent Test**: `go test -count=1 -run TestErrorCodeConstantSpellings ./contract/` passes, pinning each of the ten constants to its frozen spelling.

- [ ] T005 [US2] Add `TestErrorCodeConstantSpellings` to `contract/codes_test.go`: a table mapping each of the ten `ErrorCode*` constants (data-model.md) to its string literal, asserting equality. The literals stay literals; they pin the frozen spellings (FR-006 of spec 032, FR-011 here). It fails to compile now because four constants do not exist.
- [ ] T006 [US2] In `contract/codes.go`, add `ErrorCodeConfirmationRequired`, `ErrorCodeInternalError`, `ErrorCodeValidationError`, and `ErrorCodeInvalidSchemaDeclaration` (untyped `string`) with contract-style doc comments stating the condition and exit code. Replace the three remaining literals in `KnownErrorCodes()` with the constants, keeping byte-wise order. Run `go test -count=1 -race ./contract/` and confirm T005, `TestKnownErrorCodesProperties`, and `ExampleKnownErrorCodes` pass.

---

## Phase 4: User Story 4 — The registry and its exclusion are self-explaining (P2)

**Goal**: a separate, commented, unexported authoring-time list, and a consistency test that keeps constants and lists in step (FR-003, FR-010).

**Independent Test**: `go test -count=1 -run TestErrorCodeRegistryConsistency ./contract/` passes; adding an `ErrorCode*` constant to neither list makes it fail.

- [ ] T007 [US4] Add `TestErrorCodeRegistryConsistency` to `contract/codes_test.go` (research R6): parse the `contract` directory with `testutil.ParseStringConstants`, take the `ErrorCode*` names, and assert each value is in exactly one of `KnownErrorCodes()` and `authoringTimeErrorCodes()`, each list entry is the value of exactly one such constant, the lists are disjoint, and the authoring-time list is sorted and unique. It fails to compile now because `authoringTimeErrorCodes` does not exist.
- [ ] T008 [US4] In `contract/codes.go`, add the unexported `authoringTimeErrorCodes() []string` returning a new slice holding `ErrorCodeInvalidSchemaDeclaration`, with a comment stating spec 032 FR-001's reason (returned while the command tree is built, before `Execute` dispatches a command, so a run never returns it). Extend the `KnownErrorCodes` doc comment to say the list names only registry constants. Run `go test -count=1 -race ./contract/` and confirm T007 passes. Temporarily add an unlisted `ErrorCode*` constant, confirm T007 fails naming it, then remove it.

---

## Phase 5: User Story 1 — A new error path cannot skip the registry (P1) 🎯 MVP

**Goal**: the drift guard runs over this module in the normal test suite and every emit site names a registered constant (FR-004 to FR-008, SC-001).

**Independent Test**: `go test -count=1 -run TestErrorCodesAreRegistered ./contract/` passes with zero violations, and the fixture test from T003 still reports every planted violation.

- [ ] T009 [US1] Create `contract/codes_guard_test.go` (package `contract`) with `TestErrorCodesAreRegistered`: call `testutil.FindErrorCodeViolations` with `ModuleDir` `testutil.RepoRoot(t)`, `ModulePath` `github.com/rshade/ax-go`, `RootPackageName` `ax`, `Registered` = `KnownErrorCodes()` plus `authoringTimeErrorCodes()`, `ExcludedDirs` `internal/cmd` and `examples` with the reasons in research R5, and exactly the two `PassThroughs` from research R4 (root `error.go`/`NewError`/`code`; `contract/error.go`/`NewError`/`code`), each with its reason. Fail with `FormatErrorCodeViolations` output. Run it with `-count=1` and confirm it fails listing exactly the 21 sites in research R1 (19 literals, `execute.go:232` `contractErrorWarnings`, `schema/declarations.go:137` `invalidDeclarationCode`) and nothing else. Record any extra site as a finding before continuing.
- [ ] T010 [P] [US1] Root package: in `execute.go`, replace `contractErrorWarnings` (both the `span.SetStatus` use and the `NewError` call) with `contract.ErrorCodeWarningsAsErrors`, delete the `contractErrorWarnings` declaration and its comment, and replace `"validation_error"` and `"internal_error"` with `contract.ErrorCodeValidationError` and `contract.ErrorCodeInternalError`. In `confirm.go`, use `contract.ErrorCodeConfirmationRequired`. Add the `contract` import where missing.
- [ ] T011 [P] [US1] In `contract/error.go`, make the `WriteError` non-envelope fallback use `ErrorCodeInternalError`.
- [ ] T012 [P] [US1] In `internal/mcpserver/dispatch.go`, `server.go`, and `transport.go`, replace every `"internal_error"` and `"validation_error"` code argument with `contract.ErrorCodeInternalError` / `contract.ErrorCodeValidationError`, and build the marshal-failure fallback in `dispatch.go` by concatenating `contract.ErrorCodeInternalError` into the same JSON bytes (byte-identical result).
- [ ] T013 [P] [US1] In `internal/schema/declare.go` and `internal/schema/conflict_error.go`, use `contract.ErrorCodeValidationError`.
- [ ] T014 [P] [US1] In `mcp/command.go`, use `contract.ErrorCodeValidationError`.
- [ ] T015 [P] [US1] In `schema/schema.go`, use `contract.ErrorCodeValidationError`; in `schema/declarations.go`, delete `invalidDeclarationCode` and use `contract.ErrorCodeInvalidSchemaDeclaration`.
- [ ] T016 [US1] Run `go test -count=1 -race ./contract/ ./internal/testutil/` and confirm `TestErrorCodesAreRegistered` reports zero violations and the fixture test is unchanged. Confirm with `grep -rn 'NewError(' --include=*.go . | grep -v _test.go | grep -v internal/cmd/ | grep -v examples/` that no remaining call passes a literal.

---

## Phase 6: User Story 3 — The agent-facing contract does not move (P1)

**Goal**: no machine payload, spelling, exit code, or golden changes (FR-011, SC-003).

**Independent Test**: the full suite passes with no file under any `testdata/` changed and `ExampleBuildSchema` untouched.

- [ ] T017 [US3] Run `go test -count=1 -race ./...` and confirm every golden, envelope, exit-code, and `__schema` test passes. Confirm `git diff --stat -- '*.golden.json' example_test.go schema/example_test.go` is empty.
- [ ] T018 [US3] Re-run the T001 `known_codes` command and confirm the output is byte-identical to the recorded one.
- [ ] T019 [US3] Run `make surface-update`, then confirm `git diff internal/cmd/surfacecheck/baseline.json` adds exactly four `github.com/rshade/ax-go/contract` `const:` features (`ErrorCodeConfirmationRequired`, `ErrorCodeInternalError`, `ErrorCodeInvalidSchemaDeclaration`, `ErrorCodeValidationError`) with signature `untyped string`, present in `all` configurations and profiles, and nothing else. Confirm `specs/023-internalize-helpers/public-surface-audit.json` is unchanged. Run `make surface-check`.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T020 [P] In `README.md` → `### Standard ax.Error envelope`, add a short paragraph: every `error_code` ax-go itself returns has a `contract.ErrorCode*` constant; `__schema` lists the runtime ones under `error_envelope.known_codes`; `invalid_schema_declaration` is returned while the command tree is built and is not listed.
- [ ] T021 [P] In `AGENTS.md`, add a short `### Error Code Registry` subsection under Development Workflow: codes live in `contract/codes.go`; a new runtime code is a constant plus a `KnownErrorCodes()` entry (authoring-time codes go in `authoringTimeErrorCodes()`); emit sites name the constant; `TestErrorCodesAreRegistered` and `TestErrorCodeRegistryConsistency` enforce it; `internal/cmd/` and `examples/` are excluded and why; update the `known_codes` goldens and `ExampleBuildSchema` together. Point at `specs/033-register-error-codes/quickstart.md`.
- [ ] T022 Run every gate from AGENTS.md: `gofmt -l .` (empty), `go vet ./...`, `make validate`, `make test`, `$(mise which golangci-lint) run` once per tag set (default, `ax_no_grpc`, `ax_no_otlp`, both), `make doc-coverage`, `make cover-check`, `make surface-check`, `make size-check`, `make dead-check`, `make slop-check`, `make security`, and `mise exec -- markdownlint` on every changed Markdown file. All must pass.
- [ ] T023 Write `PR_MESSAGE.md` at the worktree root as a `feat(contract):` Conventional Commit ending with `Implements specs/033-register-error-codes/.` and `Closes #285`, and validate it with `cat PR_MESSAGE.md | npx --yes commitlint`.

---

## Dependencies & Execution Order

- **Setup (T001)** → **Foundational (T002–T004)**.
- **US2 (T005–T006)** depends only on Setup; it can run alongside Foundational.
- **US4 (T007–T008)** depends on T004 (`ParseStringConstants`) and T006 (the constants it checks).
- **US1 (T009–T016)** depends on T004 (checker), T006, and T008 (the registered set). T009 must be observed failing before T010–T015. T010–T015 touch disjoint files and run in parallel. T016 follows them.
- **US3 (T017–T019)** depends on US1 completing, because the conversions are what could move a payload.
- **Polish (T020–T023)** last; T020 and T021 are parallel.

US1 has the highest value but runs fifth because it enforces the registry the earlier phases build. Each phase still ends in a passing, independently testable state.

## Parallel Example: User Story 1

```text
After T009 is seen failing:
  T010 execute.go, confirm.go
  T011 contract/error.go
  T012 internal/mcpserver/{dispatch,server,transport}.go
  T013 internal/schema/{declare,conflict_error}.go
  T014 mcp/command.go
  T015 schema/{schema,declarations}.go
```

## Implementation Strategy

1. Build and prove the checker against the fixture (Phase 2).
2. Publish the constants and the two lists (Phases 3–4).
3. Turn the guard on for the module, watch it fail on 21 sites, convert them (Phase 5). This is the MVP: from here on a new literal fails CI.
4. Prove nothing moved and record the surface delta (Phase 6), then document and run every gate (Phase 7).
