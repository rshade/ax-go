# MCP Confirmation Contract

The MCP `tools/call` argument name is `yes` and its JSON type is boolean.

| Request | Dispatcher behavior |
|---|---|
| `yes` absent | Resolve approval false; a gated machine-mode command returns `confirmation_required`. |
| `yes: true` | Append `--yes=true` to the isolated Cobra invocation and resolve approval true. |
| `yes: false` | Resolve approval false; do not approve implicitly. |
| `yes` non-boolean | Return `validation_error` with exit code 2 before command execution. |

The argument is accepted through the same reflected persistent-flag path as the
other agent-safety primitives. The dispatcher injects its default/value into
each call's argv, applies `contract.WithApproval` to the per-call context, and
resets the shared Cobra tree before the next call. Approval is never stored on
the dispatcher, so sequential and concurrent calls cannot inherit it.

The tool result follows the existing MCP contract: success returns the command
stdout verbatim; a blocked command returns `CallToolResult{IsError:true}` with
the JSON error envelope as its sole text content. Command diagnostics remain on
the server stderr stream.
