---

description: "Task list for feature 030: serve declared MCP prompts, resources and instructions on the live server"
---

# Tasks: Serve Declared MCP Prompts, Resources, and Instructions on the Live Server

**Input**: Design documents from `specs/030-serve-mcp-prompts-resources/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/mcp-live-server.md, quickstart.md

**Tests**: REQUIRED. Constitution Principle VII (test-first, non-negotiable) and AGENTS.md "Testing-First Discipline". Every test task lands, and fails for the right reason, before its implementation task.

**Organization**: Tasks are grouped by user story. US1 (resources and instructions), US2 (prompts) and US3 (no-declaration byte identity) are P1. US4 (instructions validation) and US5 (real client) are P2.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US5 from spec.md
- All paths are repo-relative to the worktree root

## Phase 1: Setup

- [X] T001 Confirm the baseline is green before changes: run `go test -race ./schema/... ./internal/schema/... ./internal/mcpserver/... ./mcp/... ./examples/integration/...` and `make surface-check` from the worktree root, and record any pre-existing failure before proceeding
- [X] T001a Capture the v0.8.0 handshake baseline from the UNMODIFIED server before any production change: add a test in `internal/mcpserver/server_test.go` that connects an in-memory client to a declaration-free fixture and writes the `initialize` result to `testdata/mcp_initialize_baseline.golden.json`; land it as the first change so later tasks diff against v0.8.0 behavior (spec SC-002)

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: The content field, the caps, and the startup guard every story rides on (research R-001, R-004, R-005).

- [X] T002 Write failing tests in `internal/schema/declarations_test.go` for `Resource.Content`: valid text round-trips through `AddResource`/`Resources`; invalid UTF-8 rejected with field `content` reason `invalid_utf8`; content over 1 MiB rejected with reason `too_long`; a template over 64 KiB rejected with field `template` reason `too_long`; a resource with no content stays valid
- [X] T003 Write failing tests in `schema/declarations_test.go`: `DeclareResource` with `Content` succeeds and surfaces violations as `invalid_schema_declaration` with the new reasons; a `CommandSchema` and `MCPSchema` marshaled for a resource with content contain no `content` key (json:"-" guard), and the existing declaration goldens stay byte-identical; extend the type-shape reflection test so `Content` (a string) passes while still failing for func, chan, pointer, map, interface
- [X] T004 Implement a `Content` string field (JSON key `content`, omitempty) on `internal/schema/declarations.go` `Resource`, add `maxResourceContentBytes` (1 MiB) and `maxTemplateBytes` (64 KiB) with checks in `ValidateResource` and `ValidatePrompt`; add a `Content` string field excluded from JSON (tag `-`) to the public `Resource` in `schema/declarations.go`, replace the type-conversion in `toMCPResource` with an explicit field copy, keep `fromInternalResource` and `DeclareResource` copying `Content`, and make `declarationFix` field-aware for `too_long` (URI: shorten to 2048 bytes; content: at most 1 MiB; template: at most 64 KiB) with a table test over all three fields so no envelope names the wrong limit, update the `Resource` and `Prompt` doc comments (content is static, never projected; keep templates short, put long text in resource content; absent optional arguments render empty). Make T002–T003 pass
- [X] T005 Write a failing test (FR-019) in `internal/mcpserver/server_test.go`: `Serve` over a root whose tree declares a duplicate prompt name, a duplicate resource URI, or a hand-written corrupt annotation returns the same `validation_error` (exit `2`) that `__schema` emits and starts no transport
- [X] T006 Implement the startup guard (FR-019): move `treeDeclarationError` and `truncateKey` from `schema/declarations.go` into `internal/schema` as an exported `TreeDeclarationError(ctx, *Conflict) error` (no public surface change; `schema/schema.go` calls it, and the existing `__schema` duplicate/corrupt goldens must stay byte-identical), then call `internalschema.FindDuplicate` and `FindCorrupt` with it in `Serve` in `internal/mcpserver/server.go` before building the server. Make T005 pass

**Checkpoint**: Content is declarable, capped, and unprojected; conflicting trees fail closed at startup.

---

## Phase 3: User Story 1 - An agent reads a protocol document the CLI ships (Priority: P1) 🎯 MVP

**Goal**: Declared resources are listed and read on the live server, and instructions reach `initialize`.

**Independent Test**: Declare one resource with content and set instructions; an in-memory client sees the instructions, `resources/list`, and a byte-exact `resources/read`.

- [X] T007 [US1] Write failing integration tests in `internal/mcpserver/resources_test.go` (in-memory SDK client over stdio and HTTP): `resources/list` equals `schema.BuildMCPSchema` `resources` (order, fields) for a fixture with root, group, excluded leaf and hidden-subtree declarations; `resources/read` returns declared content and MIME type byte for byte on repeated calls; an unknown URI is a protocol error; a content-less resource reads as empty text with its MIME type; a 6.6 KB content round-trips byte-exact
- [X] T008 [US1] Write failing tests in `mcp/server_test.go` and `internal/mcpserver/server_test.go`: `mcp.WithInstructions("x")` puts `x` in the `initialize` result `instructions`; with the option unset the field is absent
- [X] T009 [US1] Add `instructions string` to `mcp/options.go` with `WithInstructions(text string) Option` (documented contract: valid UTF-8, at most 8 KiB, fail closed exit `2`, not part of `__schema`), thread it into `mcpserver.Config.Instructions` in `mcp/server.go`, and in `newMCPServer` pass `&sdk.ServerOptions{Instructions: cfg.Instructions}` only when set (nil otherwise, keeping the unset handshake identical)
- [X] T010 [US1] Create `internal/mcpserver/resources.go`: register each resource from `schema.CollectDeclarations(root)` with `server.AddResource`, only when at least one exists, with a handler returning a `ReadResourceResult` of one `ResourceContents{URI, MIMEType, Text}` from the declared content; call it from `newMCPServer`. Make T007–T008 pass
- [X] T011 [US1] Add golden files `testdata/mcp_resources_list.golden.json`, `testdata/mcp_resources_read.golden.json`, and `testdata/mcp_initialize_instructions.golden.json` with a golden test in `internal/mcpserver/golden_test.go` (new files only; no existing golden regenerated)

**Checkpoint**: US1 independently demonstrable; this alone resolves go-decide#22's need.

---

## Phase 4: User Story 2 - A user invokes a declared prompt (Priority: P1)

**Goal**: Declared prompts are listed and rendered by `prompts/get`.

**Independent Test**: A prompt with a required and an optional argument renders correctly and rejects bad calls with no partial render.

- [X] T012 [US2] Write failing table tests in `internal/schema/declarations_test.go` for `RenderTemplate(template, values)`: substitution, repeated placeholder, literal `{x}`, `{{ name }}`, `{{unknown}}`, `{{{x}}}` and unterminated `{{` literals (agreeing with `templatePlaceholders` on every case in the existing table), absent optional renders empty, unicode and newlines byte-exact, a value containing `{{other}}` inserted verbatim and not re-expanded
- [X] T013 [US2] Implement `RenderTemplate` in `internal/schema/declarations.go` by refactoring the placeholder scanner into one shared span iterator used by both `templatePlaceholders` and `RenderTemplate`, so validation and rendering cannot disagree. Make T012 pass and keep the existing scanner tests and fuzz seeds green
- [X] T014 [US2] Write failing integration tests in `internal/mcpserver/prompts_test.go` (stdio and HTTP): `prompts/list` equals `BuildMCPSchema` `prompts` (names, titles, descriptions, arguments, order); `prompts/get` with valid arguments returns one user-role text message; missing required argument, undeclared argument, unknown prompt name, and an argument value over 64 KiB each return invalid-params with no rendered text; hidden-subtree prompts are not served; ten identical calls return identical bytes
- [X] T015 [US2] Create `internal/mcpserver/prompts.go`: register each prompt from `schema.CollectDeclarations(root)` with `server.AddPrompt` (`Arguments` mapped to the SDK prompt arguments), only when at least one exists; the handler validates arguments (unknown, missing required, oversized; message names the argument and never echoes the value) then returns `GetPromptResult` with one `user` `TextContent` from `RenderTemplate`; call from `newMCPServer`. Make T014 pass
- [X] T016 [P] [US2] Add a fuzz test `FuzzPromptsGet` in `internal/mcpserver/fuzz_test.go` (existing pattern) and `FuzzRenderTemplate` in `internal/schema/declarations_test.go`: never panics, deterministic, output length bounded by template plus value sizes, values never re-expanded
- [X] T017 [US2] Add goldens `testdata/mcp_prompts_list.golden.json` and `testdata/mcp_prompts_get.golden.json` with their golden tests in `internal/mcpserver/golden_test.go`

**Checkpoint**: US1 and US2 work independently and together.

---

## Phase 5: User Story 3 - Trees and servers that declare nothing are unchanged (Priority: P1)

**Goal**: Byte-identical handshake and goldens for adopters who declare nothing.

**Independent Test**: A declaration-free tree yields the v0.8.0 `initialize` result and capability set.

- [X] T018 [US3] Write tests in `internal/mcpserver/server_test.go` (untagged so they run in every build configuration): `initialize` result for a no-declaration tree is byte-identical to `testdata/mcp_initialize_baseline.golden.json` captured in T001a; capability set for four cases (none, prompts only, resources only, both) advertises only what is registered
- [X] T019 [US3] Verify the existing `testdata/schema_ax*.golden.json`, `schema_mcp*.golden.json`, `mcp_tools_list.golden.json` and `examples/integration/testdata/schema_*.golden.json` pass without regeneration (`git diff --stat` shows none modified except the additive integration goldens in T026)
- [X] T020 [US3] Add a stdout-cleanliness test in `internal/mcpserver/transport_test.go`: across a full session (initialize, list, get, read) nothing is written to the process `stdout` outside the protocol channel (Principle I)

**Checkpoint**: Additivity proven.

---

## Phase 6: User Story 4 - Instructions are validated and fail closed (Priority: P2)

**Goal**: Malformed or oversized instructions fail at startup.

**Independent Test**: Invalid UTF-8 and over-cap text each exit `2` with an `ax.Error` envelope on `stderr` and empty `stdout`.

- [X] T021 [US4] Write failing table tests in `internal/mcpserver/server_test.go` and `mcp/server_test.go`: valid text starts; invalid UTF-8 fails with `validation_error` exit `2`; text of exactly 8 KiB starts and 8 KiB + 1 fails; stdout empty on failure; error message does not echo the text
- [X] T022 [US4] Implement `validateInstructions` in `internal/mcpserver/server.go` beside `validateVersion` (`maxInstructionsBytes` = 8 KiB) and call it from `Serve`. Make T021 pass

---

## Phase 7: User Story 5 - Works in a real client (Priority: P2)

**Goal**: End-to-end proof in Claude Code, plus the adopter-facing example.

- [X] T023 [US5] Update `examples/integration/main.go` to declare one prompt, one resource with content, and `mcp.WithInstructions`; add the additive goldens and tests in `examples/integration/golden_test.go` (`prompts/get`, `resources/read`), keeping existing integration goldens byte-identical except additions
- [ ] T024 [US5] Manual verification in Claude Code: `claude mcp add` the integration example, then confirm and record in the PR the Claude Code version and steps: instructions in context, prompt as `/mcp__<server>__<prompt>` slash command rendering with arguments, resource attachable by `@`, and the model reading the resource through `ReadMcpResource` with no `@` mention after seeing only the instructions

---

## Phase 8: Polish & Cross-Cutting

- [X] T025 [P] Update `mcp/doc.go` and `schema/declarations.go` docs to remove "Phase 1 is a declaration only: the live mcp-server does not serve prompts yet" and document the served contract; add `ExampleWithInstructions` (or fold into an existing `mcp` example) in `mcp/example_test.go` and verify `make doc-coverage`
- [X] T026 [P] Update `README.md` (declaration API, `Content`, `WithInstructions`, the prompt/resource/instructions pattern for agents) and `examples/integration/README.md`
- [X] T027 Run `make surface-update`, then review every line of `git diff internal/cmd/surfacecheck/baseline.json` (expected additions only: `schema.Resource.Content`, `mcp.WithInstructions`, and the root alias-attributed `Resource.Content` member); append `supported` rows for each new root feature to `specs/023-internalize-helpers/public-surface-audit.json`
- [X] T028 Run the gates: `gofmt`, `make validate`, `make test`, `golangci-lint run` (and with `--build-tags=ax_no_grpc,ax_no_otlp`), `make cover-check` (raise `internal/mcp` / `internal/schema` floors only if coverage rose, per the Coverage Policy), `make doc-coverage`, `make surface-check`, `make dead-check`, `make size-check`, `make security`, `make bench-check` (`__schema` reflection path changed)
- [X] T029 Run `npx markdownlint-cli2` on all changed Markdown; write `PR_MESSAGE.md` (`feat(mcp): ...`, body noting the 64 KiB template cap tightening, `Closes #270`, spec directory, Claude Code version from T024) and validate with `cat PR_MESSAGE.md | npx commitlint`

---

## Dependencies & Execution Order

- Phase 1 -> Phase 2 (blocks everything) -> US1, US2 (independent of each other once Phase 2 is done; both edit `newMCPServer`, so land sequentially) -> US3 -> US4 -> US5 -> Polish.
- T001a's baseline golden MUST land before any production change (it is the v0.8.0 reference for T018).
- Within each story: tests (fail) -> implementation -> goldens.

## Parallel Opportunities

- T016 is parallel with T017 (different files).
- T025 and T026 are parallel (docs only).
- US1 and US2 test authoring (T007–T008, T012, T014) can proceed in parallel; implementation merges sequentially.

## Implementation Strategy

- **MVP**: Phase 1, Phase 2, US1. That alone unblocks go-decide#22 (instructions plus a readable resource).
- Then US2 (prompts), US3 (additivity proofs), US4, US5, Polish.
- Commit and PR steps (restricted commands) need explicit user approval.
