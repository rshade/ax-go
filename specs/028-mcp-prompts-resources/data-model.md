# Data Model: MCP Prompts and Static Resources

All declaration types live in the public, import-isolated `schema` package.
Root `ax` aliases each one. Every field is a string, a bool, or a slice of a
value struct built from those, which FR-008 requires and a reflection test
enforces.

## Declaration and ax-native projection types

### `schema.Prompt`

| Field | Go type | JSON (`__schema`) | Rule |
| --- | --- | --- | --- |
| Name | `string` | `name` | required; `^[a-zA-Z0-9_.-]+$`; unique per command (declaration) and per tree (`__schema`) |
| Title | `string` | `title,omitempty` | UTF-8 |
| Description | `string` | `description,omitempty` | UTF-8 |
| Arguments | `[]PromptArgument` | `arguments,omitempty` | argument names unique within the prompt |
| Template | `string` | `template` | required, non-empty; every `{{name}}` names a declared argument |

### `schema.PromptArgument`

| Field | Go type | JSON | Rule |
| --- | --- | --- | --- |
| Name | `string` | `name` | required; `^[a-zA-Z0-9_.-]+$` |
| Title | `string` | `title,omitempty` | UTF-8 |
| Description | `string` | `description,omitempty` | UTF-8 |
| Required | `bool` | `required,omitempty` | — |

### `schema.Resource`

| Field | Go type | JSON (`__schema`) | Rule |
| --- | --- | --- | --- |
| URI | `string` | `uri` | required; ≤ 2048 bytes; no whitespace or control chars; absolute (non-empty scheme); unique per command and per tree |
| Name | `string` | `name` | required, UTF-8 |
| Title | `string` | `title,omitempty` | UTF-8 |
| Description | `string` | `description,omitempty` | UTF-8 |
| MIMEType | `string` | `mime_type,omitempty` | UTF-8, no control chars (spaces allowed) |

Phase 1 has no content field (clarification Q2).

### `schema.CommandSchema`, additive fields

| Field | Go type | JSON |
| --- | --- | --- |
| Prompts | `[]Prompt` | `prompts,omitempty` |
| Resources | `[]Resource` | `resources,omitempty` |

Placed after `Commands` and before `NonDeterministicFields`. JSON key order
follows struct order. Both are nil, and therefore omitted, when the command
declares nothing.

## MCP adapter projection types

| Type | Fields (JSON) |
| --- | --- |
| `MCPPrompt` | `name`, `title,omitempty`, `description,omitempty`, `arguments,omitempty` (`[]MCPPromptArgument`), `template` (ax extension) |
| `MCPPromptArgument` | `name`, `title,omitempty`, `description,omitempty`, `required,omitempty` |
| `MCPResource` | `uri`, `name`, `title,omitempty`, `description,omitempty`, `mimeType,omitempty` |

`MCPSchema` gains `Prompts []MCPPrompt` (`prompts,omitempty`) and
`Resources []MCPResource` (`resources,omitempty`) after `Tools`. Order is
pre-order over the root and every command not under a hidden child (reserved
commands included, matching the ax-native tree), then declaration order. The first occurrence of a
duplicate key wins.

## Storage: Cobra annotations

| Key | Value |
| --- | --- |
| `github.com/rshade/ax-go/schema/prompts` | JSON array of prompts, in declaration order |
| `github.com/rshade/ax-go/schema/resources` | JSON array of resources, in declaration order |

Lifecycle: `Declare*` validates, decodes the existing list, rejects a
same-command duplicate, appends, re-encodes, and writes. Other annotations are
preserved. On error the annotation map is not modified, and a nil map is not
allocated. `Declare*` refuses (`corrupt_annotation`) to rewrite an existing
value that fails to decode or holds an invalid entry. On read, such a value
projects no invalid entries (fail closed), and `__schema` reports it.

## Duplicate conflict (internal)

`{Kind: "prompt"|"resource", Key: string, Commands: [first path, second path]}`.
Command paths are `cobra.Command.CommandPath()`. This is the first conflict in
deterministic order: prompts before resources, then walk order.
