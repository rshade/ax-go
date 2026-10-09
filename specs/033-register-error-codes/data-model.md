# Data Model: Register every error code and guard the registry

## Code constant

An exported, untyped `string` constant in package `contract`, named
`ErrorCode` followed by the spelling in CamelCase.

| Constant | Value | List | Exit |
| --- | --- | --- | --- |
| `ErrorCodeConfigInvalid` | `config_invalid` | runtime | 2 |
| `ErrorCodeConfigMaxBytesInvalid` | `config_max_bytes_invalid` | runtime | 2 |
| `ErrorCodeConfigOptionInvalid` | `config_option_invalid` | runtime | 2 |
| `ErrorCodeConfigPatchInvalid` | `config_patch_invalid` | runtime | 2 |
| `ErrorCodeConfigTooLarge` | `config_too_large` | runtime | 2 |
| `ErrorCodeConfirmationRequired` (new) | `confirmation_required` | runtime | 2 |
| `ErrorCodeInternalError` (new) | `internal_error` | runtime | 1, or the exit the wrapped error maps to |
| `ErrorCodeValidationError` (new) | `validation_error` | runtime | 2 |
| `ErrorCodeWarningsAsErrors` | `warnings_as_errors` | runtime | 2 |
| `ErrorCodeInvalidSchemaDeclaration` (new) | `invalid_schema_declaration` | authoring-time | 2 |

Invariants:

- Each value is held by exactly one `ErrorCode*` constant.
- Each constant's value is in exactly one list.
- Values never change (frozen public contract).

## Runtime registry

`KnownErrorCodes() []string`: the nine runtime constants, byte-wise sorted, no
duplicates, a new slice per call. Published as `__schema`
`error_envelope.known_codes`. Membership is spec 032 FR-001.

## Authoring-time exclusion list

`authoringTimeErrorCodes() []string` (unexported): codes ax-go returns only
while the adopter builds the command tree, before command dispatch. Holds
`ErrorCodeInvalidSchemaDeclaration`. Disjoint from the runtime registry; never
published in `known_codes`.

## Emit site

A place in non-test library code that sets an envelope's `error_code`:

| Form | Code expression |
| --- | --- |
| `contract.NewError(ctx, code, msg, ...)` | second argument |
| root `NewError(ctx, code, msg, ...)` (qualified as `ax.NewError` outside the root package) | second argument |
| `Error{ErrorCode: code}` composite literal | the element value |
| `x.ErrorCode = code` assignment | the right-hand side |

A code expression is **registered** when it is `<contract>.<Name>` (any import
alias), or a bare `<Name>` inside package `contract`, where `<Name>` is an
exported `contract` constant whose value is in either list.

## Guard configuration (`internal/testutil.ErrorCodeGuard`)

| Field | Meaning |
| --- | --- |
| `ModuleDir` | Directory to walk; its `contract` subdirectory supplies the constants. |
| `ModulePath` | Module import path; the root package is imported by this path. |
| `RootPackageName` | Default local name of the root package import (`ax`). |
| `Registered` | Code values that count as registered (runtime plus authoring-time). |
| `ExcludedDirs` | Module-relative directories skipped with their subtrees, each with a reason. |
| `PassThroughs` | (file, function, parameter, reason) entries where a bare parameter is accepted. |

Validation (returned as an error, fail closed): `ModuleDir`, `ModulePath`,
and `RootPackageName` are non-empty; `Registered` is non-empty; every exclusion
and pass-through carries a non-empty reason; `ModuleDir/contract` exists and
parses.

## Violation (`internal/testutil.ErrorCodeViolation`)

| Field | Meaning |
| --- | --- |
| `File` | Module-relative path, slash-separated. |
| `Line` | 1-based line of the offending expression. |
| `Kind` | One of the kinds below. |
| `Expr` | The offending expression as Go source. |

| Kind | Reported when |
| --- | --- |
| `call_code` | A `NewError` call's code argument is not registered. |
| `field_code` | An `ErrorCode:` composite-literal element is not registered. |
| `assign_code` | An `x.ErrorCode = ...` right-hand side is not registered. |
| `constructor_value` | A `NewError` reference is not the function of a call. |
| `dot_import` | `contract` or the root package is dot-imported. |
| `stale_pass_through` | A pass-through entry matched no site. |

Violations are sorted by file, line, then kind, so output is deterministic.
