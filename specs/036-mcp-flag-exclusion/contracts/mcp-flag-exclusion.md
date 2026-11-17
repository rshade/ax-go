# Contract: Per-Flag MCP Exclusion (`mcp.ExcludeFlag`)

## Public Go API (`package mcp`)

```go
// ExcludeFlag marks the named flag on cmd so it is omitted from the MCP tool
// input schema (tools/list and "__schema --as=mcp") and rejected with a
// validation error (exit 2) on tools/call. The flag is looked up in
// cmd.Flags(), then cmd.PersistentFlags(); a persistent flag excluded on a
// parent command is also excluded on every descendant command that inherits it
// without shadowing it. The flag stays visible in --help and in
// "__schema --as=ax". The mark is the pflag annotation
// "github.com/rshade/ax-go/mcp/exclude" set to []string{"true"}. Existing flag
// annotations are preserved, and a nil cmd, empty name, or unknown flag name
// is a no-op.
func ExcludeFlag(cmd *cobra.Command, name string)
```

No other public package (`ax`, `contract`, `config`, `schema`, `id`, `logging`, `axtest`) gains or changes any exported symbol.

## Schema Contract (`__schema --as=mcp` and `tools/list`)

When `mcp.ExcludeFlag(cmd, "watch")` is called on a command `test` with flags `--watch` (bool) and `--filter` (string):

- `watch` MUST NOT appear in `tool.inputSchema.properties`.
- `watch` MUST NOT appear in `tool.inputSchema.required` (even if `cmd.MarkFlagRequired("watch")` was called).
- `filter` (and the standard agent-safety flags on live dispatch) remains in `tool.inputSchema.properties`.
- `<cli> __schema --as=ax` continues to include `watch` in `command.flags`.

## Wire Contract: `tools/call` Supplying an Excluded Flag

When an MCP client invokes `tools/call` with `{"name": "mycli-test", "arguments": {"watch": true}}`:

- The target command's `Run` / `RunE` MUST NOT execute.
- The server returns `CallToolResult` with `IsError: true` and `len(Content) == 1`:

```json
{
  "content": [
    {
      "type": "text",
      "text": "{\"error_code\":\"validation_error\",\"message\":\"unknown argument \\\"watch\\\" for tool \\\"mycli-test\\\"\",\"trace_id\":\"...\",\"tool\":\"mycli\",\"version\":\"1.0.0\",\"schema_version\":\"1.0.0\"}"
    }
  ],
  "isError": true
}
```
