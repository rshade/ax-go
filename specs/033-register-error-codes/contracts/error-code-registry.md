# Contract: the error-code registry and its drift guard

## Machine payloads

No change. `__schema` `error_envelope.known_codes` stays:

```json
"known_codes":["config_invalid","config_max_bytes_invalid","config_option_invalid","config_patch_invalid","config_too_large","confirmation_required","internal_error","validation_error","warnings_as_errors"]
```

Every `ax.Error` envelope keeps its `error_code` spelling and exit code. No
golden file changes.

## Go API: package `contract` (import-isolated)

New exported constants (untyped `string`), in `contract/codes.go`:

```go
const (
    ErrorCodeConfirmationRequired     = "confirmation_required"
    ErrorCodeInternalError            = "internal_error"
    ErrorCodeValidationError          = "validation_error"
    ErrorCodeInvalidSchemaDeclaration = "invalid_schema_declaration"
)
```

Unchanged: `KnownErrorCodes() []string` keeps its signature, contents, order,
and fresh-slice guarantee. Its body names only constants.

New unexported: `authoringTimeErrorCodes() []string`, holding
`ErrorCodeInvalidSchemaDeclaration`, with a comment giving spec 032 FR-001's
reason for the exclusion.

## Go API: other public packages

No exported identifier is added, removed, or changed in the root `ax`,
`schema`, `mcp`, `config`, `id`, `logging`, or `axtest` packages. Unexported
removals: root `contractErrorWarnings`, `schema.invalidDeclarationCode`.

## Surface gate delta

`internal/cmd/surfacecheck/baseline.json`, package
`github.com/rshade/ax-go/contract`: four `added` `const:` features with
signature `untyped string`, present in `all` configurations and profiles. The
root audit (`specs/023-internalize-helpers/public-surface-audit.json`) does
not change.

## Internal test helper: package `internal/testutil`

```go
type ErrorCodeGuard struct {
    ModuleDir       string
    ModulePath      string
    RootPackageName string
    Registered      []string
    ExcludedDirs    []ExcludedDir
    PassThroughs    []PassThrough
}

type ExcludedDir struct{ Dir, Reason string }

type PassThrough struct{ File, Func, Param, Reason string }

type ErrorCodeViolation struct {
    File string
    Line int
    Kind string
    Expr string
}

func FindErrorCodeViolations(guard ErrorCodeGuard) ([]ErrorCodeViolation, error)
func FormatErrorCodeViolations(violations []ErrorCodeViolation) string
func ParseStringConstants(dir string) (map[string]string, error)
```

- `FindErrorCodeViolations` returns the sorted violations, or an error when
  the configuration is invalid, a directory cannot be read, or a file does not
  parse. An error means the guard could not run and the caller fails the
  test; it never means "no violations".
- `FormatErrorCodeViolations` renders one line per violation:
  `file:line: kind: expr`.
- `ParseStringConstants` returns every exported constant in the non-test
  files of `dir` whose value is a string literal, keyed by name.

The helper takes no `context.Context`: it reads only local source files with
no network, goroutines, or cancellation (Principle X applies to cancelable
I/O).

## Standing tests

| Test | Asserts |
| --- | --- |
| `internal/testutil` `TestFindErrorCodeViolationsFixture` | The fixture yields exactly its planted violations, one or more of each kind, at their file and line; excluded paths and valid sites yield none. |
| `internal/testutil` `TestFindErrorCodeViolationsRejectsInvalidGuard` | Each invalid configuration returns an error. |
| `internal/testutil` `TestParseStringConstants` | The fixture's `contract` directory yields exactly its exported string constants. |
| `contract` `TestErrorCodeConstantSpellings` | Each of the ten constants holds its frozen spelling. |
| `contract` `TestErrorCodesAreRegistered` | This module yields zero violations. |
| `contract` `TestErrorCodeRegistryConsistency` | Constant ↔ list bijection, list disjointness, and list entries named only by identifiers (research R6). |
| `contract` `TestKnownErrorCodesProperties` (existing) | Sorted, unique, excludes the authoring-time code, fresh slice. |
| `contract` `ExampleKnownErrorCodes` (existing) | The exact nine codes. |
