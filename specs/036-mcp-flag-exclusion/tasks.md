# Tasks: Per-Flag MCP Exclusion (`mcp.ExcludeFlag`)

**Input**: Design documents from `/specs/036-mcp-flag-exclusion/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/mcp-flag-exclusion.md

**Tests**: Required. Constitution Principle VII (Test-First Discipline) requires failing tests before implementation, and `ExampleExcludeFlag` is required on the new public entry point in `mcp/example_test.go`.

**Organization**: Tasks are grouped by user story (`US1`, `US2`, `US3`) and followed by a Polish phase for documentation, surface inventory update, and repository validation gates. No governing ADR exists, so no ADR-retirement task is included.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: User Story 1 - Per-Flag Exclusion from MCP Tool Schemas (Priority: P1) 🎯 MVP

**Goal**: Provide `mcp.ExcludeFlag(cmd *cobra.Command, name string)` (backed by `internal/mcp.MarkFlagExcluded` and `internal/mcp.IsFlagExcluded`) so excluded flags are omitted from `inputSchema.properties` and `inputSchema.required` in both `__schema --as=mcp` and live `mcp-server` `tools/list`, while remaining untouched in `--help`, direct CLI execution, and `__schema --as=ax`, and handling `nil` commands, empty/unknown flag names, and existing flag annotations safely.

**Independent Test**: `go test -race ./internal/mcp ./mcp -run 'TestIsFlagExcluded|TestMarkFlagExcluded|TestInputSchemaExcludedFlag|TestExcludeFlag'`

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T001 [US1] Add failing unit tests in `internal/mcp/exclude_test.go` (`TestIsFlagExcludedRequiresCanonicalValue`, `TestIsFlagExcludedNilFlag`, `TestMarkFlagExcludedNoopCases`, `TestMarkFlagExcludedLocalAndPersistent`, `TestMarkFlagExcludedPreservesExistingAnnotations`, `TestMarkFlagExcludedOverwritesNonCanonicalValue`) covering `IsFlagExcluded` and `MarkFlagExcluded` (FR-001, FR-002, FR-003).
- [x] T002 [P] [US1] Add failing unit tests in `internal/mcp/input_schema_test.go` (`TestInputSchemaExcludedFlags`) asserting that a local flag or a required flag marked with `MarkFlagExcluded` is omitted from `inputSchema.properties` and `inputSchema.required` while non-excluded flags remain (FR-004, SC-001).
- [x] T003 [P] [US1] Add failing public tests in `mcp/exclude_test.go` (`TestExcludeFlagOmittedFromStaticAndLiveSchemas`, `TestExcludeFlagStaysInHelpAndAXSchema`, `TestExcludeFlagNoopAndPreservesAnnotations`) verifying static/live schema omission, `--help` presence, `__schema --as=ax` presence, direct CLI execution with the excluded flag, and no-op/annotation-preservation semantics, plus runnable example `ExampleExcludeFlag` in `mcp/example_test.go` (FR-001, FR-002, FR-004, FR-007, FR-009, SC-001, SC-003).

### Implementation for User Story 1

- [x] T004 [US1] Implement `IsFlagExcluded(flag *pflag.Flag) bool`, `MarkFlagExcluded(cmd *cobra.Command, name string)`, and the `IsFlagExcluded` filter in `collectFlags` in `internal/mcp/mcp.go` (FR-001, FR-002, FR-003, FR-004, FR-005).
- [x] T005 [US1] Export `ExcludeFlag(cmd *cobra.Command, name string)` in `mcp/exclude.go` delegating to `internalmcp.MarkFlagExcluded(cmd, name)` with contract doc comments (FR-001, FR-002, FR-008, FR-009).

**Checkpoint**: User Story 1 unit, integration, and example tests pass (`T001`–`T005`).

---

## Phase 2: User Story 2 - Reject Excluded Flags on `tools/call` Before Execution (Priority: P1)

**Goal**: Reject any `tools/call` request supplying an excluded flag in `internal/mcpserver` (`buildArgs` / `acceptedFlags`) with `IsError: true` and a single `TextContent` item carrying the `validation_error` `ax.Error` envelope (exit `2`) without invoking the command's `Run`/`RunE`, while allowing `tools/call` requests that omit the excluded flag.

**Independent Test**: `go test -race ./internal/mcpserver ./mcp -run 'TestDispatchExcludedFlag|TestExcludeFlagRejectedOnToolsCall'`

### Tests for User Story 2 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T006 [US2] Add failing unit tests in `internal/mcpserver/dispatch_test.go` (`TestDispatchExcludedFlagIsValidationError`) asserting that supplying an excluded flag (bool, slice, or `null`) returns `IsError: true`, `len(Content) == 1`, `error_code == "validation_error"` (exit `2`), and `RunE` call count `0`, while omitting the excluded flag succeeds (FR-006, SC-002).
- [x] T007 [P] [US2] Add a failing live session test in `mcp/exclude_test.go` (`TestExcludeFlagRejectedOnToolsCallAndNeverRuns`) asserting that calling `tools/call` over the live MCP server with an excluded flag returns `IsError: true` with a `validation_error` envelope (exit `2`) and never invokes `RunE`, whereas calling the same tool without the excluded flag succeeds (FR-006, SC-002).

### Implementation for User Story 2

- [x] T008 [US2] Update `acceptedFlags` in `internal/mcpserver/dispatch.go` to record `seen[flag.Name]` and omit flags where `internalmcp.IsFlagExcluded(flag)` is `true` (FR-005, FR-006, SC-002).

**Checkpoint**: User Story 2 dispatcher and live session tests pass (`T006`–`T008`).

---

## Phase 3: User Story 3 - Persistent Flag Inheritance, Local Shadowing, and Static/Live Parity (Priority: P2)

**Goal**: Ensure excluded persistent flags on a parent command are excluded on inheriting child tools, local flags shadowing a persistent flag of the same name are evaluated by the local flag's own exclusion mark without falling back to the shadowed inherited flag, and static (`__schema --as=mcp`) and live (`mcp-server`) schemas and dispatch stay in lock-step.

**Independent Test**: `go test -race ./internal/mcp ./internal/mcpserver ./mcp -run 'TestInputSchemaExcludedFlags|TestDispatchExcludedFlag|TestExcludeFlagInheritanceAndShadowingParity'`

- [x] T009 [US3] Extend `TestInputSchemaExcludedFlags` in `internal/mcp/input_schema_test.go`, `TestDispatchExcludedFlagIsValidationError` in `internal/mcpserver/dispatch_test.go`, and add `TestExcludeFlagInheritanceAndShadowingParity` in `mcp/exclude_test.go` covering: (1) an excluded persistent flag on a parent is omitted and rejected on a child tool; (2) an excluded local flag on a child shadowing an unexcluded persistent flag on the parent is omitted and rejected on the child tool (without leaking the parent's flag) while remaining accepted on the parent tool; and (3) an unexcluded local flag on a child shadowing an excluded persistent flag on the parent remains included and accepted on the child tool (FR-004, FR-005, FR-006, SC-001, SC-002).

**Checkpoint**: All inheritance, shadowing, and static/live parity scenarios pass (`T009`).

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Update exported surface inventory, package and user-facing documentation, and run all repository quality gates.

- [x] T010 [P] Run `make surface-update` to record `func:ExcludeFlag` in `internal/cmd/surfacecheck/baseline.json` under `github.com/rshade/ax-go/mcp`, and verify `git diff internal/cmd/surfacecheck/baseline.json` shows only `mcp.ExcludeFlag` added and zero changes to root `ax` or the import-isolated packages (FR-008).
- [x] T011 [P] Update package documentation in `mcp/doc.go`, repository documentation in `README.md`, and the docs site guide in `docs/src/content/docs/guides/expose-schema.md` to document `mcp.ExcludeFlag` alongside `mcp.Exclude` (FR-009).
- [x] T012 Run `gofmt` and execute all repository verification gates (`make validate`, `make test`, `golangci-lint`, `make cover-check`, `make doc-coverage`, `make surface-check`, `make dead-check`, `make slop-check`, `make size-check`, `govulncheck`) across the 4-tag build matrix, plus verify all existing golden files in `testdata/` and `examples/integration/testdata/` are unchanged (SC-004).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (US1)**: Starts immediately (`T001`–`T003` tests first, then `T004`–`T005` implementation).
- **Phase 2 (US2)**: Depends on `T004` (`IsFlagExcluded` available in `internal/mcp`); `T006`–`T007` tests first, then `T008` implementation.
- **Phase 3 (US3)**: Depends on `T004`, `T005`, and `T008`.
- **Phase 4 (Polish)**: `T010` and `T011` can run in parallel once `T005` and `T008` are complete; `T012` runs last after all code, test, surface baseline, and documentation edits are complete.

### Parallel Opportunities

- `T001` (`internal/mcp/exclude_test.go`), `T002` (`internal/mcp/input_schema_test.go`), and `T003` (`mcp/exclude_test.go`, `mcp/example_test.go`) touch disjoint files and can be written in parallel.
- `T006` (`internal/mcpserver/dispatch_test.go`) and `T007` (`mcp/exclude_test.go`) touch disjoint files and can be written in parallel.
- `T010` (`internal/cmd/surfacecheck/baseline.json`) and `T011` (`mcp/doc.go`, `README.md`, `docs/src/content/docs/guides/expose-schema.md`) touch disjoint files and can be completed in parallel.

---

## Implementation Strategy

### MVP First (User Stories 1 & 2)

1. Write failing tests `T001`, `T002`, and `T003`.
2. Implement `T004` (`internal/mcp/mcp.go`) and `T005` (`mcp/exclude.go`).
3. Verify `T001`–`T005` pass.
4. Write failing tests `T006` and `T007`.
5. Implement `T008` (`internal/mcpserver/dispatch.go`).
6. Verify `T006`–`T008` pass.
7. Complete `T009` (US3 inheritance/shadowing parity), `T010` (`surface-update`), `T011` (docs), and `T012` (full repository validation gates).
