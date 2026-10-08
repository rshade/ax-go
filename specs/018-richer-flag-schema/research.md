# Research: Richer Per-Flag `__schema` Semantics

**Feature**: 018-richer-flag-schema | **Date**: 2026-10-07

The spec resolved its own open questions: the vocabulary and the meaning of an
absent class were settled in Clarifications 2026-07-24, and FR-014 and FR-015
record the author's decisions. The Technical Context therefore contained no
`NEEDS CLARIFICATION` items. This document records the design decisions made
during planning.

## Decision Records Absorbed

None. Issue #28 asks to amend "ADR-0003", but that file no longer exists.
`docs/adr/` holds only ADR-0004 (trace-ID format) and ADR-0008 (Cobra as the
CLI framework). This feature obeys ADR-0008's constraint and changes neither
decision, so no ADR is absorbed and no retirement task is required. Per the
spec's Assumptions, the issue's acceptance criterion "ADR-0003 amended" is met
by this spec and research record.

---

## R1. Where enum enforcement happens

**Decision**: `WithFlagEnum` replaces the flag's `pflag.Value` with an internal
`enumValue` wrapper. The wrapper checks membership in `Set` and only then calls
the inner value's `Set`. It delegates `String()` and `Type()` to the inner value.

**Rationale**:

- pflag calls `Value.Set` while it parses flags. In Cobra's `execute`, that
  happens before `PersistentPreRunE`, which is where `ax.Execute` installs
  dry-run, and before `RunE`, which is where `Guard` and `Perform` run. An
  out-of-set value therefore cannot produce a dry-run envelope or a side effect
  (FR-014, US1-AS4/AS5), and the ordering does not depend on any change to
  `Execute`.
- pflag v1.0.10 wraps a `Set` error in `*pflag.InvalidValueError`
  (`flag.go:495`), which implements `Unwrap` (`errors.go:120`). Cobra's default
  `FlagErrorFunc` returns the error unchanged. `ax.Execute`'s
  `normalizeExecuteError` already calls `errors.As(err, &*ax.Error)`, so a
  `*contract.Error` returned from `Set` surfaces with exit code `2`.
  `normalizeExecuteError` fills in `trace_id`, `tool` and `version`. **No change
  to `execute.go` is needed.**
- The live `mcp-server` dispatcher also parses flags through Cobra, so the same
  wrapper enforces the enum there. A single enforcement point keeps the static
  and live paths from diverging.
- Persistent flags propagate: `cobra.InheritedFlags()` adds the same
  `*pflag.Flag` pointer to each child's set, so children see the wrapper with no
  duplicated declaration (FR-010).

**Alternatives considered**:

- *Validate in a wrapped `PersistentPreRunE`*: this runs only under
  `ax.Execute`, so the MCP server and direct `cmd.Execute()` callers would skip
  it. It would also run after the author's own `PersistentPreRun` ordering
  concerns. Rejected.
- *Validate in `Execute` before `ExecuteContext`*: this requires re-parsing
  argv, has the same MCP gap, and duplicates Cobra's parser. Rejected.
- *Use Cobra's `ValidArgs` or `RegisterFlagCompletionFunc`*: these provide shell
  completion only and do not enforce anything. Rejected.

**Consequence**: Code that type-asserts `flag.Value` to a concrete pflag type,
such as `*stringValue`, no longer matches after `WithFlagEnum`. That pattern is
rare and unsupported by pflag. It is documented on `WithFlagEnum`.

## R2. Source of truth for the allowed set

**Decision**: The `enumValue` wrapper owns the allowed set. The schema readers
in `internal/schema` and `internal/mcp` read the set with a type assertion on
`flag.Value`. The enum is **not** duplicated into `pflag.Flag.Annotations`.

**Rationale**: If the set were stored in two places, the advertised set and the
enforced set could diverge, for example when someone edits an annotation by
hand. One owner makes SC-002 (advertised) and SC-007 (enforced) the same fact.
If something later wraps `flag.Value` again, the schema stops advertising the
enum while enforcement continues. That is the fail-closed direction FR-009
requires: the contract never claims a set that is not enforced.

**Alternatives considered**: An annotation as the source, with the wrapper
reading it on each `Set`. This allows divergence and costs a map lookup on
every `Set`. Rejected.

## R3. Storing the example and the capability

**Decision**:

- The example is stored in `pflag.Flag.Annotations` under the key
  `github.com/rshade/ax-go/schema/example`, as a one-element `[]string`.
- The capability class and note are stored in `cobra.Command.Annotations` under
  `github.com/rshade/ax-go/schema/capability` and
  `github.com/rshade/ax-go/schema/capability-note`.

**Rationale**: These match the existing idiom. `WithNonDeterministicFields`
writes `cmd.Annotations` under the `github.com/rshade/ax-go/schema/...`
namespace, and Cobra marks required flags with a flag annotation. The data
travels with the Cobra objects, so the package needs no global state (Principle
X). `internal/schema.BuildCommand` already clones `cmd.Annotations`.

**Fail-closed reads (FR-009)**: The readers emit nothing when an annotation is
absent, empty, or malformed. Malformed includes a class outside the vocabulary,
an example annotation that is not exactly one element, and an example that no
longer validates against the flag's type or enum.

## R4. Surfacing authoring errors (FR-013)

**Decision**: Each declaration function returns `error`. Every authoring
failure wraps the sentinel `schema.ErrInvalidDeclaration` with `%w`, plus
detail naming the command path, the flag and the cause. When a declaration
fails, the function changes nothing: it checks everything before it mutates
anything.

**Rationale**:

- An inconsistency is found at the point of declaration, which is when the
  author builds the command tree, not when an agent first runs `__schema`.
- `BuildSchema` and `BuildMCPSchema` have no error return. Adding one would be a
  breaking change.
- The no-panic rule (Principle IX) rules out panicking. `errors.Is` works on the
  returned errors.
- The sentinel is a plain error, not an `*ax.Error`. A programmer error that
  reaches `ax.Execute` maps to exit code `1` (internal), which is correct,
  because a malformed declaration is not bad input from the agent.

**Defence in depth**: If the state changes after a successful declaration, for
example because the author mutates `DefValue` or edits an annotation by hand,
the schema readers check consistency again and omit the field (R3).

**Alternatives considered**:

- *No return value, matching `WithNonDeterministicFields`, with errors reported
  at schema-build time*: the error would surface late and only to agents, which
  is the opposite of FR-013. Rejected.
- *A `MustWithFlagEnum` that panics*: this violates Principle IX. Rejected.

## R5. Type support and canonical membership

**Decision**:

- **Supported flag types for an enum**: `string`, plus the integer scalars
  `int`, `int8` through `int64` and `uint`, `uint8` through `uint64`.
- **Custom `pflag.Value` types**: any type whose `Type()` is not a built-in
  scalar or slice type name is treated as a string.
- **Declaration errors**: `bool`, `count`, float types, duration and every slice
  type return `ErrInvalidDeclaration` (unsupported type).
- **Comparison**:
  - Integer flags compare canonically. The input is parsed with `strconv` at
    the flag's bit size and compared numerically, so `--n=03` matches `"3"`.
  - String and custom flags compare exactly and are case-sensitive.
- **The default**: a non-empty `DefValue` must be a member of the set. An empty
  `DefValue` means "no default" and is exempt. The existing schema already
  treats it that way: `Default` is `omitempty`, and
  `internal/mcp.jsonSchemaScalarDefault` emits no default for `""`. pflag never
  calls `Set` for the default, so an unset flag is never rejected. An explicit
  `--flag=""` is rejected unless `""` is a declared member.

**Rationale**:

- A boolean enum is just the type itself.
- Float equality is unreliable.
- Durations have many spellings, and canonicalising them is out of scope.
- Slice enums need per-element semantics, which the spec does not ask for. The
  spec's Assumptions note that multiple examples are also deferred.
- Treating `""` as unset avoids forcing every optional enum flag, such as an
  `--output` with an automatic default, to list `""` as a member.

**Membership-check code shared with the MCP adapter**: the `strconv` conversion
helpers that already exist in `internal/mcp`, namely `jsonSchemaType`,
`jsonSchemaScalarDefault` and `convertArrayDefault`, move into
`internal/schema/convert.go` so the declaration validator and the MCP adapter
use one implementation. The canonicaliser parses user input, so it gets a fuzz
test (Principle VII).

**Alternatives considered**: Allow every type and compare raw strings. Then
`--n=03` would be rejected while the schema advertised `3`, which is confusing
for an agent. Rejected.

## R6. MCP representation

**Decision**:

- **`enum`**: emitted on the input-schema property as a JSON array whose
  elements are typed to the property's JSON-Schema type. Integer flags produce
  numbers and string flags produce strings, in the author's order (FR-004,
  SC-005).
- **Example**: emitted as JSON-Schema `examples`, which is an **array** (draft
  2019-09 and later). It holds one element of the property's type. A slice flag
  produces an array value inside it, converted element by element with the
  existing slice converter.
- **Capability on the static `MCPTool`**:
  - An ax-specific `capability` object, `{"class": ..., "note": ...}`. This
    follows the existing ax-specific `nonDeterministicFields` extension field.
  - The standard MCP `annotations` object, derived from the class:

| Class | readOnlyHint | destructiveHint | openWorldHint |
|-------|--------------|-----------------|---------------|
| `read-only` | `true` | (omitted) | (omitted) |
| `create` | (omitted, false) | `false` | (omitted) |
| `mutate` | (omitted, false) | `true` | (omitted) |
| `delete` | (omitted, false) | `true` | (omitted) |
| `external-network` | (omitted) | (omitted) | `true` |
| `admin` | (omitted, false) | `true` | (omitted) |

- **Live `mcp-server`**: registers the same hints as `sdk.ToolAnnotations` on
  `sdk.Tool.Annotations`. It does not emit the ax-specific `capability` object
  in `_meta`; that is deferred, because naming `_meta` keys needs its own
  decision.

**Rationale**:

- With the standard hints, a generic MCP client can apply its own safety policy
  without knowing anything about ax-go. This is the same argument SC-005 makes
  for `enum`.
- The hints are conservative. MCP defines `destructiveHint` as "may perform
  destructive updates", and an update that overwrites state qualifies, so
  `mutate` and `admin` map to `true`.
- `external-network` says nothing about whether the command is read-only. Its
  only hint is `openWorldHint`, and `readOnlyHint` stays absent, which MCP
  interprets as false. That is the safe reading.
- The `examples` array matches JSON Schema. `example` (singular) is an
  OpenAPI-ism.
- The `schema` package defines its own `MCPToolAnnotations` struct and does not
  import the SDK type, so import isolation holds.

**Alternatives considered**:

- *Emit only the ax `capability`*: generic clients would gain nothing. Rejected.
- *Emit only the hints*: the hints are lossy. `create` and `mutate` differ only
  in `destructiveHint`, and the note would be lost. Rejected.
- *Set `idempotentHint`*: ax-go cannot know whether a command is idempotent.
  The note is where an author says "idempotent by name". Rejected.

## R7. One class or a set of classes per command

**Decision**: Exactly one class per command, as written in FR-015 ("a required
class"). A command that both mutates and reaches the network declares its most
consequential state effect, `mutate`, and states the network access in the note.

**Rationale**: This is what the spec says. A single value is what an agent
branches on, and a set would need ordering and conflict rules. Changing a
string field to an array later would break the payload, so if multi-class
support is ever needed it must be a new additive field, such as `also`, rather
than a re-type. This consequence is recorded here so a future feature does not
re-type the field.

**Alternatives considered**: A set of classes. More expressive, but outside the
spec and harder to branch on. Deferred.

## R8. Golden-file strategy

**Decision**: The existing `testdata/schema_ax.golden.json` and
`testdata/schema_mcp.golden.json` stay **byte-identical**. The fixture behind
them declares nothing, so it is the regression proof for SC-004 and FR-008. Two
new files, `testdata/schema_ax_enriched.golden.json` and
`testdata/schema_mcp_enriched.golden.json`, pin a fixture that declares an enum
on a string flag and on an int flag, an example on a slice flag, an example on a
persistent flag shown on a child command, and a capability with and without a
note. `examples/integration` adopts the declarations, so its goldens regenerate
with additive changes. The build-tag parity tests
(`buildtags_parity_test.go`) assert the enriched goldens too, because the files
are untagged.

## R9. The rejection envelope

**Decision**:

- `error_code`: `validation_error`, with exit code `2`.
- `message`: a constant shape that never contains the user's value, for example
  `flag --output: value is not one of the allowed values`.
- `context`: `{"flag": "<name>", "value": "<input>", "allowed": [...]}`.
- `suggestions`: one `--<flag>=<value>` string per allowed value, in declared
  order.

**Rationale**:

- The agent can repair its call from the envelope without reading `__schema`
  again.
- Keeping the raw input out of `message` follows the spirit of the
  log-injection rule in Principle IX: `message` is often echoed to people.
- `context` is a JSON-escaped map, and `encoding/json` sorts its keys, so the
  envelope is deterministic.

**MCP dispatcher adjustment**: `internal/mcpserver` overrides `FlagErrorFunc`
and currently re-wraps every flag error as `validationError(ferr.Error())`. That
loses the structured `context` and `suggestions`. The override will first check
`errors.As(ferr, &*contract.Error)` and return the structured error when it
finds one, so the live path returns the same envelope as `ax.Execute`.

## R10. Naming and placement of the public API

**Decision**: `schema.WithFlagEnum`, `schema.WithFlagExample`,
`schema.WithCapability`, the typed `schema.Capability` with constants
`CapabilityReadOnly`, `CapabilityCreate`, `CapabilityMutate`,
`CapabilityDelete`, `CapabilityExternalNetwork` and `CapabilityAdmin`, and
`schema.ErrInvalidDeclaration`. Root `ax` re-exports all of them by alias or
forwarding function. The constants are re-declared as typed constants of the
alias type, so `ax.CapabilityMutate` and `schema.CapabilityMutate` are the same
value.

**Rationale**: The `With<Thing>(cmd, ...)` shape mirrors
`WithNonDeterministicFields[T](cmd)`. That is the "one idiom" US4 asks for. The
`schema` package is the import-isolated home of the existing declaration
function, so a thin consumer can declare metadata without linking the runtime.

**Alternatives considered**: Functional options on `NewSchemaCommand`, for
example `WithFlagEnums(map[...]...)`. That moves the declaration away from the
flag, and an option has no way to return an error. Rejected.

## Rejected for scope

- **ax-go declaring an enum on its own `--format` flag**: the `__schema` output
  of every adopting CLI would change, which breaks FR-008 and SC-004. `--format`
  is already validated by `ResolveMode`. Possible follow-up issue.
- **Mapping flag-parse errors that do not come from ax-go to exit `2` in
  `ax.Execute`**: today an unknown flag or a non-numeric int falls through to
  `internal_error`, exit `1`. This gap already exists and is outside this
  feature. Candidate follow-up issue.
