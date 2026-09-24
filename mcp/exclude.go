package mcp

import (
	"github.com/spf13/cobra"

	internalmcp "github.com/rshade/ax-go/internal/mcp"
)

// Exclude marks cmd so it is never exposed as an MCP tool, for example because
// it is interactive, long-running, or a TUI entry point. Only cmd itself is
// excluded: its runnable descendants remain tools, and cmd stays fully visible
// in --help and in "__schema --as=ax". The mark is the Cobra annotation
// "github.com/rshade/ax-go/mcp/exclude" set to "true" on the command tree, so
// the static "__schema --as=mcp" adapter and the live "mcp-server" drop the
// same command, and a tools/call on its derived name fails as an unknown tool.
// Existing annotations are preserved, and a nil cmd is a no-op.
//
// Use cobra.Command.Hidden instead to remove a command and its whole subtree
// from both MCP and help.
func Exclude(cmd *cobra.Command) {
	internalmcp.MarkExcluded(cmd)
}
