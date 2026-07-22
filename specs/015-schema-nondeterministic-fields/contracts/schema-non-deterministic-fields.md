# Contract: `non_deterministic_fields` in `__schema` output

**Feature**: `015-schema-nondeterministic-fields` | **Data model**: [data-model.md](../data-model.md)

ax-go is a library, not a network service — its "contract" is the public Go
API surface plus the stable JSON shapes it emits (`__schema` output, the
`ax.Error` envelope). This document pins both halves of the contract added
by this feature. See `research.md` decisions D1–D6 for the reasoning behind
each choice below.

## 1. JSON contract — `__schema` (ax-native format)

### Before (current `testdata/schema_ax.golden.json`, abbreviated)

```json
{
  "schema_version": "1.0.0",
  "tool": "app",
  "version": "v0.1.0",
  "mode_detection": "--format flag > AGENT_MODE env > TTY detection",
  "command": {
    "use": "app",
    "short": "test app",
    "flags": [{ "name": "config", "type": "string", "usage": "config file" }],
    "commands": [
      {
        "use": "run",
        "short": "run something",
        "flags": [
          { "name": "name", "type": "string", "usage": "name to use" },
          { "name": "config", "type": "string", "usage": "config file" }
        ]
      }
    ]
  },
  "error_envelope": {
    "schema_version": "1.0.0",
    "required": ["error_code", "message", "trace_id", "tool", "version", "schema_version"],
    "optional": ["actionable_fix", "context", "suggestions"]
  }
}
```

### After (ax-native)

```json
{
  "schema_version": "1.0.0",
  "tool": "app",
  "version": "v0.1.0",
  "mode_detection": "--format flag > AGENT_MODE env > TTY detection",
  "command": {
    "use": "app",
    "short": "test app",
    "flags": [{ "name": "config", "type": "string", "usage": "config file" }],
    "non_deterministic_fields": [],
    "commands": [
      {
        "use": "run",
        "short": "run something",
        "flags": [
          { "name": "name", "type": "string", "usage": "name to use" },
          { "name": "config", "type": "string", "usage": "config file" }
        ],
        "non_deterministic_fields": [
          "data.generated_at",
          "meta.idempotency_key",
          "meta.span_id",
          "meta.trace_id"
        ]
      }
    ]
  },
  "error_envelope": {
    "schema_version": "1.0.0",
    "required": ["error_code", "message", "trace_id", "tool", "version", "schema_version"],
    "optional": ["actionable_fix", "context", "suggestions"],
    "non_deterministic_fields": ["trace_id"]
  }
}
```

**Guarantees**:

- `command.non_deterministic_fields` (and every nested `commands[].non_deterministic_fields`)
  is present on every command node, sorted, deduplicated, never `null`.
- `error_envelope.non_deterministic_fields` is present, exactly `["trace_id"]`.
- `schema_version` is unchanged (`"1.0.0"`) — this is an additive change
  (research.md D7).
- Adding a locator to a command's list later is non-breaking; removing one
  that shipped previously is breaking and requires the
  `breaking-change-approved` PR label plus a `feat!:`/`BREAKING CHANGE:`
  commit (Constitution Principle XI, spec.md FR-010).

## 2. JSON contract — `__schema --as=mcp`

### Before (MCP)

```json
{ "tools": [{ "name": "app run", "description": "run something", "inputSchema": { "type": "object", "properties": { "name": { "type": "string", "description": "name to use", "default": "" } } } }] }
```

### After (MCP)

```json
{
  "tools": [
    {
      "name": "app",
      "description": "test app",
      "inputSchema": {
        "type": "object",
        "properties": { "config": { "type": "string", "description": "config file", "default": "" } }
      },
      "nonDeterministicFields": []
    },
    {
      "name": "app run",
      "description": "run something",
      "inputSchema": {
        "type": "object",
        "properties": { "name": { "type": "string", "description": "name to use", "default": "" } }
      },
      "nonDeterministicFields": [
        "data.generated_at",
        "meta.idempotency_key",
        "meta.span_id",
        "meta.trace_id"
      ]
    }
  ]
}
```

**Guarantees**:

- `nonDeterministicFields` (camelCase, matching `inputSchema`'s convention)
  carries the identical locator set as the corresponding
  `CommandSchema.NonDeterministicFields` for the same command.
- The field is present on every MCP tool, sorted, deduplicated, and never
  `null`; tools with no command-scoped non-deterministic fields emit `[]`.

## 3. Go API contract

### `schema.WithNonDeterministicFields[T any](cmd *cobra.Command)`

*(forwarded as `ax.WithNonDeterministicFields[T any](cmd *cobra.Command)` from
the root package facade, per the existing `BuildSchema`/`NewSchemaCommand`
pattern.)*

- **Input**: `cmd`, the `*cobra.Command` being constructed; `T`, the payload
  type that command's `RunE` will wrap in `contract.Envelope[T]`.
- **When to call**: once, at command-construction time — anywhere after
  `cmd` is allocated and before `ax.Execute(root)` mounts `__schema` (in
  practice, alongside the command's other one-time setup such as
  `cmd.Flags()...`).
- **Effect**: marks `cmd` as a standard success-envelope command, adds the
  built-in `meta.trace_id`, `meta.span_id`, and `meta.idempotency_key`
  locators for that command, reflects `T`'s `ax:"nondeterministic"`-tagged
  fields into `data.`-prefixed locators (research.md D3), stores the
  registration on `cmd`, and makes the list available to `__schema` generation
  via the shared `internal/schema` helper.
- **Errors**: none returned. A nil `cmd` is a no-op; unusual `T` shapes are
  skipped fail-closed rather than panicking (see data-model.md, "Failure mode").
- **Idempotency**: calling it more than once for the same `cmd` overwrites
  the prior registration; it does not accumulate across calls.
- **Concurrency**: not safe to call concurrently with `ax.Execute(root)` for
  the same tree (command construction is expected to complete before
  `Execute` runs, matching every other Cobra command-tree-building call in
  this library).

### `ax:"nondeterministic"` struct tag

- **Applies to**: any exported field of a type passed as `T` to
  `WithNonDeterministicFields[T]`, or of `contract.Metadata`.
- **Recognized value**: exactly `"nondeterministic"`. No other value has
  defined behavior under this feature.
- **Effect on JSON marshaling**: none — it is a distinct tag key from `json`,
  read only by the reflection helper behind `WithNonDeterministicFields`.

### Extended structs (public API surface, `schema` package, forwarded to root `ax`)

```go
type CommandSchema struct {
    // ... existing fields unchanged ...
    NonDeterministicFields []string `json:"non_deterministic_fields"`
}

type ErrorSchemaInfo struct {
    // ... existing fields unchanged ...
    NonDeterministicFields []string `json:"non_deterministic_fields"`
}

type MCPTool struct {
    // ... existing fields unchanged ...
    NonDeterministicFields []string `json:"nonDeterministicFields"`
}
```

Adding fields to existing exported structs is non-breaking under
Constitution Principle XI (Go API surface: `add` is non-breaking; only
`remove`/`rename`/`re-type` is breaking).
