# Quickstart: Declaring Prompts and Resources

```go
report := &cobra.Command{Use: "report", RunE: runReport}
root.AddCommand(report)

if err := ax.DeclarePrompt(root, ax.Prompt{
    Name:        "triage-spike",
    Title:       "Triage a cost spike",
    Description: "Find and explain the top cost driver in a window.",
    Arguments: []ax.PromptArgument{
        {Name: "window", Description: "lookback window, e.g. 7d", Required: true},
    },
    Template: "Run `app report --since={{window}}`, then `app explain` on the top line item.",
}); err != nil {
    return err // *ax.Error, error_code invalid_schema_declaration, exit 2
}

if err := ax.DeclareResource(root, ax.Resource{
    URI:      "app://docs/pricing-model",
    Name:     "pricing-model",
    MIMEType: "text/markdown",
}); err != nil {
    return err
}
```

Verify:

```bash
app __schema | jq '.command.prompts, .command.resources'
app __schema --as=mcp | jq '.prompts, .resources'
```

Validate the feature end to end in the worktree:

```bash
go test -race ./schema/... ./internal/schema/... ./internal/mcp/... ./internal/mcpserver/... ./examples/integration/...
make validate test cover-check doc-coverage surface-check size-check
golangci-lint run
```
