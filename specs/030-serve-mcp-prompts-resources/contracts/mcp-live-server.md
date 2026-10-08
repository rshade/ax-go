# Contract: Live `mcp-server` Prompts, Resources, and Instructions

## Go API

```go
// package mcp
func WithInstructions(text string) Option

// package schema
type Resource struct {
    // ... existing fields ...
    Content string `json:"-"`
}
```

- `WithInstructions` text must be valid UTF-8 and at most 8 KiB. Violations
  fail `Serve` / `NewCommand`'s run with `validation_error`, exit `2`, empty
  `stdout`.
- `DeclareResource` rejects content that is invalid UTF-8 or over 1 MiB, and
  `DeclarePrompt` rejects a template over 64 KiB, with
  `invalid_schema_declaration` and reason `too_long` / `invalid_utf8`
  (`context.field` `content` / `template`).
- Root `ax` re-exports through its existing identity aliases; `mcp.WithInstructions`
  is not re-exported by root (root does not alias `mcp`; verify with
  `make surface-check`).

## `initialize`

- With no prompts, no resources, and no instructions: byte-identical to
  v0.8.0.
- `capabilities.prompts` present only when at least one prompt is declared;
  `capabilities.resources` only when at least one resource is declared.
- `instructions` present only when `WithInstructions` was used.

## `prompts/list`

Same names, titles, descriptions, and arguments as `__schema --as=mcp`
`prompts`, same order. `template` is not part of the MCP prompt shape and is
not listed.

## `prompts/get`

Request `{name, arguments}`. Response: one message,
`{"role":"user","content":{"type":"text","text":<rendered>}}`, plus the
prompt description.

| Condition | Result |
| --- | --- |
| valid arguments | rendered text; `{{name}}` of declared args substituted, other braces literal; single pass |
| absent optional argument | rendered as empty text |
| missing required argument | invalid-params error, no text |
| argument not declared by the prompt | invalid-params error, no text |
| unknown prompt name | invalid-params error |
| argument value over 64 KiB | invalid-params error |

## `resources/list` and `resources/read`

`resources/list` matches `__schema --as=mcp` `resources` (uri, name, title,
description, mimeType), same order. `resources/read` for a declared URI returns
`{"contents":[{"uri":..., "mimeType":..., "text":<content>}]}`; an empty
declared content returns an empty `text`. An unknown URI is the SDK's
resource-not-found protocol error.

## `__schema` and `__schema --as=mcp`

Unchanged. Existing goldens stay byte-identical; no `content` key anywhere.

## Startup failure on tree conflicts

`Serve` fails closed with the same `validation_error` envelope `__schema`
emits for a duplicate prompt name, duplicate resource URI, or corrupt
declaration annotation, before registering anything.
