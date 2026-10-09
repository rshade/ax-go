# Research: Known codes include the config package's runtime codes

No Technical Context item was marked NEEDS CLARIFICATION. This file records
the decisions the plan rests on and the corrections to issue #284's file
list that the survey turned up.

## R1. Which codes satisfy FR-001

**Decision**: Exactly nine codes: `config_invalid`,
`config_max_bytes_invalid`, `config_option_invalid`, `config_patch_invalid`,
`config_too_large`, `confirmation_required`, `internal_error`,
`validation_error`, `warnings_as_errors`.

**Rationale**: A survey of every `NewError` call site and every
`ErrorCode` assignment in non-test library code (excluding `internal/cmd/`
and `examples/`) found these nine plus `invalid_schema_declaration`:

| Code | Emitted from | Runtime? |
| --- | --- | --- |
| `confirmation_required` | `confirm.go` | yes |
| `internal_error` | `contract/error.go`, `internal/mcpserver/dispatch.go` | yes |
| `validation_error` | `execute.go`, `mcp/command.go`, `internal/mcpserver/{server,transport,dispatch}.go` | yes |
| `warnings_as_errors` | `execute.go` via `contract.ErrorCodeWarningsAsErrors` | yes |
| `config_*` (five) | `config/config.go` | yes; adopter calls the helper inside `RunE` |
| `invalid_schema_declaration` | `schema/declarations.go` | no; returned while the tree is built |

The `mcp-server` startup paths (`internal/mcpserver/server.go`) run inside
the `mcp-server` subcommand, so they are runtime codes. All are
`validation_error`, so including them changes nothing.

**Alternatives considered**: Listing every code in the module, including the
gate tools' `surface_*`, `size_*`, `deadcode_*`, and `slopcheck_*` codes.
Rejected: those binaries are this repository's CI tooling and never run
inside an adopting CLI, so an agent driving that CLI can never receive
them.

## R2. Is widening the definition a breaking change?

**Decision**: No. It ships as `feat:` with no `breaking-change-approved`
label.

**Rationale**: Principle XI treats a machine payload as additive-tolerant.
Adding a field is non-breaking, and a semantic change to an existing field
is breaking. The field's meaning stays the same: it lists the codes ax-go
emits that an agent can receive. What changes is the list's accuracy. It
under-reported five codes that agents could already receive. A consumer that
reads `known_codes` as a set of codes it may meet stays correct with more
entries. No consumer can have depended on those five codes being absent,
because the codes reached them at runtime either way. The Go surface only
gains five constants, and adding them is non-breaking.

**Alternatives considered**: Treating the change as a semantic change and
labelling it breaking. Rejected: under that reading every future addition to
the list would be breaking, which would make the list impossible to correct.

## R3. Where the constants live and what they are called

**Decision**: Add five exported untyped string constants to package
`contract`, named after the existing `ErrorCodeWarningsAsErrors` pattern:
`ErrorCodeConfigInvalid`, `ErrorCodeConfigMaxBytesInvalid`,
`ErrorCodeConfigOptionInvalid`, `ErrorCodeConfigPatchInvalid`,
`ErrorCodeConfigTooLarge`. Move `KnownErrorCodes` and
`ErrorCodeWarningsAsErrors` from `contract/warnings.go` into a new
`contract/codes.go` that holds the code constants and the list, so #285 can
add its constants to the same file.

**Rationale**: `contract` is the import-isolated package a thin consumer
already links to match an envelope. Moving a declaration within the same
package doesn't change the API: the identifiers, signatures, and import
path stay the same, so `surfacecheck` sees no change for the moved ones.
The root `ax` package does not re-export `ErrorCodeWarningsAsErrors`
today, so it does not re-export the new constants either. That keeps the
root audit untouched and the root surface unchanged.

**Alternatives considered**:

- Constants in `config`. Rejected: a consumer matching an envelope would
  then need `config` imported, and #285 needs one home for every code.
- A typed `ErrorCode string` type. Rejected: `Error.ErrorCode` is a plain
  `string` field and retyping it is breaking. A typed constant would also
  diverge from the existing `ErrorCodeWarningsAsErrors`.
- Root re-exports such as `ax.ErrorCodeConfigTooLarge`. Rejected for this
  feature, because there is no precedent and it would add root audit rows.
  It can be a later additive change.

## R4. Correction: the surface audit is not touched

**Decision**: Update `internal/cmd/surfacecheck/baseline.json` only. Do not
edit `specs/023-internalize-helpers/public-surface-audit.json`.

**Rationale**: The issue lists both. `AGENTS.md` (Public Surface Gate) says
the audit is scoped to the root package, and that the other seven public
packages are gated by the baseline alone. This is confirmed:
`const:ErrorCodeWarningsAsErrors` is in the baseline under
`github.com/rshade/ax-go/contract` and has no audit row. Five new `contract`
constants are therefore five `added` baseline entries and zero audit rows.

## R5. How the goldens change

**Decision**: Edit the `known_codes` array in the four minified goldens
(`testdata/schema_ax.golden.json`, `testdata/schema_ax_declarations.golden.json`,
`testdata/schema_ax_enriched.golden.json`,
`examples/integration/testdata/schema_ax.golden.json`), and verify the edits
with the tests that read them.

**Rationale**: Each golden is one line, and `known_codes` is the only field
that changes, so the expected diff is exactly that array. The root and
`schema` goldens have no update flag (`assertGolden` compares bytes), and
`examples/integration` has `-update`. Hand-editing one array and letting the
byte-compare tests confirm it keeps the diff reviewable. The `--as=mcp`
projection does not carry `known_codes`, so the MCP goldens do not change.

## R6. Test strategy (Principle VII, test first)

**Decision**:

- `contract/codes_test.go`: a property test that the list is sorted, has no
  duplicates, excludes `invalid_schema_declaration`, and is a fresh slice on
  each call (mutating one result leaves the next call unchanged).
- `contract` `ExampleKnownErrorCodes` with an `// Output:` block that pins
  the exact nine codes. The exact list is asserted once, in the verified
  example, rather than duplicated in a unit test.
- `config/config_test.go`: a table over the six failure paths (nil option,
  out-of-range cap, oversized input, invalid Hujson on read, invalid Hujson
  on patch, invalid patch document) that asserts each envelope's
  `ErrorCode` equals the matching `contract` constant and is in
  `KnownErrorCodes()`.
- `examples/integration/main_test.go`: run the integration CLI's `__schema`,
  then trigger the existing oversized-config failure, and assert the
  envelope's `error_code` appears in that CLI's `known_codes` (spec SC-002,
  end to end).
- Goldens (R5) cover the `__schema` byte shape.

The new tests fail before the implementation for the right reason: the
constants do not exist yet (compile failure), the example's output lists
four codes, and the integration assertion finds `config_too_large` missing
from `known_codes`.

## R7. Gates this touches

- `make surface-check`: five `added` features, reviewed and regenerated with
  `make surface-update`.
- `make size-check`: there is no new dependency. Unused untyped constants
  are not linked, so the isolated logging binary does not grow.
- `make doc-coverage`: a new example adds coverage and cannot regress it.
- `make bench-check`: no tracked hot path changes. `__schema` reflection
  copies a list that grows from four to nine strings, which is below noise.
  No run is required.
- No build-tag-gated file is touched. The 4-tag matrix still runs through
  `make test` and `make validate`.
