# Quickstart: Serving Prompts, Resources, and Instructions

```go
root := &cobra.Command{Use: "decide-cli"}

_ = schema.DeclareResource(root, schema.Resource{
    URI:      "cli://decide/skill",
    Name:     "decide-skill",
    Title:    "Decide protocol",
    MIMEType: "text/markdown",
    Content:  decideSkillMarkdown, // static text, captured at declaration
})
_ = schema.DeclarePrompt(root, schema.Prompt{
    Name:      "decide",
    Arguments: []schema.PromptArgument{{Name: "question", Required: true}},
    Template:  "Run the decide protocol on: {{question}}",
})

root.AddCommand(mcp.NewCommand(root, mcp.WithVersion(version),
    mcp.WithInstructions("Read cli://decide/skill before calling ask or score.")))
```

Verify:

```bash
decide-cli __schema --as=mcp          # prompts/resources metadata, no content
decide-cli mcp-server                  # serves them; initialize carries instructions
claude mcp add decide -- decide-cli mcp-server
```

In a new Claude Code session: the instructions appear in context, the prompt is
`/mcp__decide__decide`, and the resource can be `@`-mentioned or read through
`ReadMcpResource`.

Tests: `make test`, `make surface-check`, `make doc-coverage`, `make dead-check`.
