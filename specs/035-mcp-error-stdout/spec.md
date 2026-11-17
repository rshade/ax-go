# Feature Specification: Preserve Command Stdout on MCP Non-Zero Exit

**Feature Branch**: `035-mcp-error-stdout`

**Created**: 2026-11-17

**Status**: Draft

**Input**: GitHub issue rshade/ax-go#333. When a dispatched command exits non-zero, `mcp-server` returns only the `ax.Error` envelope and discards the command's captured stdout. For commands where a non-zero exit is a verdict (such as `check`, `test`, or `lint` commands in consumers like `github.com/rshade/tsacl`), the stdout result envelope is the evidence explaining the verdict, and an MCP client never sees it. When a command fails with non-empty captured stdout, `mcp-server` must emit `IsError: true` with the `ax.Error` envelope as the first `TextContent` item and the captured stdout bytes verbatim as the second `TextContent` item. Empty stdout, validation errors, and panic recovery must retain their current single-item behavior.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An agent reads the command's stdout verdict alongside the error envelope on non-zero exit (Priority: P1)

An agent invokes a CLI command through MCP `tools/call` where a non-zero exit represents a structured verdict (for example, a reachability check returning exit `1` for unreachable, a test runner reporting failed assertions, or a linter reporting findings). The command writes its structured JSON result envelope to `stdout` and returns an error. The MCP server responds with `IsError: true` and two `TextContent` items in order: the `ax.Error` envelope first, and the command's verbatim `stdout` payload second, so existing clients reading the first content item continue working while verdict-aware agents inspect the second item without forcing the CLI author to duplicate the stdout payload inside `ax.Error.context`.

**Why this priority**: This is the core defect reported in #333. Without the second content item, MCP clients lose the entire verdict payload whenever a command signals a negative verdict via a non-zero exit code.

**Independent Test**: Mount a command that writes a JSON envelope to `stdout` and returns an `ax.Error`. Call it via `tools/call` (both through the dispatcher and over a live MCP session). Verify the `CallToolResult` has `IsError: true` and `len(Content) == 2`, where `Content[0]` decodes as the `ax.Error` envelope and `Content[1]` equals the command's `stdout` bytes verbatim.

**Acceptance Scenarios**:

1. **Given** a command that writes bytes to `stdout` and returns a non-nil error, **When** an MCP client invokes it via `tools/call`, **Then** the response has `IsError: true` and two `TextContent` items: `Content[0]` is the marshaled `ax.Error` envelope and `Content[1]` is the exact `stdout` bytes written by the command.
2. **Given** a command that writes JSON to `stdout` and returns a non-nil error, **When** `tools/call` returns `Content[1]`, **Then** the second item's `Text` is byte-for-byte identical to what the command wrote to `stdout` (no re-parsing, re-formatting, or trimming).
3. **Given** the same failing command with non-empty `stdout` invoked repeatedly with deterministic inputs, **When** `tools/call` responses are compared, **Then** `Content[1]` is byte-identical across runs and the server continues serving subsequent calls normally.

---

### User Story 2 - Commands that fail without stdout and pre-execution validation errors keep their single-item shape (Priority: P1)

An agent calls a tool that fails before writing to `stdout` (such as an unknown tool name, invalid or missing arguments, flag parse failure, confirmation gate refusal, or a command whose body returns an error without writing to `stdout`). The MCP server returns `IsError: true` with a single `TextContent` item containing only the `ax.Error` envelope—never appending an empty trailing `TextContent` item.

**Why this priority**: Backward compatibility and contract cleanliness require that errors without `stdout` evidence do not gain an empty second content block.

**Independent Test**: Call an unknown tool, a tool with invalid arguments, and a failing command that writes nothing to `stdout`. Verify each returns `IsError: true` with `len(Content) == 1` containing the `ax.Error` envelope.

**Acceptance Scenarios**:

1. **Given** a command that writes nothing to `stdout` (0 bytes) and returns a non-nil error, **When** an MCP client invokes it via `tools/call`, **Then** the response has `IsError: true` and exactly one `TextContent` item (`Content[0]`, the `ax.Error` envelope).
2. **Given** a `tools/call` request that fails validation (malformed request, unknown tool, invalid JSON arguments, unknown argument, type-mismatched argument, missing required flag, or out-of-set enum flag), **When** the dispatcher rejects the call, **Then** the response has `IsError: true` and exactly one `TextContent` item (`Content[0]`, the `validation_error` envelope).

---

### User Story 3 - Panic recovery still discards partial stdout (Priority: P1)

A command writes partial output to `stdout` and then panics during execution. Because a panicking command may have emitted truncated or invalid JSON before crashing, the panic recovery boundary resets the `stdout` buffer and returns `IsError: true` with only the single `internal_error` envelope item.

**Why this priority**: Stream separation and output determinism forbid exposing half-written, corrupt buffers from a crashed command as machine payload evidence.

**Independent Test**: Mount a command that writes partial text to `cmd.OutOrStdout()` and then panics. Call it via `tools/call` and verify `IsError: true`, `len(Content) == 1`, `error_code == "internal_error"`, and no partial `stdout` in the result.

**Acceptance Scenarios**:

1. **Given** a command that writes bytes to `stdout` and then panics, **When** an MCP client calls it via `tools/call`, **Then** the panic is recovered, the captured `stdout` buffer is discarded, and the response has `IsError: true` with a single `TextContent` item carrying the `internal_error` envelope.
2. **Given** a command that writes to both `stdout` and `stderr` before panicking, **When** it panics during `tools/call`, **Then** `stdout` is discarded from the tool result while captured `stderr` diagnostics are still forwarded to the server's `stderr` stream.

---

### Edge Cases

- **Verbatim preservation on non-zero exit**: `stdout` is never parsed, trimmed, or re-serialized by `errorResult`. Whether a command wrote a single JSON object, trailing newline, or multiple NDJSON lines before returning an error, `Content[1]` carries the exact byte sequence as `TextContent.Text`.
- **Empty vs. non-empty `stdout`**: The presence of `Content[1]` is gated strictly on `len(stdout) > 0`. Zero bytes of `stdout` always produces a 1-element `Content` slice.
- **Flag parse failure in `ExecuteContext`**: If Cobra fails flag parsing inside `ExecuteContext`, nothing has been written to `outBuf` (`SilenceUsage` and `SilenceErrors` are both `true`, and diagnostics go to `errBuf`), so `len(stdout) == 0` and the result has 1 content item.
- **Stream separation preserved**: Command `stderr` output during a failing command with non-empty `stdout` continues to be forwarded exclusively to the server's `stderr` writer and never appears in `Content[0]` or `Content[1]`.
- **Live wire format pinned by golden**: The wire JSON of a `tools/call` error result carrying both the `ax.Error` envelope and verbatim `stdout` is pinned by a golden fixture in `testdata/`.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a dispatched command returns a non-nil error and its captured `stdout` has length greater than zero (`len(stdout) > 0`), the MCP dispatcher MUST return a `CallToolResult` with `IsError: true` and two `TextContent` items in `Content`: `Content[0]` carrying the marshaled `ax.Error` envelope and `Content[1]` carrying the captured `stdout` bytes verbatim as `TextContent{Text: string(stdout)}`.
- **FR-002**: When a dispatched command returns a non-nil error and its captured `stdout` is empty (`len(stdout) == 0`), the MCP dispatcher MUST return a `CallToolResult` with `IsError: true` and a single `TextContent` item (`Content[0]`, the marshaled `ax.Error` envelope), identical to existing behavior.
- **FR-003**: Pre-execution and argument validation failures (malformed request, unknown tool, invalid arguments, unknown flag, mistyped flag, missing required flag, or rejected enum value) MUST continue to return `IsError: true` with a single `TextContent` item carrying the `ax.Error` envelope.
- **FR-004**: When a panic occurs during per-call flag reset, slice assignment, or command execution, `runRecovered` MUST reset the captured `stdout` buffer (`outBuf.Reset()`) before returning the recovered `internal_error`, so a panicking command never emits partial `stdout` in `Content[1]`.
- **FR-005**: The dispatcher MUST NOT parse, trim, or re-serialize the captured `stdout` bytes when constructing `Content[1]`; the bytes written to `cmd.OutOrStdout()` MUST be preserved verbatim (Constitution Principle II).
- **FR-006**: Stream separation MUST be preserved: captured command `stderr` MUST be forwarded to the server's configured `stderr` stream and MUST NEVER appear in `CallToolResult.Content` (Constitution Principle I).
- **FR-007**: Package documentation (`internal/mcpserver/doc.go`, `mcp/doc.go`, `mcp/server.go`), repository documentation (`README.md`), and the docs site (`docs/src/content/docs/guides/expose-schema.md`) MUST document the two-item error result contract when a failing command writes to `stdout`.
- **FR-008**: A golden-file test in `internal/mcpserver/golden_test.go` MUST pin the live MCP `tools/call` wire response for a non-zero exit with non-empty `stdout`, and all existing goldens MUST pass unchanged.

### Key Entities

- **Tool Error Result (`CallToolResult` with `IsError: true`)**: The MCP tool execution result returned when a tool call fails. Contains `Content[0]` (always the `ax.Error` JSON envelope) and, when `len(stdout) > 0` without a panic, `Content[1]` (the command's verbatim `stdout` payload).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Calling a command via MCP `tools/call` that writes a JSON payload to `stdout` and returns an error produces `IsError: true` with `len(Content) == 2`, where `Content[0]` is a valid `ax.Error` envelope and `Content[1]` is byte-for-byte identical to the command's `stdout`.
- **SC-002**: Calling a command via MCP `tools/call` that writes nothing to `stdout` and returns an error, or fails request/argument validation, produces `IsError: true` with `len(Content) == 1`.
- **SC-003**: Calling a command via MCP `tools/call` that writes to `stdout` and then panics produces `IsError: true` with `len(Content) == 1` (`error_code: "internal_error"`), discarding all partial `stdout` bytes.
- **SC-004**: 100% of existing golden files in `testdata/` and `examples/integration/testdata/` pass without modification, and a new golden file pins the two-item error result shape.
- **SC-005**: All repository quality gates (`make validate`, `make test`, `golangci-lint`, `make cover-check`, `make doc-coverage`, `make surface-check`, `make dead-check`, `make slop-check`, `make size-check`, `govulncheck`) pass across the build-tag matrix.

## Assumptions

- Source inputs: GitHub issue #333. No frozen ADR in `docs/adr/` governs this feature (`0004-trace-id-format.md` and `0008-cli-framework-cobra.md` are unrelated).
- Clarification was not required: issue #333 specifies the exact problem, the two-item `TextContent` ordering (`ax.Error` first, verbatim `stdout` second), the empty-stdout single-item behavior, the validation-error invariant, the panic-recovery buffer reset invariant, and the documentation and golden-test expectations.
- The change is additive to the MCP `tools/call` machine contract (clients reading `Content[0]` continue to see the `ax.Error` envelope unchanged, while clients inspecting `Content[1]` receive the command's `stdout` payload when present) and makes zero changes to exported Go signatures.
