# Contract: Per-Flag MCP Exclusion (`mcp.ExcludeFlag`)

## Public Go API (`package mcp`)

```go
// ExcludeFlag marks the named flag on cmd so it is omitted from the MCP tool
// input schema (tools/list and "__schema --as=mcp") and rejected with a
// validation error (exit 2) on tools/call. The flag is looked up among the
// flags cmd itself declares, in cmd.Flags(), then cmd.PersistentFlags(); a
// persistent flag excluded on a parent command is also excluded on every
// descendant command that inherits it without shadowing it. A name cmd only
// inherits is a no-op. A flag that is both required and excluded makes every
// tools/call on the tool fail with a validation error (exit 2). The flag
// stays visible in --help and in "__schema --as=ax". The mark is the pflag
// annotation "github.com/rshade/ax-go/mcp/exclude" set to []string{"true"}.
//
// The ax agent-safety and output flags (format, dry-run, yes,
// idempotency-key, strict) are reserved and never excluded. Existing flag
// annotations are preserved, and a nil cmd, empty name, or unknown flag name
// is a no-op.
func ExcludeFlag(cmd *cobra.Command, name string)
```

## Reserved Flags

`format`, `dry-run`, `yes`, `idempotency-key`, and `strict` are never excluded.
`mcp.ExcludeFlag` does not mark them, and `IsFlagExcluded` reports `false` for
them even when the annotation is set by hand, so they remain in
`tool.inputSchema.properties` and accepted on `tools/call`.

## Wire Contract: `tools/call` on a Tool with an Excluded Required Flag

When `watch` is both `MarkFlagRequired` and excluded, every `tools/call` on the
tool returns `IsError: true` with a `validation_error` envelope (exit `2`) whose
message is `required argument "watch" is excluded from MCP; tool "mycli-test"
cannot be called`. `Run` / `RunE` MUST NOT execute.

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
