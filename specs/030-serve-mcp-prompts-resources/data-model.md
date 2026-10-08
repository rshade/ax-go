# Data Model: Serve Declared MCP Prompts, Resources, and Instructions

## Resource declaration (extended)

| Field | Type | Validation | Projected in `__schema` |
| --- | --- | --- | --- |
| `URI` | string | unchanged (spec 028) | yes |
| `Name` | string | unchanged | yes |
| `Title`, `Description` | string | unchanged | yes |
| `MIMEType` | string | unchanged | yes |
| `Content` | string | valid UTF-8, at most 1 MiB, new `too_long` / `invalid_utf8` reasons on field `content` | **never** |

- Public `schema.Resource.Content` is tagged `json:"-"`; internal
  `Resource.Content` is `json:"content,omitempty"` for the annotation encoding.
- Content is captured by value when declared. It is never a callback.

## Prompt declaration (validation extended)

Fields unchanged. `Template` additionally capped at 64 KiB (`too_long`).

## Server configuration

| Field | Where | Validation |
| --- | --- | --- |
| `Instructions` | `mcp.WithInstructions` -> `mcpserver.Config.Instructions` | valid UTF-8, at most 8 KiB; failure is `validation_error`, exit `2` |

## Rendered prompt

Derived, never stored: one `user`-role text message produced from a template
and an argument map on each `prompts/get`.

## Served sets

`prompts` and `resources` registered on the server equal
`schema.CollectDeclarations(root)`, after `FindDuplicate` and `FindCorrupt`
pass. The sets are fixed for the life of the server.
