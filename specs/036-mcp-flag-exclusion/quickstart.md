# Quickstart: Excluding an Incompatible Flag from MCP (`mcp.ExcludeFlag`)

When a command is a useful MCP tool, but one of its flags cannot work over MCP's request/response protocol (for example, `--watch`, which blocks forever), call `mcp.ExcludeFlag` on the command after registering the flag:

```go
testCmd := &cobra.Command{
    Use:   "test",
    Short: "Run policy tests",
    RunE:  runTest,
}
testCmd.Flags().String("filter", "", "run only matching tests")
testCmd.Flags().Bool("watch", false, "re-run tests on file changes")

mcp.ExcludeFlag(testCmd, "watch")
```

## What Happens

1. **`__schema --as=mcp` and `mcp-server` `tools/list`**: `mycli-test` is advertised with `filter` in `inputSchema.properties`, while `watch` is omitted.
2. **`mcp-server` `tools/call`**:
   - Calling `mycli-test` with `{"filter": "dns"}` runs `testCmd` with `--watch=false`.
   - Calling `mycli-test` with `{"watch": true}` returns `isError: true` with a `validation_error` `ax.Error` envelope (exit `2`: `unknown argument "watch" for tool "mycli-test"`) and never runs `runTest`.
3. **CLI `--help` and `__schema --as=ax`**: `mycli test --help`, `mycli test --watch`, and `mycli __schema --as=ax` still include `--watch` normally.

## Verification

```bash
go test -race ./internal/mcp ./internal/mcpserver ./mcp
make surface-check
make doc-coverage
make validate
make test
```
