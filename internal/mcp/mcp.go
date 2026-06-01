package mcp

import (
	"github.com/spf13/cobra"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

// Schema is the internal MCP-compatible adapter shape.
type Schema struct {
	Tools []Tool
}

// Tool describes one command as an MCP-compatible tool.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Build adapts a Cobra command tree to MCP-compatible tool metadata.
func Build(root *cobra.Command) Schema {
	var tools []Tool
	internalschema.WalkCommands(root, func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		tools = append(tools, Tool{
			Name:        cmd.CommandPath(),
			Description: cmd.Short,
			InputSchema: map[string]any{
				"type":       "object",
				"properties": flagProperties(cmd),
			},
		})
	})
	return Schema{Tools: tools}
}

func flagProperties(cmd *cobra.Command) map[string]any {
	properties := map[string]any{}
	for _, flag := range internalschema.CollectFlags(cmd) {
		properties[flag.Name] = map[string]any{
			"type":        jsonSchemaType(flag.Type),
			"description": flag.Usage,
			"default":     flag.Default,
		}
	}
	return properties
}

func jsonSchemaType(flagType string) string {
	switch flagType {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "integer"
	case "float32", "float64":
		return "number"
	default:
		return "string"
	}
}
