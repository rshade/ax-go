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

// ExcludeFlag marks the named flag on cmd so it is omitted from the MCP tool
// input schema (tools/list and "__schema --as=mcp") and rejected with a
// validation error (exit 2) on tools/call, for example because the flag starts
// a watch loop or interactive session that cannot return over request/response.
// The flag is looked up among the flags cmd itself declares, in cmd.Flags(),
// then cmd.PersistentFlags(); a persistent flag excluded on a parent command
// is also excluded on every descendant command that inherits it without
// shadowing it. A name cmd only inherits is a no-op: exclude it on the command
// that declares it. A flag that is both required and excluded makes every
// tools/call on the tool fail with a validation error (exit 2) naming the
// flag, because a client can never supply it. The flag stays visible in
// --help and in "__schema --as=ax". The mark is the pflag annotation
// "github.com/rshade/ax-go/mcp/exclude" set to []string{"true"}.
//
// The ax agent-safety and output flags (format, dry-run, yes,
// idempotency-key, strict) are reserved: they stay available to MCP clients,
// ExcludeFlag ignores them, and a hand-set annotation on them has no effect.
// Existing flag annotations are preserved, and a nil cmd, empty name, or
// unknown flag name is a no-op.
func ExcludeFlag(cmd *cobra.Command, name string) {
	internalmcp.MarkFlagExcluded(cmd, name)
}
