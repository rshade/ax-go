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

// WithNonDeterministicFields registers cmd as emitting the standard success
// envelope for T. It reflects T once and records its ax:"nondeterministic"
// fields for __schema output; a nil command is ignored.
func WithNonDeterministicFields[T any](cmd *cobra.Command) {
	isolatedschema.WithNonDeterministicFields[T](cmd)
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

// BuildMCPSchema adapts the command tree to the MCP adapter shape: tools plus
// declared prompts and resources. It cannot fail and keeps the first of a
// duplicated prompt name or resource URI; __schema --as=mcp fails closed.
func BuildMCPSchema(root *cobra.Command) MCPSchema {
	return isolatedschema.BuildMCPSchema(root)
}

// Prompt is a static workflow template declared on a command with
// DeclarePrompt and projected into __schema.
type Prompt = isolatedschema.Prompt

// PromptArgument is one declared input of a Prompt.
type PromptArgument = isolatedschema.PromptArgument

// Resource is static, read-only reference metadata declared on a command with
// DeclareResource and projected into __schema.
type Resource = isolatedschema.Resource

// MCPPrompt is a Prompt in the __schema --as=mcp adapter shape.
type MCPPrompt = isolatedschema.MCPPrompt

// MCPPromptArgument is a PromptArgument in the MCP adapter shape.
type MCPPromptArgument = isolatedschema.MCPPromptArgument

// MCPResource is a Resource in the MCP resources/list shape.
type MCPResource = isolatedschema.MCPResource

// DeclarePrompt validates prompt and records a copy of it on cmd. It returns
// an *Error with error_code invalid_schema_declaration (exit 2) on a nil cmd,
// an invalid prompt, a name already declared on cmd, or a hand-written prompt
// annotation that does not decode cleanly, leaving cmd unchanged.
func DeclarePrompt(cmd *cobra.Command, prompt Prompt) error {
	return isolatedschema.DeclarePrompt(cmd, prompt)
}

// DeclareResource validates resource and records a copy of it on cmd, with the
// same error contract as DeclarePrompt; resources are keyed by URI.
func DeclareResource(cmd *cobra.Command, resource Resource) error {
	return isolatedschema.DeclareResource(cmd, resource)
}
