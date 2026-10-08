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

// ErrInvalidDeclaration is wrapped (%w) by every authoring error returned from
// WithFlagEnum, WithFlagExample, and WithCapability. It is a programmer error:
// if it reaches ax.Execute it maps to exit code 1, not 2.
var ErrInvalidDeclaration = errors.New("schema: invalid declaration")
```

The root facade exposes the same names:

- `type Capability = schema.Capability`
- the six constants, re-declared as `const CapabilityReadOnly = schema.CapabilityReadOnly`,
  and so on
- `var ErrInvalidDeclaration = schema.ErrInvalidDeclaration`, which is the same
  error value, so `errors.Is` holds across both packages.

## Functions

### `WithFlagEnum(cmd *cobra.Command, flag string, values ...string) error`

Declares the only values the named flag accepts. The function finds the flag in
`cmd.Flags()` first, then in `cmd.PersistentFlags()`. A persistent flag's
declaration applies to every descendant command.

- **On success**: the flag's `Value` is wrapped. `__schema` lists `values`, in
  the given order, as `enum`. `--as=mcp` emits them as a typed JSON-Schema
  `enum`. Any value outside the set given at parse time is rejected before
  `PersistentPreRunE` or `RunE` runs, with `validation_error` and exit code `2`,
  including under `--dry-run`. The flag's own default is always accepted, even
  when it is empty and not a member. Calling the function again replaces the set.
- **Errors**: the function returns an error wrapping `ErrInvalidDeclaration`,
  and leaves `cmd` unchanged, when:
  - `cmd` is nil or the flag is not found;
  - the flag type is not a string, custom, or integer scalar type;
  - `values` is empty;
  - a value does not parse as the flag's type;
  - two values are duplicates after canonicalisation;
  - a non-empty default is not a member of the set;
  - an example already declared on the flag is not a member.
- **Caveat**: after the call, `flag.Value` is no longer the concrete pflag type.
  Do not type-assert it.

### `WithFlagExample(cmd *cobra.Command, flag string, example string) error`

Attaches one example value, written in CLI form exactly as it would appear on
argv. For a slice flag, write the elements as CSV, for example `a,b`.

- **On success**: `__schema` emits the value as `example`. `--as=mcp` emits it
  as a one-element `examples` array typed to the property. Calling the function
  again replaces the example.
- **Errors**: the function returns an error wrapping `ErrInvalidDeclaration`
  when:
  - `cmd` is nil or the flag is not found;
  - the example is empty;
  - the example does not parse as the flag's known type;
  - the example is not a member of the flag's declared enum.

  Values for custom `pflag.Value` types are not type-checked.

### `WithCapability(cmd *cobra.Command, class Capability, note string) error`

Classifies the command's side effects. `note` is optional free-form detail, for
example `"idempotent by name"`. Surrounding whitespace is trimmed, and an empty
note is omitted.

- **On success**: the command node in `__schema` carries
  `capability: {class, note?}`. The `--as=mcp` tool carries the same
  `capability` object and the standard MCP `annotations` hints. The live
  `mcp-server` tool carries the hints. Hidden and reserved commands keep their
  declaration but stay out of the MCP tool set. Calling the function again
  replaces the class and note.
- **Errors**: the function returns an error wrapping `ErrInvalidDeclaration`
  when `cmd` is nil or `class` is not one of the six constants.

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

if err := schema.WithFlagEnum(deploy, "output", "json", "table", "yaml"); err != nil {
    return err
}
if err := schema.WithFlagExample(deploy, "timeout", "45s"); err != nil {
    return err
}
if err := schema.WithCapability(deploy, schema.CapabilityMutate, "idempotent by release name"); err != nil {
    return err
}
```
