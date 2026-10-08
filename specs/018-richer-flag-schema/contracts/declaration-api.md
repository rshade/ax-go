# Contract: Declaration API

**Packages**: `github.com/rshade/ax-go/schema` (import-isolated, canonical) and
`github.com/rshade/ax-go` (root facade, forwarding/aliases).

**Stability**: additive public surface. The commit type is `feat:`, which means
a minor bump pre-v1.0 (Principle XI). Each identifier below is gated by
`surfacecheck`. The root identifiers also get audit rows classified as
`supported`.

## Types and constants

```go
// Capability is a command's side-effect class from ax-go's fixed vocabulary.
// It is the machine-comparable value an agent branches on; its meaning is
// identical across every ax-go CLI.
type Capability string

const (
    CapabilityReadOnly        Capability = "read-only"
    CapabilityCreate          Capability = "create"
    CapabilityMutate          Capability = "mutate"
    CapabilityDelete          Capability = "delete"
    CapabilityExternalNetwork Capability = "external-network"
    CapabilityAdmin           Capability = "admin"
)
```

The root facade exposes the same names:

- `type Capability = schema.Capability`
- the six constants, re-declared as `const CapabilityReadOnly = schema.CapabilityReadOnly`,
  and so on

## Authoring-error contract

Shared with spec 028's `DeclarePrompt` and `DeclareResource`. Every declaration
function returns nil on success. On an authoring mistake it returns an
`*ax.Error` (`*contract.Error`) and leaves `cmd` unchanged:

- `error_code`: `invalid_schema_declaration`
- exit code: `2`
- `message`: `invalid <kind> declaration "<key>": <field> <reason>`, where kind
  is `flag enum`, `flag example` or `capability`
- `context`: `{"field": ..., "reason": ...}`
- `actionable_fix`: a non-empty remediation hint per reason, from the same
  table as spec 028 (a `values`/`duplicate` enum gets its own canonicalisation
  hint)

| `field` | `reason` values |
|---------|-----------------|
| `cmd` | `nil_command` |
| `flag` | `flag_not_found`, `unsupported_type` |
| `values` | `required`, `invalid_value`, `duplicate` |
| `default` | `not_in_enum` |
| `example` | `required`, `invalid_value`, `not_in_enum` |
| `class` | `not_in_vocabulary` |

Callers branch with `errors.As(err, &axErr)` and `axErr.ErrorCode` /
`axErr.Context["reason"]`. No sentinel error is exported.

## Functions

### `DeclareFlagEnum(cmd *cobra.Command, flag string, values ...string) error`

Declares the only values the named flag accepts. The function finds the flag in
`cmd.Flags()` first, then in `cmd.PersistentFlags()`. A persistent flag's
declaration applies to every descendant command.

- **On success**: the flag's `Value` is wrapped. `__schema` lists `values`, in
  the given order, as `enum`. `--as=mcp` emits them as a typed JSON-Schema
  `enum`. Any value outside the set given at parse time is rejected before
  `PersistentPreRunE` or `RunE` runs, with `validation_error` and exit code `2`,
  including under `--dry-run`. The flag's own default is always accepted, even
  when it is empty and not a member. Calling the function again replaces the set.
- **Errors** (see the authoring-error contract), checked in this order:
  - `cmd` is nil (`cmd`/`nil_command`) or the flag is not found
    (`flag`/`flag_not_found`);
  - the flag type is not a string, custom, or integer scalar type
    (`flag`/`unsupported_type`);
  - `values` is empty (`values`/`required`);
  - a value does not parse as the flag's type (`values`/`invalid_value`);
  - two values are duplicates after canonicalisation (`values`/`duplicate`);
  - a non-empty default is not a member of the set (`default`/`not_in_enum`);
  - an example already declared on the flag is not a member
    (`example`/`not_in_enum`).
- **Caveat**: after the call, `flag.Value` is no longer the concrete pflag type.
  Do not type-assert it.

### `DeclareFlagExample(cmd *cobra.Command, flag string, example string) error`

Attaches one example value, written in CLI form exactly as it would appear on
argv. For a slice flag, write the elements as CSV, for example `a,b`.

- **On success**: `__schema` emits the value as `example`. `--as=mcp` emits it
  as a one-element `examples` array typed to the property. Calling the function
  again replaces the example.
- **Errors** (see the authoring-error contract):
  - `cmd` is nil or the flag is not found;
  - the example is empty (`example`/`required`);
  - the example does not parse as the flag's known type
    (`example`/`invalid_value`);
  - the example is not a member of the flag's declared enum
    (`example`/`not_in_enum`).

  Values for custom `pflag.Value` types are not type-checked.

### `DeclareCapability(cmd *cobra.Command, class Capability, note string) error`

Classifies the command's side effects. `note` is optional free-form detail, for
example `"idempotent by name"`. Surrounding whitespace is trimmed, and an empty
note is omitted.

- **On success**: the command node in `__schema` carries
  `capability: {class, note?}`. The `--as=mcp` tool carries the same
  `capability` object and the standard MCP `annotations` hints. The live
  `mcp-server` tool carries the hints. Hidden and reserved commands keep their
  declaration but stay out of the MCP tool set. Calling the function again
  replaces the class and note.
- **Errors** (see the authoring-error contract): `cmd` is nil
  (`cmd`/`nil_command`), or `class` is not one of the six constants
  (`class`/`not_in_vocabulary`).

## Unchanged behaviour (FR-006 / FR-008)

- `default` is still derived from `pflag.Flag.DefValue`, and `required` from
  Cobra's required annotation. The author does not declare them again.
- A CLI that calls none of these functions produces byte-identical `__schema`,
  `--as=mcp` and `mcp-server` `tools/list` output.

## Usage

```go
deploy := &cobra.Command{Use: "deploy", RunE: runDeploy}
deploy.Flags().String("output", "json", "output format")
deploy.Flags().Duration("timeout", 30*time.Second, "deadline")

if err := schema.DeclareFlagEnum(deploy, "output", "json", "table", "yaml"); err != nil {
    return err
}
if err := schema.DeclareFlagExample(deploy, "timeout", "45s"); err != nil {
    return err
}
if err := schema.DeclareCapability(deploy, schema.CapabilityMutate, "idempotent by release name"); err != nil {
    return err
}
```
