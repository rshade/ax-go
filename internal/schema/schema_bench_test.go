package schema_test

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/schema"
)

// benchCommandTree builds a small but representative *cobra.Command tree — a
// root command with three subcommands, each carrying a handful of flags of
// different types — sized to reflect a real ax-go-based CLI's command tree,
// not a worst-case stress test (research.md Decision 4).
func benchCommandTree() *cobra.Command {
	root := &cobra.Command{
		Use:     "ax",
		Short:   "ax-go CLI",
		Long:    "ax-go is the Agentic Experience foundation for Go CLI tools.",
		Example: "ax do-thing --format json",
	}
	root.PersistentFlags().String("format", "json", "output format")
	root.PersistentFlags().Bool("dry-run", false, "produce output without side effects")

	get := &cobra.Command{
		Use:     "get [resource]",
		Short:   "Get a resource",
		Long:    "Get fetches a single resource by ID.",
		Example: "ax get widget --id 123",
	}
	get.Flags().String("id", "", "resource ID")
	get.Flags().StringP("output", "o", "table", "output style")
	_ = get.MarkFlagRequired("id")

	list := &cobra.Command{
		Use:     "list",
		Short:   "List resources",
		Long:    "List enumerates resources, optionally filtered.",
		Example: "ax list widget --limit 50",
	}
	list.Flags().Int("limit", 100, "maximum results")
	list.Flags().StringSlice("filter", nil, "filter expressions")
	list.Flags().Bool("all", false, "include archived resources")

	schemaCmd := &cobra.Command{
		Use:     "__schema",
		Short:   "Emit the command schema",
		Long:    "__schema emits a structured JSON description of the command tree.",
		Example: "ax __schema --as=mcp",
	}
	schemaCmd.Flags().String("as", "", "schema adapter (e.g. mcp)")

	root.AddCommand(get, list, schemaCmd)
	return root
}

// BenchmarkBuildCommand measures schema.BuildCommand's allocation profile
// across the __schema reflection path — the tree is walked and every command
// and flag is copied into the internal schema shape on every invocation of
// the __schema command. This substantiates the __schema hot-path allocation
// claim tracked by the CI performance regression budget (research.md
// Decision 4).
func BenchmarkBuildCommand(b *testing.B) {
	root := benchCommandTree()

	b.ReportAllocs()
	for b.Loop() {
		_ = schema.BuildCommand(root)
	}
}
