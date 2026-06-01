package schema

import (
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Command is the internal Cobra command reflection shape.
type Command struct {
	Use      string
	Short    string
	Long     string
	Example  string
	Flags    []Flag
	Commands []Command
}

// Flag is the internal Cobra flag reflection shape.
type Flag struct {
	Name      string
	Shorthand string
	Type      string
	Default   string
	Usage     string
	Required  bool
}

// BuildCommand reflects a Cobra command tree into the internal schema.
func BuildCommand(cmd *cobra.Command) Command {
	schema := Command{
		Use:     cmd.Use,
		Short:   cmd.Short,
		Long:    cmd.Long,
		Example: cmd.Example,
		Flags:   CollectFlags(cmd),
	}

	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		schema.Commands = append(schema.Commands, BuildCommand(child))
	}

	return schema
}

// CollectFlags returns local and inherited flags without duplicates.
func CollectFlags(cmd *cobra.Command) []Flag {
	seen := map[string]struct{}{}
	var flags []Flag

	add := func(flag *pflag.Flag) {
		if _, ok := seen[flag.Name]; ok {
			return
		}
		seen[flag.Name] = struct{}{}
		flags = append(flags, Flag{
			Name:      flag.Name,
			Shorthand: flag.Shorthand,
			Type:      flag.Value.Type(),
			Default:   flag.DefValue,
			Usage:     flag.Usage,
			Required:  isRequiredFlag(flag),
		})
	}

	cmd.NonInheritedFlags().VisitAll(add)
	cmd.InheritedFlags().VisitAll(add)

	return flags
}

// WalkCommands visits cmd and every descendant.
func WalkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		WalkCommands(child, visit)
	}
}

func isRequiredFlag(flag *pflag.Flag) bool {
	return slices.Contains(flag.Annotations[cobra.BashCompOneRequiredFlag], "true")
}
