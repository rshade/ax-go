# Data Model: Known codes include the config package's runtime codes

## Known code

An `error_code` string that satisfies spec FR-001.

| Code | Contract constant | Exit | Emitted by |
| --- | --- | --- | --- |
| `config_invalid` | `contract.ErrorCodeConfigInvalid` (new) | 2 | `config` read and patch |
| `config_max_bytes_invalid` | `contract.ErrorCodeConfigMaxBytesInvalid` (new) | 2 | `config` read |
| `config_option_invalid` | `contract.ErrorCodeConfigOptionInvalid` (new) | 2 | `config` option application |
| `config_patch_invalid` | `contract.ErrorCodeConfigPatchInvalid` (new) | 2 | `config` patch |
| `config_too_large` | `contract.ErrorCodeConfigTooLarge` (new) | 2 | `config` read |
| `confirmation_required` | none (#285) | 2 | `ax.Confirm` |
| `internal_error` | none (#285) | 1 | runtime, MCP dispatch |
| `validation_error` | none (#285) | 2 | runtime, MCP server |
| `warnings_as_errors` | `contract.ErrorCodeWarningsAsErrors` | 2 | `--strict` escalation |

Validation rules:

- Spellings and exit codes are frozen (spec FR-006).
- Each constant's value equals its code byte for byte.

## Known-code list

Returned by `contract.KnownErrorCodes()` and published as `__schema`
`error_envelope.known_codes`.

- Exactly the nine codes above (FR-002).
- Byte-wise ascending, no duplicates (FR-003).
- Never contains `invalid_schema_declaration` (FR-004).
- A fresh slice per call (FR-010).

Excluded on purpose:

| Code or class | Reason |
| --- | --- |
| `invalid_schema_declaration` | Authoring-time, returned before dispatch |
| Adopter codes (for example `integration_failure`) | Owned by the adopting CLI |
| Gate-tool codes (`surface_*`, `size_*`, `deadcode_*`, `slopcheck_*`) | Repository CI tooling, never inside an adopting CLI |

No state transitions. The list is a fixed value.
