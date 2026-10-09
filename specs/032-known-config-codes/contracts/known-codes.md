# Contract: `known_codes` and the code constants

## Machine payload: `__schema` `error_envelope.known_codes`

Before (v0.9.0):

```json
"known_codes":["confirmation_required","internal_error","validation_error","warnings_as_errors"]
```

After:

```json
"known_codes":["config_invalid","config_max_bytes_invalid","config_option_invalid","config_patch_invalid","config_too_large","confirmation_required","internal_error","validation_error","warnings_as_errors"]
```

Guarantees:

- Scope: every `error_code` that ax-go library code can place in an error
  envelope delivered as the result of a command run, meaning anything
  returned between the start of command dispatch and the return of
  `ax.Execute` or an MCP tool call. That includes codes from public helper
  packages such as `config`.
- Excluded: adopter-defined codes, authoring-time codes
  (`invalid_schema_declaration`), and codes emitted only by this
  repository's gate tools.
- Byte-wise ascending, no duplicates, and identical across runs.
- Additive change under Principle XI. No field is added, removed, or
  retyped.

## Go API: package `contract` (import-isolated)

New exported constants (untyped `string`):

```go
const (
    ErrorCodeConfigInvalid         = "config_invalid"
    ErrorCodeConfigMaxBytesInvalid = "config_max_bytes_invalid"
    ErrorCodeConfigOptionInvalid   = "config_option_invalid"
    ErrorCodeConfigPatchInvalid    = "config_patch_invalid"
    ErrorCodeConfigTooLarge        = "config_too_large"
)
```

Unchanged identifiers, moved from `contract/warnings.go` to
`contract/codes.go` within the same package:

```go
const ErrorCodeWarningsAsErrors = "warnings_as_errors"
func KnownErrorCodes() []string
```

`KnownErrorCodes` keeps its signature. Its doc comment states the FR-001
scope. It returns a fresh slice on each call.

## Go API: package `schema`

`ErrorSchemaInfo.KnownCodes` keeps its type and JSON tag. It gains a field
doc comment that states the same scope rule.

## Surface gate delta

`internal/cmd/surfacecheck/baseline.json`, package
`github.com/rshade/ax-go/contract`: five `added` `const:` features with
signature `untyped string`, present in `all` configurations and profiles.
The root audit (`specs/023-internalize-helpers/public-surface-audit.json`)
does not change. See research R4.
