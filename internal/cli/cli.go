package cli

import (
	"strconv"

	"github.com/spf13/cobra"
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
