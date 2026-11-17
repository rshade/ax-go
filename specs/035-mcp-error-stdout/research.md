# Research: Preserve Command Stdout on MCP Non-Zero Exit

No governing ADR in `docs/adr/`; nothing to absorb or retire.

## R-001: Error result shape when `len(stdout) > 0`

- **Decision**: When `runErr != nil` in `dispatcher.handle`, pass the captured `stdout` slice to `d.errorResult(callCtx, runErr, stdout)`. `errorResult` always places the marshaled `ax.Error` envelope in `Content[0]` (`*sdk.TextContent`). When `len(stdout) > 0`, `errorResult` appends a second `*sdk.TextContent{Text: string(stdout)}` at `Content[1]`. When `len(stdout) == 0` (including pre-dispatch validation errors where `nil` is passed), `Content` has length 1.
- **Rationale**:
  - Keeping the `ax.Error` envelope at `Content[0]` preserves backward compatibility for existing MCP clients that inspect `res.Content[0]` when `res.IsError` is true.
  - Appending `stdout` verbatim at `Content[1]` gives verdict-aware MCP clients (such as agents calling `tsacl check`, `test`, or `lint`) the exact machine payload the command wrote to `stdout` without forcing the CLI author to duplicate the payload inside `ax.Error.Context`.
  - Using `TextContent` rather than `StructuredContent` matches `successResult(stdout)`, which wraps verbatim `stdout` bytes in `TextContent` without re-parsing or re-serializing JSON (preserving byte-for-byte determinism, NDJSON streams, and field ordering per Constitution Principle II).
- **Alternatives considered**:
  - Populating `CallToolResult.StructuredContent`: Rejected because `StructuredContent` requires a single parsed JSON object (`map[string]any` or `json.RawMessage`), cannot represent multi-line NDJSON streams without changing shape, and diverges from `successResult`.
  - Embedding `stdout` inside `ax.Error.Context`: Rejected by the issue statement because it duplicates data, forces re-serialization, and bloats the error envelope.
  - Concatenating `ax.Error` and `stdout` into a single `TextContent`: Rejected because two concatenated JSON documents in one `TextContent` are not valid single-document JSON and would break existing clients that `json.Unmarshal` `Content[0]`.

## R-002: Panic recovery and partial stdout

- **Decision**: Keep `outBuf.Reset()` in `runRecovered`'s `defer` block when `recover() != nil`.
- **Rationale**: A command that panics mid-execution may have written truncated or malformed bytes to `outBuf`. Discarding `outBuf` on panic ensures `execute` returns empty `stdout` (`len(stdout) == 0`), so `errorResult` emits only the `internal_error` envelope in `Content[0]`.

## R-003: Deterministic golden fixture for `tools/call` error with stdout

- **Decision**: Add a golden fixture `testdata/mcp_tools_call_error_stdout.golden.json` asserted in `internal/mcpserver/golden_test.go`. Because `startCallSpan` generates a random OTel `trace_id` when no `traceparent` is present in `_meta`, the golden test passes a fixed W3C `traceparent` in `CallToolParams.Meta` (`00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01`) and a fixed `"idempotency-key"` argument (or a command payload that does not include non-deterministic fields) so both `Content[0]` (`ax.Error.trace_id`) and `Content[1]` (the command's `stdout` envelope) are 100% deterministic across runs.
- **Rationale**: Golden tests require byte-identical output on every run without masking. Supplying `traceparent` in `_meta` and `idempotency-key` in `Arguments` exercises the real live session path while pinning a deterministic `trace_id` and `idempotency_key`.
