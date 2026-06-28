package cli

import (
	"strconv"

	"github.com/spf13/cobra"
)

// Persistent agent-safety/output flag names installed on every ax-go root
// command. They live here, in the package both ax.Execute and the MCP
// dispatcher already depend on, so the engine's flag installer and the MCP
// server resolve the same flags from a single source of truth.
const (
	// FlagFormat selects the output mode (json or human).
	FlagFormat = "format"
	// FlagDryRun emits the envelope without performing side effects.
	FlagDryRun = "dry-run"
	// FlagIdempotencyKey carries an opaque key that suppresses duplicate-create
	// retries.
	FlagIdempotencyKey = "idempotency-key"
)

// EnsurePersistentStringFlag adds a persistent string flag unless it exists.
func EnsurePersistentStringFlag(root *cobra.Command, name, value, usage string) {
	if root.PersistentFlags().Lookup(name) != nil {
		return
	}
	root.PersistentFlags().String(name, value, usage)
}

// EnsurePersistentBoolFlag adds a persistent bool flag unless it exists.
func EnsurePersistentBoolFlag(root *cobra.Command, name string, value bool, usage string) {
	if root.PersistentFlags().Lookup(name) != nil {
		return
	}
	root.PersistentFlags().Bool(name, value, usage)
}

// LookupFlagString resolves a local or inherited flag value.
func LookupFlagString(cmd *cobra.Command, name string) string {
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag.Value.String()
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil {
		return flag.Value.String()
	}
	return ""
}

// LookupFlagBool resolves a local or inherited bool flag value.
func LookupFlagBool(cmd *cobra.Command, name string) bool {
	value := LookupFlagString(cmd, name)
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}
