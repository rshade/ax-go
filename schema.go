package ax

import (
	"github.com/spf13/cobra"

	isolatedschema "github.com/rshade/ax-go/schema"
)

// SchemaVersion is the current SemVer version for ax-native schemas.
const SchemaVersion = isolatedschema.SchemaVersion

// schemaCommandName is the reserved machine-discoverability command name every
// ax-go CLI exposes (Principle III).
const schemaCommandName = "__schema"

// Schema is the ax-native reflective JSON tree emitted by __schema.
type Schema = isolatedschema.Schema

// ErrorSchemaInfo describes the shared stderr error envelope.
type ErrorSchemaInfo = isolatedschema.ErrorSchemaInfo

// CommandSchema describes a Cobra command and its direct children.
type CommandSchema = isolatedschema.CommandSchema

// FlagSchema describes a command flag.
type FlagSchema = isolatedschema.FlagSchema

// SchemaOption configures BuildSchema and NewSchemaCommand.
type SchemaOption = isolatedschema.Option

// WithSchemaVersion sets the tool version reported by __schema.
func WithSchemaVersion(version string) SchemaOption {
	return isolatedschema.WithSchemaVersion(version)
}

// BuildSchema reflects a Cobra command tree into the ax-native schema.
func BuildSchema(root *cobra.Command, opts ...SchemaOption) Schema {
	return isolatedschema.BuildSchema(root, opts...)
}

// NewSchemaCommand builds the reserved __schema command.
func NewSchemaCommand(root *cobra.Command, opts ...SchemaOption) *cobra.Command {
	return isolatedschema.NewSchemaCommand(root, opts...)
}

// MCPSchema is the lightweight MCP-compatible adapter shape.
type MCPSchema = isolatedschema.MCPSchema

// MCPTool describes one command as an MCP-compatible tool.
type MCPTool = isolatedschema.MCPTool

// BuildMCPSchema adapts the command tree to a simple MCP tools list.
func BuildMCPSchema(root *cobra.Command) MCPSchema {
	return isolatedschema.BuildMCPSchema(root)
}
