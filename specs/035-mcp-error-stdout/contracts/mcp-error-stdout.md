# Contract: MCP `tools/call` Error Result with Command Stdout

## Public Go API

Unchanged. Zero exported identifiers, signatures, or struct fields change in `ax`, `mcp`, `schema`, `contract`, `config`, `id`, `logging`, or `axtest`.

## Wire Contract: `tools/call` Result (`CallToolResult`)

### 1. Success (`runErr == nil`)

Unchanged:
```json
{
  "content": [
    {
      "type": "text",
      "text": "<verbatim command stdout>"
    }
  ]
}
```

### 2. Failure without command stdout (`runErr != nil` and `len(stdout) == 0`, or validation error, or recovered panic)

Unchanged:
```json
{
  "content": [
    {
      "type": "text",
      "text": "{\"error_code\":\"...\",\"message\":\"...\",\"trace_id\":\"...\",\"tool\":\"...\",\"version\":\"...\",\"schema_version\":\"1.0.0\"}"
    }
  ],
  "isError": true
}
```

### 3. Failure with non-empty command stdout (`runErr != nil` and `len(stdout) > 0`, no panic)

`IsError` is `true` and `content` contains two `text` items in order:
1. `content[0]`: the marshaled `ax.Error` envelope JSON.
2. `content[1]`: the command's captured `stdout` bytes verbatim (never re-parsed, trimmed, or re-serialized).

```json
{
  "content": [
    {
      "type": "text",
      "text": "{\"error_code\":\"...\",\"message\":\"...\",\"trace_id\":\"...\",\"tool\":\"...\",\"version\":\"...\",\"schema_version\":\"1.0.0\"}"
    },
    {
      "type": "text",
      "text": "<verbatim command stdout>"
    }
  ],
  "isError": true
}
```

## Stream Separation Invariant

Captured command `stderr` is forwarded exclusively to the server's configured `stderr` stream (`Config.Stderr` / `os.Stderr`) and never appears in `content[0]` or `content[1]`.
