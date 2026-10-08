# Data Model: Richer Per-Flag `__schema` Semantics

**Feature**: 018-richer-flag-schema | **Date**: 2026-10-07

## Entities

### Capability (closed vocabulary)

A typed string (`schema.Capability`, alias `ax.Capability`) with exactly six
legal values (FR-015):

| Constant | Value | Meaning an agent may rely on |
|----------|-------|------------------------------|
| `CapabilityReadOnly` | `read-only` | Observes state only; safe to call speculatively |
| `CapabilityCreate` | `create` | Creates new state; retries may duplicate without `--idempotency-key` |
| `CapabilityMutate` | `mutate` | Changes existing state (may overwrite) |
| `CapabilityDelete` | `delete` | Removes state |
| `CapabilityExternalNetwork` | `external-network` | Reaches a network endpoint outside the host; says nothing about state change |
| `CapabilityAdmin` | `admin` | Privileged or administrative operation |

- **Validation**: membership is checked with an exhaustive `switch`. Any other
  value, including `""`, a different case such as `Read-Only`, and surrounding
  whitespace, makes `DeclareCapability` return `invalid_schema_declaration`
  (`class`/`not_in_vocabulary`).
- **Absence**: a command that never declares a class is *unclassified*. The
  field is omitted, and no class is assumed (FR-003).

### Command capability declaration

The author's classification of one command.

| Field | Type | Rules |
|-------|------|-------|
| class | `Capability` | Required; must be a vocabulary member |
| note | `string` | Optional; `strings.TrimSpace` applied; empty after trim means absent; descriptive only |

**Storage** is `cmd.Annotations`:

- `github.com/rshade/ax-go/schema/capability` holds the class.
- `github.com/rshade/ax-go/schema/capability-note` holds the note. The key is
  deleted when the note is empty.

**Re-declaration** replaces the previous class and note, so the last call wins.
This matches `RegisterEnvelope`.

**Fail-closed read**: if the stored class is not a vocabulary member, the reader
omits the whole capability, the note included.

### Flag enum declaration (`enumValue` wrapper)

An internal type in `internal/schema` that wraps a flag's `pflag.Value`.

| Field | Type | Rules |
|-------|------|-------|
| inner | `pflag.Value` | The original value; never another `enumValue` (see re-declaration below) |
| allowed | `[]string` | Non-empty; no duplicates after canonicalisation; author order preserved |
| kind | internal enum | `string` or `int` with signedness and bit size, derived from `inner.Type()` |
| flag | `*pflag.Flag` | The owning flag: supplies the name for the rejection envelope and the live `DefValue` that `Set` always accepts |

**Methods**:

- `Set(v)`: if `v` equals the live `flag.DefValue`, it returns `inner.Set(v)`
  without a membership check, so restoring the default can never fail
  (research.md R5). Otherwise it canonicalises `v`. If `v` is not a member, it
  returns `*contract.Error` (R9) and does **not** call `inner.Set`, so the bound
  variable is not modified. If `v` is a member, it returns `inner.Set(v)`.
- `String()` and `Type()` delegate to `inner`.

**Re-declaration**: calling `DeclareFlagEnum` on a flag that is already wrapped
replaces `allowed` on the existing wrapper. It never wraps twice.

**Declaration-time validation** (R5). The checks run in this order, and the
first failure returns `invalid_schema_declaration` (exit 2) with nothing
mutated:

1. `cmd` is non-nil, and the flag exists in `cmd.Flags()` or
   `cmd.PersistentFlags()`.
2. The flag type is supported: `string`, a custom type, or an integer scalar.
3. `values` is non-empty.
4. Each value parses as the flag's type. Integer values must be valid at the
   flag's bit size.
5. No duplicates after canonicalisation. For example, `"3"` and `"03"` on an int
   flag count as duplicates.
6. A non-empty `DefValue` canonicalises to a member of the set.
7. An example already declared on the flag is a member of the set.

### Flag example declaration

| Field | Type | Rules |
|-------|------|-------|
| example | `string` | Non-empty; written in CLI form, exactly as an agent would pass it on argv |

**Storage**: `flag.Annotations["github.com/rshade/ax-go/schema/example"]` holds
`[]string{example}`.

**Declaration-time validation**. The first failure returns
`invalid_schema_declaration` (exit 2):

1. `cmd` is non-nil and the flag exists.
2. The example is non-empty.
3. The example parses as the flag's type. Known scalar types are checked with
   the shared converter, and known slice types are split as CSV with each
   element converted. Custom types are accepted unchecked, because ax-go cannot
   build a fresh instance of an unknown `pflag.Value`. The `DeclareFlagExample`
   doc comment says so.
4. If the flag has an enum, the example is a member of the set.

**Re-declaration** replaces the previous example.

**Fail-closed read**: the reader omits the example in three cases:

- the annotation does not have exactly one element;
- the element is empty;
- the element no longer passes check 3 or 4. This catches an annotation that
  someone edited by hand.

### Flag schema entry (extended), `schema.FlagSchema`

| JSON field | Go field | Type | Status |
|------------|----------|------|--------|
| `name` | `Name` | `string` | existing |
| `shorthand` | `Shorthand` | `string,omitempty` | existing |
| `type` | `Type` | `string` | existing |
| `default` | `Default` | `string,omitempty` | existing (derived, FR-006) |
| `usage` | `Usage` | `string,omitempty` | existing |
| `required` | `Required` | `bool,omitempty` | existing (derived, FR-006) |
| `enum` | `Enum` | `[]string,omitempty` | **new**; CLI-form strings in author order; nil when undeclared |
| `example` | `Example` | `string,omitempty` | **new**; CLI-form string |

Ax-native output keeps every value in CLI string form. That matches how
`default` is already emitted. Typed conversion happens only in the MCP adapter.

### Command schema entry (extended), `schema.CommandSchema`

The new field `Capability *CapabilitySchema` is emitted as
`json:"capability,omitempty"` and is nil when the command is unclassified.

`CapabilitySchema` has two fields:

- `Class Capability` with tag `json:"class"`
- `Note string` with tag `json:"note,omitempty"`

### MCP tool (extended), `schema.MCPTool` and `internal/mcp.Tool`

| JSON field | Type | Status |
|------------|------|--------|
| `name`, `description`, `inputSchema`, `nonDeterministicFields` | | existing |
| `capability` | `*CapabilitySchema,omitempty` | **new** |
| `annotations` | `*MCPToolAnnotations,omitempty` | **new**; nil when unclassified |

`MCPToolAnnotations` is ax-go's own struct, so the SDK is not imported into
`schema`. Its fields:

- `ReadOnlyHint bool` with tag `json:"readOnlyHint,omitempty"`
- `DestructiveHint *bool` with tag `json:"destructiveHint,omitempty"`
- `OpenWorldHint *bool` with tag `json:"openWorldHint,omitempty"`

Research R6 gives the mapping from class to hints.

**Input-schema property additions** (`internal/mcp.flagProperty`):

- `"enum"`: a `[]any` typed to the property type. Integer flags give `int64` or
  `uint64` values; string flags give `string` values.
- `"examples"`: a one-element `[]any`. A scalar flag gives a typed scalar; a
  slice flag gives a typed `[]any`.

## Relationships

```text
cobra.Command ──Annotations──▶ capability class + note ──▶ CommandSchema.Capability
      │                                                 └─▶ MCPTool.Capability / .Annotations ─▶ sdk.ToolAnnotations (live)
      └─ Flags()/PersistentFlags() ─▶ pflag.Flag
                                        ├─ Value: enumValue{inner, allowed} ─▶ FlagSchema.Enum, inputSchema.enum
                                        │                                     └─▶ Set() rejection (exit 2)
                                        └─ Annotations[example] ─▶ FlagSchema.Example, inputSchema.examples
Inherited flags: same *pflag.Flag pointer ⇒ child sees wrapper + annotation (FR-010)
```

## State transitions

**A flag value at parse time**:

```text
argv token ─▶ pflag.FlagSet.Set ─▶ enumValue.Set
      member?  yes ─▶ inner.Set(v) ─▶ (normal Cobra flow: PersistentPreRunE → RunE)
               no  ─▶ *contract.Error{validation_error, exit 2}
                       ─▶ pflag.InvalidValueError{cause}
                       ─▶ Cobra FlagErrorFunc
                            ├─ ax.Execute: default (identity) ─▶ normalizeExecuteError (errors.As) ─▶ stderr envelope, exit 2
                            └─ mcp-server: errors.As → structured result, IsError=true
                       (PersistentPreRunE/RunE never run ⇒ no dry-run envelope, no side effect)
```

**A declaration**: each declaration function is atomic. It validates everything
first and mutates only if every check passes, so a failed declaration leaves the
command exactly as it was.
