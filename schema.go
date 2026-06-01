package ax

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/mcp"
	internalschema "github.com/rshade/ax-go/internal/schema"
)

// SchemaVersion is the current SemVer version for ax-native schemas.
const SchemaVersion = ErrorSchemaVersion

// Schema is the ax-native reflective JSON tree emitted by __schema.
type Schema struct {
	SchemaVersion string          `json:"schema_version"`
	Tool          string          `json:"tool"`
	Version       string          `json:"version"`
	ModeDetection string          `json:"mode_detection"`
	Command       CommandSchema   `json:"command"`
	ErrorEnvelope ErrorSchemaInfo `json:"error_envelope"`
}

// ErrorSchemaInfo describes the shared stderr error envelope.
type ErrorSchemaInfo struct {
	SchemaVersion string   `json:"schema_version"`
	Required      []string `json:"required"`
	Optional      []string `json:"optional"`
}

// CommandSchema describes a Cobra command and its direct children.
type CommandSchema struct {
	Use      string          `json:"use"`
	Short    string          `json:"short,omitempty"`
	Long     string          `json:"long,omitempty"`
	Example  string          `json:"example,omitempty"`
	Flags    []FlagSchema    `json:"flags,omitempty"`
	Commands []CommandSchema `json:"commands,omitempty"`
}

// FlagSchema describes a command flag.
type FlagSchema struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

// SchemaOption configures BuildSchema and NewSchemaCommand.
type SchemaOption func(*schemaConfig)

type schemaConfig struct {
	version string
}

// WithSchemaVersion sets the tool version reported by __schema.
func WithSchemaVersion(version string) SchemaOption {
	return func(cfg *schemaConfig) {
		cfg.version = version
	}
}

// BuildSchema reflects a Cobra command tree into the ax-native schema.
func BuildSchema(root *cobra.Command, opts ...SchemaOption) Schema {
	cfg := schemaConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	return Schema{
		SchemaVersion: SchemaVersion,
		Tool:          root.Name(),
		Version:       cfg.version,
		ModeDetection: ModeDetectionRule,
		Command:       convertCommandSchema(internalschema.BuildCommand(root)),
		ErrorEnvelope: ErrorSchemaInfo{
			SchemaVersion: ErrorSchemaVersion,
			Required: []string{
				"error_code",
				"message",
				"trace_id",
				"tool",
				"version",
				"schema_version",
			},
			Optional: []string{
				"actionable_fix",
				"context",
				"suggestions",
			},
		},
	}
}

// NewSchemaCommand builds the reserved __schema command.
func NewSchemaCommand(root *cobra.Command, opts ...SchemaOption) *cobra.Command {
	var as string
	cmd := &cobra.Command{
		Use:   "__schema",
		Short: "Emit the AX machine-discoverability schema",
		Example: `  app __schema
  app __schema --as=mcp`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch as {
			case "", "ax":
				return WriteJSON(cmd.OutOrStdout(), BuildSchema(root, opts...))
			case "mcp":
				return WriteJSON(cmd.OutOrStdout(), BuildMCPSchema(root))
			default:
				return NewError(
					cmd.Context(),
					"validation_error",
					fmt.Sprintf("unknown schema format %q", as),
					WithErrorExitCode(ExitValidation),
				)
			}
		},
	}
	cmd.Flags().StringVar(&as, "as", "ax", "schema format: ax or mcp")
	return cmd
}

// MCPSchema is the lightweight MCP-compatible adapter shape.
type MCPSchema struct {
	Tools []MCPTool `json:"tools"`
}

// MCPTool describes one command as an MCP-compatible tool.
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// BuildMCPSchema adapts the command tree to a simple MCP tools list.
func BuildMCPSchema(root *cobra.Command) MCPSchema {
	schema := mcp.Build(root)
	tools := make([]MCPTool, 0, len(schema.Tools))
	for _, tool := range schema.Tools {
		tools = append(tools, MCPTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return MCPSchema{Tools: tools}
}

func convertCommandSchema(command internalschema.Command) CommandSchema {
	schema := CommandSchema{
		Use:      command.Use,
		Short:    command.Short,
		Long:     command.Long,
		Example:  command.Example,
		Flags:    convertFlagSchemas(command.Flags),
		Commands: make([]CommandSchema, 0, len(command.Commands)),
	}

	for _, child := range command.Commands {
		schema.Commands = append(schema.Commands, convertCommandSchema(child))
	}

	return schema
}

func convertFlagSchemas(source []internalschema.Flag) []FlagSchema {
	flags := make([]FlagSchema, 0, len(source))
	for _, flag := range source {
		flags = append(flags, FlagSchema{
			Name:      flag.Name,
			Shorthand: flag.Shorthand,
			Type:      flag.Type,
			Default:   flag.Default,
			Usage:     flag.Usage,
			Required:  flag.Required,
		})
	}

	return flags
}
