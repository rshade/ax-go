# Phase 1 Data Model: __schema Non-Deterministic Field Enumeration

**Feature**: `015-schema-nondeterministic-fields` | **Research**: [research.md](./research.md)

This feature has no persistent storage or database entities — "data model"
here means the Go types and their invariants that carry the new contract
through `__schema`, per [research.md](./research.md) decisions D1–D6.

## Entities

### Non-Deterministic Field Marking

A declarative annotation on a single struct field of an output payload type,
expressed as the struct tag `ax:"nondeterministic"`.

- **Applies to**: any exported field of a type used as `T` in
  `contract.Envelope[T]`, or a field of `contract.Metadata` itself.
- **Value**: presence-only; the tag key `ax` with value exactly
  `"nondeterministic"` marks the field. No other values are defined by this
  feature (D1 reserves the `ax:"..."` tag namespace for future
  non-determinism-adjacent annotations, but only `"nondeterministic"` has
  meaning today).
- **Invariant**: the tag has no effect on JSON marshaling (it is not a
  `json:` tag) and no effect on any field not reachable from a type passed to
  `WithNonDeterministicFields[T]` (or from `contract.Metadata`'s drift-test
  documentation tags, D2) — an orphaned tag on a field nobody ever registers
  is inert, not an error (D9).

### Non-Deterministic Field Locator

A `string` identifying one field's JSON path from an envelope's root, per
research.md D3: dot-separated `json` tag names, no array indices or map keys,
embedded structs inlined.

- **Examples**: `meta.trace_id`, `meta.span_id`, `meta.idempotency_key`,
  `data.report_id`, `data.items.id`.
- **Uniqueness**: unique within one command's own list; not required to be
  globally unique across the whole command tree (spec.md Assumptions).
- **Ordering**: every locator list in this feature's output is sorted
  lexicographically and deduplicated before being written (D8) — this is an
  output invariant, not a property of the locator type itself.

### `CommandSchema.NonDeterministicFields`

Extends the existing `schema.CommandSchema` struct (in
`schema/schema.go`) with:

```go
type CommandSchema struct {
    Use                    string          `json:"use"`
    Short                  string          `json:"short,omitempty"`
    Long                   string          `json:"long,omitempty"`
    Example                string          `json:"example,omitempty"`
    Flags                  []FlagSchema    `json:"flags,omitempty"`
    Commands               []CommandSchema `json:"commands,omitempty"`
    NonDeterministicFields []string        `json:"non_deterministic_fields"`
}
```

- **Cardinality**: exactly one list per command node, scoped to that
  command's own output shape; not inherited by or propagated to child
  commands in `Commands` (spec.md Edge Cases).
- **Contents**: for commands registered via
  `WithNonDeterministicFields[T]`, the sorted, deduplicated union of (a) the
  fixed built-in metadata literal `{"meta.trace_id", "meta.span_id",
  "meta.idempotency_key"}` (research.md D2) and (b) whatever `data.` locators
  were computed for the registered payload type. For commands without an
  envelope-output registration, the list is explicit and empty.
- **Presence**: no `omitempty` — always serialized, including the reachable
  empty-list case for raw-output, operational, or error-only command nodes, per
  FR-001 / D6. Populated with a non-nil `[]string{}` at minimum so
  `encoding/json` never emits `null`.

### `ErrorSchemaInfo.NonDeterministicFields`

Extends the existing `schema.ErrorSchemaInfo` struct with:

```go
type ErrorSchemaInfo struct {
    SchemaVersion           string   `json:"schema_version"`
    Required                []string `json:"required"`
    Optional                []string `json:"optional"`
    NonDeterministicFields  []string `json:"non_deterministic_fields"`
}
```

- **Cardinality**: exactly one, global — `ax.Error` is one flat shape shared
  by the whole CLI, not per-command (research.md D4).
- **Contents**: the fixed literal `["trace_id"]`.
- **Locator scope note**: unlike `CommandSchema`'s locators, these are
  unprefixed (no `meta.`/`data.` prefix) because `contract.Error` fields sit
  directly at the envelope root, matching the existing unprefixed naming in
  `Required`/`Optional`.

### `MCPTool.NonDeterministicFields`

Extends the existing `schema.MCPTool` struct with:

```go
type MCPTool struct {
    Name                    string         `json:"name"`
    Description              string         `json:"description,omitempty"`
    InputSchema              map[string]any `json:"inputSchema"`
    NonDeterministicFields   []string       `json:"nonDeterministicFields"`
}
```

- **Contents**: identical source list to the corresponding
  `CommandSchema.NonDeterministicFields` for the same command (research.md
  D5) — the same union computed once and shared via the `internal/schema`
  helper, not recomputed independently.
- **Presence**: no `omitempty` — always serialized, including empty-list tools,
  so MCP consumers get the same explicit signal as direct `__schema`
  consumers.

### `WithNonDeterministicFields[T any](cmd *cobra.Command)`

New exported generic function, `schema` package (forwarded from the root
`ax` package per the existing facade pattern):

```go
// WithNonDeterministicFields registers cmd as emitting contract.Envelope[T],
// adds the standard meta.* non-deterministic locators, and registers T's
// ax:"nondeterministic"-tagged fields as data.* locators. Call once, at
// command construction time, alongside the command's other setup (flags,
// etc.). T is reflected once, at registration time — never on the __schema
// request path.
func WithNonDeterministicFields[T any](cmd *cobra.Command)
```

- **Preconditions**: none that can panic. A nil `cmd` is a no-op. `T` may be any
  type, including one with zero `ax:"nondeterministic"`-tagged fields (the
  command still receives the built-in metadata set because it is registered as
  an envelope emitter, per D9).
- **Postconditions**: for a non-nil `cmd`, `cmd.Annotations` contains a private
  envelope-output marker plus the computed `data.` locator list under a private
  key; calling this function twice for the same `cmd` overwrites the previous
  registration (last write wins — no accumulation across multiple calls for the
  same command, since a command has exactly one output type).
- **Failure mode**: never panics, never returns an error — nil commands are
  ignored, and the runtime walker treats every reachable field kind as either
  "tagged: extract" or "not applicable: skip" (D9).

### `internal/schema.Command.Annotations` (new field, internal-only)

```go
type Command struct {
    Use         string
    Short       string
    Long        string
    Example     string
    Flags       []Flag
    Commands    []Command
    Annotations map[string]string // NEW: passthrough of cmd.Annotations
}
```

Pure passthrough — `BuildCommand` copies `cmd.Annotations` verbatim. No
output-shape inference happens in `BuildCommand`; the envelope-registration
plus locator-union logic (research.md D2/D5) lives in a small internal helper
(also in `internal/schema`, since both `schema` and `internal/mcp` already
import it) that both consumers call, given a `*cobra.Command` (for the
direct-Cobra-walk path in `internal/mcp`) or a `Command.Annotations` map (for
the `schema` package's tree-conversion path).

## Validation Rules

- A locator list, wherever it appears in output, MUST be sorted
  lexicographically and MUST NOT contain duplicate entries (D8).
- `CommandSchema.NonDeterministicFields` and
  `MCPTool.NonDeterministicFields` MUST be present (non-nil, non-omitted) on
  every command/tool node, including nodes with no registered envelope output
  and no marked fields (D5/D6).
- Adding a new locator to an existing command's list is non-breaking; removing
  one that was previously present is breaking (FR-010) — this is a review-time
  policy, not something enforced by a runtime check in this feature.
- `SchemaVersion`/`contract.ErrorSchemaVersion` are unchanged by this feature
  (D7) — no validation rule requires or forbids a bump; this is a reminder
  that none is expected as part of this change.

## Relationships

```text
contract.Metadata (TraceID, SpanID, IdempotencyKey tagged ax:"nondeterministic")
        │  (documents; NOT reflected at runtime — D2)
        ▼
built-in literal {"meta.trace_id", "meta.span_id", "meta.idempotency_key"}
        │  (used only when command has the envelope marker)
        ├──────────────► CommandSchema.NonDeterministicFields  (union with author fields, per registered command)
        │
        └──────────────► MCPTool.NonDeterministicFields        (same union, same command, via internal/schema helper)

T (author payload struct, tagged fields)
        │  WithNonDeterministicFields[T](cmd) — reflected once, at registration time
        ▼
cmd.Annotations["<private envelope marker>"] = "true"
cmd.Annotations["<private locator key>"] = "data.report_id,..."
        │  (internal/schema.Command.Annotations passthrough)
        ▼
CommandSchema.NonDeterministicFields / MCPTool.NonDeterministicFields (unioned with built-in literal above)

contract.Error (TraceID field)
        │  (fixed literal, no reflection — D4)
        ▼
ErrorSchemaInfo.NonDeterministicFields = ["trace_id"]
```
