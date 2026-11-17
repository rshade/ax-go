# Quickstart: Preserving Verdict Stdout on Non-Zero Exit in MCP

A command whose non-zero exit is a verdict (such as `check`, `test`, or `lint`) writes its structured result envelope to `cmd.OutOrStdout()` and returns an `*ax.Error`:

```go
check := &cobra.Command{
    Use:   "check",
    Short: "Check reachability",
    RunE: func(cmd *cobra.Command, _ []string) error {
        verdict := checkResult{Reachable: false, Reason: "blocked by policy rule #4"}
        if err := ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), verdict)); err != nil {
            return err
        }
        return ax.NewError(cmd.Context(), "unreachable", "destination is not reachable",
            ax.WithErrorExitCode(ax.ExitInternal))
    },
}
```

When invoked via CLI, the caller reads the JSON verdict from `stdout`, the `ax.Error` envelope from `stderr`, and the non-zero exit code.

When invoked via `mcp-server` (`tools/call`), the MCP client receives `isError: true` with two `TextContent` items:
- `content[0].text`: the `ax.Error` JSON envelope (`error_code: "unreachable"`).
- `content[1].text`: the command's verbatim `stdout` JSON envelope (`{"data":{"reachable":false,"reason":"blocked by policy rule #4"},...}`).

Commands that fail without writing to `stdout` (or fail argument validation, or panic) return a single `content[0]` item as before.

## Verification

```bash
go test -race ./internal/mcpserver ./mcp
make validate
make test
```
