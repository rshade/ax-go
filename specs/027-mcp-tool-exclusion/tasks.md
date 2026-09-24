# Tasks: MCP Tool Exclusion

**Input**: [spec.md](spec.md), [plan.md](plan.md)

**Tests**: TDD. Write each test task first and confirm it fails before the matching implementation task.

## Phase 1: Setup

- [X] T001 Confirm the baseline: `make test`, `make lint`, `make surface-check` green on `027-mcp-tool-exclusion` before changes. Record the current `__schema --as=mcp` tool list for `examples/` CLIs as a before-snapshot.

## Phase 2: User Story 1 - Non-callable commands never become tools (P1)

- [X] T002 [US1] Table test in `internal/mcp/walk_test.go`: `WalkCallableCommands` skips non-runnable group nodes but visits their runnable children; skips a non-runnable root; skips a command named `help` (Cobra default via `InitDefaultHelpCmd`, plus an adopter-defined `help`).
- [X] T003 [US1] Implement the `Runnable()` node-only skip and the `help` reserved name in `internal/mcp/mcp.go` (FR-001, FR-002). Update the reserved-name comment block.

## Phase 3: User Story 2 - Node-only exclusion (P1)

- [X] T004 [US2] Tests in `internal/mcp/exclude_test.go` for `IsExcluded`/`MarkExcluded`: nil cmd, nil `Annotations`, existing annotations preserved, canonical value only (`"TRUE"`, `"1"`, `""` ⇒ not excluded).
- [X] T005 [US2] Walk test in `internal/mcp/walk_test.go`: an excluded runnable root keeps its children; an excluded leaf is absent; `Hidden` + excluded behaves like `Hidden` alone.
- [X] T006 [US2] Implement `ExcludeAnnotationKey`, `IsExcluded`, `MarkExcluded`, and the walk check in `internal/mcp/mcp.go` (FR-003, FR-004, FR-005).
- [X] T007 [US2] Add `mcp/exclude.go` with `func Exclude(cmd *cobra.Command)` plus `mcp/exclude_test.go`, and an `Example` in `mcp/example_test.go`.
- [X] T008 [US2] Tests: in `mcp/exclude_test.go`, `tools/call` over the wire on each excluded or skipped name fails identically to a never-registered name; in `internal/mcpserver/dispatch_test.go`, the dispatcher guard returns the `validation_error` envelope. `RunE` call counters stay 0 (FR-007, SC-003).
- [X] T009 [US2] Test in `mcp/exclude_test.go`: an excluded command still appears in `--help` output and in the `__schema --as=ax` command tree (unchanged shape), and its `cobra.Command.Annotations` carries the exclusion annotation (FR-008).

## Phase 4: User Story 3 - Parity (P2)

- [X] T010 [US3] Parity test in `mcp/exclude_test.go`: build a finfocus-shaped fixture with no positional-argument commands (runnable root with `RunE` marked `Exclude`, 6 groups, default help, `completion`, one blocking leaf marked `Exclude`, one hidden subtree). Assert static `__schema --as=mcp` names == live `tools/list` names, in order (FR-006, SC-001, SC-002).

## Phase 5: Example text

- [X] T011 Test and implement: `mcp.NewCommand(root)` `Example` uses the root name instead of `mycli` (FR-010).

## Phase 6: Polish

- [X] T012 Update every surface that states which commands become tools: `mcp/doc.go`, the `NewCommand` `Long` text in `mcp/command.go`, the `internal/mcp.Build`/`WalkCallableCommands` and `internal/mcpserver.discoverTools` doc comments, README "Running as an MCP server", and `docs/src/content/docs/guides/expose-schema.md` (new "Excluding commands" section with `Exclude` usage).
- [X] T013 `make surface-update` for the intentional `mcp.Exclude` addition. Verify root `ax` gained no exported symbols (FR-009).
- [X] T014 Update any golden/fixture whose diff is exactly group/help removal. Note each one for the commit message.
- [X] T015 Full gate: `make test`, `make validate`, `make lint`, `make doc-coverage`, `make cover-check`, `make surface-check`, all green.
- [X] T016 Write `PR_MESSAGE.md` (conventional commit `feat(mcp): ...`, validated with commitlint), including the behavior-change note and a reference to the finfocus adopter report. Do NOT commit.
