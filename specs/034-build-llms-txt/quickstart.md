# Quickstart: build-time discovery document

An adopting CLI generates the file from the command tree it already builds.
The CLI commits and publishes the file. ax-go does not serve it.

```go
var buf bytes.Buffer
err := ax.GenerateLLMsTxt(ctx, &buf, root,
    ax.WithPreambleFile("docs/llms.preamble.md"),
    ax.WithLinks([]ax.LinkSection{{
        Heading: "Docs",
        Links: []ax.Link{{
            Title: "Command reference",
            URL:   "https://example.test/cli",
            Notes: "Human documentation",
        }},
    }}),
)
```

The preamble file starts with a title line and contains a summary quote:

```markdown
# Tool name

> One sentence summary an assistant can quote.

More introduction, optional.
```

In this repository the integration example is the consumer. From the module
root, `go test ./examples/integration -run TestLLMsTxt -count=1` checks
`examples/integration/llms.txt` against a fresh generation. Regenerate that
file with `go test ./examples/integration -run TestLLMsTxt -update`, then
commit it in the same change as the command tree.

Do not add `__schema --as=llms-txt`. Agents that can execute the binary keep
using `__schema`.
