# Tasks: Preserve Command Stdout on MCP Non-Zero Exit

**Input**: Design documents from `/specs/035-mcp-error-stdout/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/mcp-error-stdout.md

**Tests**: Required. Constitution Principle VII (Test-First Discipline) requires failing tests before implementation, and FR-008 requires a golden-file test pinning the live `tools/call` error-with-stdout response.

**Organization**: Tasks are grouped by user story (`US1`, `US2`, `US3`) and followed by a Polish phase for documentation and repository validation gates. No governing ADR exists, so no ADR-retirement task is included.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: User Story 1 - Preserve Command Stdout on Non-Zero Exit (Priority: P1) 🎯 MVP

**Goal**: When a dispatched command returns a non-nil error and has written non-empty bytes to `stdout`, `mcp-server` returns `IsError: true` with two `TextContent` items: `Content[0]` carrying the marshaled `ax.Error` envelope and `Content[1]` carrying the captured `stdout` bytes verbatim, while forwarding command `stderr` exclusively to the server's `stderr` stream.

**Independent Test**: `go test -race ./internal/mcpserver ./mcp -run 'TestDispatchNonZeroExitPreservesStdout|TestLiveServerResponsesGolden|TestServeToolsCall'`

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T001 [US1] Add failing unit tests in `internal/mcpserver/dispatch_test.go` (`TestDispatchNonZeroExitPreservesStdout`) asserting that a command writing JSON or multi-line NDJSON to `stdout` (and diagnostic lines to `stderr`) before returning an error yields `IsError: true` with `len(Content) == 2`, where `Content[0]` decodes to the `ax.Error` envelope, `Content[1]` equals the exact `stdout` bytes verbatim across repeated calls, `stderr` is forwarded only to the server `stderr` writer (never into `Content[0]` or `Content[1]`), and the server continues serving subsequent calls (FR-001, FR-005, FR-006, SC-001).
- [x] T002 [P] [US1] Add a failing golden assertion in `internal/mcpserver/golden_test.go` and create `testdata/mcp_tools_call_error_stdout.golden.json` pinning the live MCP `tools/call` wire response for a non-zero-exit command with non-empty `stdout` using a deterministic W3C `traceparent` in `_meta` and a fixed `idempotency-key` argument (FR-001, FR-008, SC-001, SC-004).
- [x] T003 [P] [US1] Extend `TestServeToolsCall` in `mcp/server_test.go` with a failing assertion that a tool returning both a `stdout` JSON verdict envelope and an `ax.Error` over the public `mcp.Serve` HTTP session returns `IsError: true` with 2 content items (`ax.Error` first, verbatim `stdout` second) (FR-001, FR-005, SC-001).

### Implementation for User Story 1

- [x] T004 [US1] Update `dispatcher.errorResult` and `dispatcher.handle` in `internal/mcpserver/dispatch.go` so `errorResult(ctx context.Context, err error, stdout []byte)` places the marshaled `ax.Error` envelope in `Content[0]` and appends `&sdk.TextContent{Text: string(stdout)}` at `Content[1]` when `len(stdout) > 0` (FR-001, FR-005, FR-006, SC-001).

**Checkpoint**: User Story 1 unit, public server, and golden tests pass (`T001`–`T004`).

---

## Phase 2: User Story 2 - Single-Item Result on Empty Stdout and Validation Errors (Priority: P1)

**Goal**: Commands that fail with empty `stdout` (`len(stdout) == 0`) and all pre-execution or flag/argument validation errors return `IsError: true` with a single `TextContent` item (`len(Content) == 1`), identical to existing behavior.

**Independent Test**: `go test -race ./internal/mcpserver -run 'TestDispatchFailureKeepsServing|TestDispatchValidationErrors|TestDispatchInvalidScalarFlagValueIsValidationError|TestDispatchMissingRequiredFlagIsValidationError|TestDispatchRejectsOutOfSetEnum'`

- [x] T005 [US2] Verify and explicitly assert in `internal/mcpserver/dispatch_test.go` (`TestDispatchFailureKeepsServing`, `TestDispatchValidationErrors`, and flag validation tests) that a failing command with empty `stdout` and every validation error path returns `IsError: true` with `len(res.Content) == 1` and no trailing empty `TextContent` block (FR-002, FR-003, SC-002).

**Checkpoint**: User Story 2 single-item invariant is verified across all empty-stdout and validation-error cases.

---

## Phase 3: User Story 3 - Panic Recovery Discards Partial Stdout (Priority: P1)

**Goal**: When a command writes partial bytes to `stdout` (and optionally `stderr`) and then panics, `runRecovered` resets `outBuf` so `errorResult` receives empty `stdout` and returns `IsError: true` with only the single `internal_error` item (`len(Content) == 1`), while still flushing captured `stderr` to the server's `stderr` stream.

**Independent Test**: `go test -race ./internal/mcpserver -run 'TestDispatchRecoversPanic|TestDispatchFlushesGuardAuditLinesOnPanic'`

- [x] T006 [US3] Update `dispatchTestRoot`'s `"boom"` command (or add a dedicated subtest in `TestDispatchRecoversPanic` in `internal/mcpserver/dispatch_test.go`) so the panicking command writes partial bytes to `cmd.OutOrStdout()` and diagnostic text to `cmd.ErrOrStderr()` before panicking, asserting `res.IsError == true`, `len(res.Content) == 1`, `error_code == "internal_error"`, no partial `stdout` in `res.Content`, and `stderr` diagnostics still forwarded to the server's `stderr` writer (FR-004, FR-006, SC-003).

**Checkpoint**: User Story 3 panic-recovery buffer discard is verified.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Update package and user-facing documentation for the dispatch contract and run all repository quality gates.

- [x] T007 [P] Update package and function doc comments in `internal/mcpserver/doc.go`, `mcp/doc.go`, and `mcp/server.go` to document that `tools/call` on a non-zero exit returns `IsError: true` with the `ax.Error` envelope in `Content[0]` and, when the command wrote non-empty `stdout` without panicking, the verbatim `stdout` payload in `Content[1]` (FR-007).
- [x] T008 [P] Update `README.md` and `docs/src/content/docs/guides/expose-schema.md` to document the two-item `tools/call` error result contract when a failing command emits a `stdout` payload (FR-007).
- [x] T009 Run `gofmt` and execute all repository verification gates (`make validate`, `make test`, `golangci-lint`, `make cover-check`, `make doc-coverage`, `make surface-check`, `make dead-check`, `make slop-check`, `make size-check`, `govulncheck`) across the build-tag matrix (SC-004, SC-005).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (US1)**: Starts immediately (`T001`–`T003` tests first, then `T004` implementation).
- **Phase 2 (US2)**: Depends on `T004` (`errorResult` signature and behavior in place).
- **Phase 3 (US3)**: Depends on `T004` (`errorResult` and `runRecovered` interaction verified).
- **Phase 4 (Polish)**: `T007` and `T008` can run in parallel once `T004` is complete; `T009` runs last after all code, test, golden, and documentation edits are complete.

### Parallel Opportunities

- `T002` (`internal/mcpserver/golden_test.go` + `testdata/mcp_tools_call_error_stdout.golden.json`) and `T003` (`mcp/server_test.go`) touch separate files from `T001` (`internal/mcpserver/dispatch_test.go`) and can be written in parallel.
- `T007` (Go doc comments) and `T008` (Markdown documentation) touch disjoint files and can be completed in parallel.

---

## Implementation Strategy

### MVP First (User Story 1)

1. Write failing tests `T001`, `T002`, and `T003`.
2. Implement `T004` in `internal/mcpserver/dispatch.go`.
3. Verify `T001`–`T003` pass.
4. Complete `T005` (US2) and `T006` (US3) to lock in the empty-stdout, validation-error, and panic-recovery invariants.
5. Update documentation (`T007`, `T008`) and run all repository gates (`T009`).
