# Error Envelope Contract

Blocked confirmation uses the existing `ax.Error` / `contract.Error` JSON
shape. The feature adds no field and does not change the success envelope.

| Member | Value for this feature |
|---|---|
| `error_code` | Exactly `confirmation_required` |
| `message` | Deterministic confirmation-required message containing the caller's subject |
| `actionable_fix` | Fixed remediation naming the flag, `pass --yes to confirm this operation` |
| exit code | `2` (`ExitValidation`) |
| stream | `stderr` in direct CLI execution; MCP tool result text for `tools/call` |
| stdout | Zero bytes from the blocked direct-CLI path |

`trace_id`, `tool`, `version`, and `schema_version` retain the existing
normalization rules. `Confirm` returns the error; `Execute` or the MCP
dispatcher performs the canonical serialization. The envelope's field set is
covered by the existing error golden/surface checks.
