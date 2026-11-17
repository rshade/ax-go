# Data Model: Preserve Command Stdout on MCP Non-Zero Exit

## MCP `CallToolResult` on Error (`IsError: true`)

No new Go types or struct fields are added. The existing `sdk.CallToolResult` returned by `dispatcher.handle` / `dispatcher.errorResult` has the following `Content` invariant:

| Failure Scenario | `IsError` | `len(Content)` | `Content[0]` | `Content[1]` |
| --- | --- | --- | --- | --- |
| Pre-dispatch validation error (malformed request, unknown tool, invalid JSON arguments) | `true` | `1` | `*sdk.TextContent` with marshaled `ax.Error` (`validation_error`) | *(absent)* |
| Argument / flag validation error (`buildArgs`, `SetFlagErrorFunc`, missing required flag, enum rejection) | `true` | `1` | `*sdk.TextContent` with marshaled `ax.Error` (`validation_error`) | *(absent)* |
| Command returns non-nil error with empty `stdout` (`len(stdout) == 0`) | `true` | `1` | `*sdk.TextContent` with marshaled `ax.Error` | *(absent)* |
| Command panics during reset, flag assignment, or body (after optional partial writes to `stdout`) | `true` | `1` | `*sdk.TextContent` with marshaled `ax.Error` (`internal_error`) | *(absent — `outBuf` reset on panic)* |
| Command returns non-nil error with non-empty `stdout` (`len(stdout) > 0`) and does not panic | `true` | `2` | `*sdk.TextContent` with marshaled `ax.Error` | `*sdk.TextContent` with verbatim `string(stdout)` |
