---

description: "Task list for feature 028: MCP prompts and static resources in the schema contract"
---

# Tasks: MCP Prompts and Static Resources in the Schema Contract

**Input**: Design documents from `specs/028-mcp-prompts-resources/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/schema-prompts-resources.md, quickstart.md

**Tests**: REQUIRED. Constitution Principle VII (test-first, non-negotiable) and AGENTS.md "Testing-First Discipline". Every test task lands, and fails for the right reason, before its implementation task.

**Organization**: Tasks are grouped by user story. US1 (prompts) and US4 (no-declaration byte identity) are P1. US2 (resources) and US3 (static guarantee) are P2.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US4 from spec.md
- All paths are repo-relative to the worktree root

## Phase 1: Setup

- [X] T001 Confirm the baseline is green before changes: run `go test -race ./schema/... ./internal/schema/... ./internal/mcp/... ./internal/mcpserver/... ./examples/integration/...` and `make surface-check` from the worktree root, and record any pre-existing failure before proceeding

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: Shared internal mechanics that both prompts and resources ride on (research R1, R3, R4, R6).

- [X] T002 Write failing tests in `internal/schema/declarations_test.go` for `WalkDeclarationCommands`: pre-order visit order (command, then children in Cobra order), a hidden command prunes its subtree, and reserved commands (`__schema`, `mcp-server`, `completion`, `help`) are **visited**, matching `BuildCommand`'s hidden-only pruning (research R6)
- [X] T003 Write failing table tests in `internal/schema/declarations_test.go` for the shared validators: `validName` (MCP charset `^[a-zA-Z0-9_.-]+$`, empty rejected), UTF-8 validation of free-text fields, and the `{{name}}` placeholder scanner (exact form only, `{{ name }}` literal, unterminated `{{` literal, multiple placeholders, placeholder at start and end)
- [X] T004 Create `internal/schema/declarations.go` with `const` annotation keys `github.com/rshade/ax-go/schema/prompts` and `github.com/rshade/ax-go/schema/resources`; `Violation{Field, Reason string}` with the reason constants from contracts/schema-prompts-resources.md (`nil_command`, `required`, `invalid_charset`, `invalid_utf8`, `duplicate`, `undeclared_placeholder`, `not_absolute`, `too_long`, `invalid_character`); `WalkDeclarationCommands(root *cobra.Command, visit func(*cobra.Command))`; `validName`; and a hand-written placeholder scanner (no package-level `*regexp.Regexp`). Make T002–T003 pass. The walk prunes hidden commands only, the same rule as `BuildCommand`, with no reserved-name list

**Checkpoint**: The shared walk and validators are green. The story phases can start.

---

## Phase 3: User Story 1 - Adopter declares a workflow prompt (Priority: P1) 🎯 MVP

**Goal**: `DeclarePrompt` validates and stores prompts, and both `__schema` formats project them (FR-001, FR-003, FR-004, FR-004a, FR-005–FR-007, FR-010).

**Independent Test**: Prompts declared on a non-runnable group and a leaf appear on their declaring nodes in `__schema` and in top-level `prompts` of `__schema --as=mcp`, in walk then declaration order, golden-pinned. Invalid prompts are rejected with `invalid_schema_declaration`.

### Tests for User Story 1

- [X] T005 [P] [US1] Write failing table tests in `internal/schema/declarations_test.go` for `ValidatePrompt` and `AddPrompt`: one case per reason (nil command, empty name, bad name charset, invalid UTF-8 in title/description/template, empty template, duplicate argument name, bad argument name, undeclared placeholder, same-command duplicate prompt name); success appends in declaration order; existing unrelated annotations are preserved; on failure the annotation map is unchanged and a nil map stays nil
- [X] T006 [P] [US1] Write failing tests in `internal/schema/declarations_test.go` for `Prompts(annotations)`: absent key → nil; undecodable JSON → nil (fail closed); a decoded entry that fails re-validation is dropped while valid siblings remain
- [X] T007 [P] [US1] Write failing tests in `internal/mcp/mcp_test.go` (or the existing `internal/mcp` test file that covers `Build`) asserting `Build(root).Prompts` aggregates over the declaration walk: includes prompts on a non-runnable group and on an `mcp.Exclude`-annotated leaf, excludes a hidden subtree, is ordered by walk then declaration order, and keeps the first of a cross-command duplicate name
- [X] T008 [P] [US1] Write failing tests in `schema/declarations_test.go`: `DeclarePrompt` failure returns an error where `errors.As` yields `*contract.Error` with `ErrorCode == "invalid_schema_declaration"`, exit code `2`, and `Context` holding `field` and `reason`; a success case returns nil
- [X] T009 [US1] Write failing golden tests in `schema/declarations_test.go` over a fixture tree (prompts on a runnable root, a non-runnable group, an `mcp.Exclude`d leaf, and a hidden subtree) that run `NewSchemaCommand` for `--as=ax` and `--as=mcp`, comparing against `testdata/schema_ax_declarations.golden.json` and `testdata/schema_mcp_declarations.golden.json`. Create the goldens from reviewed output only after T012–T015 land, and check by inspection that the hidden subtree's prompts are absent and the `template` field is present in both formats
- [X] T010 [US1] Write a failing test in `schema/declarations_test.go`: when two commands declare the same prompt name, `__schema` and `__schema --as=mcp` write nothing to stdout and return a `*contract.Error` with code `validation_error`, exit `2`, `context` `{kind:"prompt", key, commands:[path1,path2]}` and the actionable fix; the envelope is pinned in `testdata/schema_duplicate_declaration.golden.json` with `trace_id` masked via `internal/testutil.MaskNonDeterministic`. Also assert `BuildSchema` and `BuildMCPSchema` keep the first declaration in walk order

### Implementation for User Story 1

- [X] T011 [US1] Add the internal `Prompt` and `PromptArgument` types (ax-native JSON tags per data-model.md), plus `ValidatePrompt`, `AddPrompt`, and `Prompts` in `internal/schema/declarations.go` (makes T005–T006 pass)
- [X] T012 [US1] Add `FindDuplicate(root *cobra.Command) *Conflict` (`Conflict{Kind, Key string; Commands []string}` using `CommandPath()`) in `internal/schema/declarations.go`, checking prompts first, over `WalkDeclarationCommands`. Add its unit tests to `internal/schema/declarations_test.go`: none, duplicate prompt, duplicate inside a hidden subtree ignored, a same-command duplicate from a hand-written annotation reported with that path twice, and the same key used as a prompt name and a resource name allowed
- [X] T013 [US1] Extend `internal/mcp.Schema` with `Prompts []internalschema.Prompt` and make `Build` aggregate prompts with first-wins dedup in `internal/mcp/mcp.go` (makes T007 pass)
- [X] T014 [US1] In `schema/schema.go`, add the public `Prompt`, `PromptArgument`, `MCPPrompt`, and `MCPPromptArgument` types with doc comments written as contracts; add `CommandSchema.Prompts` (`json:"prompts,omitempty"`, placed after `Commands`) and `MCPSchema.Prompts` (`json:"prompts,omitempty"`, after `Tools`); add `DeclarePrompt(cmd *cobra.Command, prompt Prompt) error` that maps a `Violation` to `contract.NewError(context.Background(), "invalid_schema_declaration", ...)` with `WithErrorExitCode(contract.ExitValidation)` and `WithErrorContext` (makes T008 pass)
- [X] T015 [US1] In `schema/schema.go`, thread a first-wins `seen` set through `convertCommandSchema` to fill `CommandSchema.Prompts`; map `internal/mcp` prompts into `MCPSchema.Prompts` in `BuildMCPSchema`; in `NewSchemaCommand`'s `RunE`, call `internalschema.FindDuplicate(root)` before either format and return the `validation_error` envelope from research R4 (makes T009–T010 pass; generate the T009/T010 goldens now and review them line by line)
- [X] T016 [US1] Add root re-exports in `schema.go`: aliases `Prompt`, `PromptArgument`, `MCPPrompt`, `MCPPromptArgument` and the wrapper `DeclarePrompt`, with doc comments mirroring the `schema` package
- [X] T017 [P] [US1] Add `ExampleDeclarePrompt` with a verified `// Output:` (minified `__schema --as=mcp` prompts excerpt) in `schema/example_test.go`

**Checkpoint**: Prompts work end to end in both formats. This is the MVP.

---

## Phase 4: User Story 4 - Trees that declare nothing are unchanged (Priority: P1)

**Goal**: FR-009 and FR-011. Byte identity for existing trees, and an untouched live server.

**Independent Test**: The existing goldens pass with no regeneration, and the live server's capabilities have no prompts or resources key.

- [X] T018 [P] [US4] Verify without regeneration that `schema_test.go`, `schema/schema_test.go`, and `buildtags_parity_test.go` still pass against the unchanged `testdata/schema_ax.golden.json` and `testdata/schema_mcp.golden.json`, and that `git diff --stat testdata/schema_ax.golden.json testdata/schema_mcp.golden.json testdata/mcp_tools_list.golden.json` is empty
- [X] T019 [P] [US4] Add a test in `internal/mcpserver/server_test.go` that builds the server from the existing `mcp_tools_list` golden fixture tree plus prompt and resource declarations, and asserts that the `initialize` result's capabilities have neither `prompts` nor `resources` and that `tools/list` still matches `testdata/mcp_tools_list.golden.json` (FR-011, SC-006). The resource half depends on T025 (`DeclareResource`); land the prompt half first if US2 is not done

**Checkpoint**: Additivity is proven.

---

## Phase 5: User Story 2 - Adopter declares a static, read-only resource (Priority: P2)

**Goal**: `DeclareResource` with metadata only, projected in both formats (FR-002–FR-007).

**Independent Test**: A resource on the root appears with uri, name, title, description, and MIME type (snake_case `mime_type` ax-native, camelCase `mimeType` in MCP), with no content field.

### Tests for User Story 2

- [X] T020 [P] [US2] Write failing table tests in `internal/schema/declarations_test.go` for `ValidateResource`, `AddResource`, and `Resources(annotations)`: nil command, empty URI, URI > 2048 bytes (`too_long`), URI with a space or control character (`invalid_character`), relative URI with no scheme (`not_absolute`), unparseable URI, empty name, invalid UTF-8, control character in MIME type, same-command duplicate URI; success and preservation cases; fail-closed decoding
- [X] T021 [P] [US2] Write failing tests in the `internal/mcp` test file covering `Build(root).Resources` aggregation order, hidden pruning, and first-wins dedup by URI
- [X] T022 [P] [US2] Extend `schema/declarations_test.go`: `DeclareResource` error shape (`invalid_schema_declaration`, exit `2`, field and reason); add resources to the T009 fixture tree, and add a duplicate-URI case to the T010 duplicate test (`kind:"resource"`), including that `FindDuplicate` reports prompts before resources when a tree has both

### Implementation for User Story 2

- [X] T023 [US2] Add the internal `Resource` type, `ValidateResource` (using `net/url.Parse` and `unicode/utf8`), `AddResource`, and `Resources` in `internal/schema/declarations.go`, and extend `FindDuplicate` to check resources after prompts (makes T020 and the internal half of T022 pass)
- [X] T024 [US2] Extend `internal/mcp.Schema` with `Resources []internalschema.Resource`, aggregated first-wins in `Build` in `internal/mcp/mcp.go` (makes T021 pass)
- [X] T025 [US2] In `schema/schema.go`, add the public `Resource` (JSON `mime_type`) and `MCPResource` (JSON `mimeType`) types, `CommandSchema.Resources` (`resources,omitempty`, after `Prompts`), `MCPSchema.Resources` (`resources,omitempty`, after `Prompts`), and `DeclareResource`, and project them in `convertCommandSchema` and `BuildMCPSchema` (makes T022 pass); regenerate and review the declaration goldens and the duplicate golden
- [X] T026 [US2] Add root re-exports in `schema.go`: aliases `Resource` and `MCPResource` and the wrapper `DeclareResource`
- [X] T027 [P] [US2] Add `ExampleDeclareResource` with a verified `// Output:` in `schema/example_test.go`

**Checkpoint**: Resources work end to end.

---

## Phase 6: User Story 3 - Resources are provably static and read-only (Priority: P2)

**Goal**: FR-008 and US3. Dynamic state is unrepresentable, and declarations are captured by value.

**Independent Test**: The type-shape reflection test and the mutation test pass, and the shape test fails if a func, chan, pointer, map, or interface field is added.

- [X] T028 [P] [US3] Add a recursive reflection test in `schema/declarations_test.go` over `Prompt`, `PromptArgument`, `Resource`, `MCPPrompt`, `MCPPromptArgument`, and `MCPResource`: every field kind is `String`, `Bool`, or `Slice` of a struct satisfying the same rule. The failure message names the offending type, field, and kind, and explains that resources are static by constitution (Principle VI)
- [X] T029 [P] [US3] Add a capture-by-value test in `schema/declarations_test.go`: declare a prompt whose `Arguments` slice and string fields come from caller variables, and a resource; mutate the caller's slice element and struct after declaring; run `__schema` twice and assert both runs equal the pre-mutation expectation and are byte-identical
- [X] T030 [US3] Add a determinism test in `schema/declarations_test.go` that runs `__schema` and `__schema --as=mcp` 10 times over the declaration fixture and asserts byte-identical stdout (FR-010, SC-004)

**Checkpoint**: The static guarantee is enforced by tests.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T031 Add one prompt and one resource to the integration CLI root in `examples/integration/main.go` (handling the returned errors), then regenerate and review `examples/integration/testdata/schema_ax.golden.json` and `examples/integration/testdata/schema_mcp.golden.json`; check with `git diff --word-diff` that the only change is the inserted `prompts`/`resources` members (FR-014, SC-001)
- [X] T032 [P] Document the declaration API, its validation rules, the duplicate failure, and the Phase 2 deferral in `schema/doc.go` and the `__schema` section of `README.md`
- [X] T033 [P] Add a "Declare prompts and resources" section to `docs/src/content/docs/guides/expose-schema.md` covering the quickstart code, both output shapes, and the note that the live `mcp-server` does not serve them yet
- [X] T034 Run `make surface-update`, review every changed line of `internal/cmd/surfacecheck/baseline.json` (additions only), then run `go run ./internal/cmd/surfacecheck -audit-seed` and append hand-classified `supported` / `keep-public` / `live` records for every new root feature (types, wrappers, and the promoted `CommandSchema.Prompts`/`Resources` and `MCPSchema.Prompts`/`Resources` fields) to `specs/023-internalize-helpers/public-surface-audit.json`, keeping its required sort order; run `make surface-check` until green
- [X] T035 Run `gofmt`, `make validate`, `make test` (four-tag matrix, race), `golangci-lint run` (directly, per the repo's actionlint note), `make cover-check` (confirm the `internal/schema` 93% and `internal/mcp` 96.9% floors), `make doc-coverage`, `make dead-check`, `make size-check`, and `make security`; fix every finding
- [X] T036 Run markdownlint on every changed Markdown file (`README.md`, `docs/src/content/docs/guides/expose-schema.md`, `specs/028-mcp-prompts-resources/**/*.md`) and walk through `specs/028-mcp-prompts-resources/quickstart.md` against the integration binary to confirm the documented output

---

## Dependencies & Execution Order

- **Setup (T001)** → **Foundational (T002–T004)** → story phases.
- **US1 (T005–T017)** is the MVP and depends only on Foundational.
- **US4 (T018–T019)**: T018 can run after US1. The resource half of T019 needs US2.
- **US2 (T020–T027)** depends on Foundational. It touches the same files as US1 (`internal/schema/declarations.go`, `schema/schema.go`, the duplicate golden), so run it after US1 rather than in parallel.
- **US3 (T028–T030)** depends on US1 and US2 types existing.
- **Polish (T031–T036)** depends on all stories. T034 must follow every exported-surface change. T035 runs last before T036.
- No governing ADR, so there is no ADR-retirement task.

## Parallel Opportunities

- US1 tests T005, T006, T007, T008 are in separate concerns and can be written together. T005 and T006 share a file, so write them as sequential edits.
- T017, T027, T032, and T033 touch independent files.
- T028 and T029 can be authored together.

## Implementation Strategy

1. **MVP**: Setup, Foundational, US1. Prompts are declared and projected and fail closed on duplicates. Stop and validate.
2. Add US4 to prove additivity.
3. Add US2 resources, then US3's static guarantee.
4. Finish with Polish: example, docs, surface baseline and audit, every gate.
