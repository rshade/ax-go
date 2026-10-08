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

// DeclareFlagEnum declares the only values the named flag accepts; a value
// outside the set is rejected at parse time with validation_error and exit
// code 2, before PersistentPreRunE and RunE and so before any dry-run handling.
// An authoring mistake returns an *Error with error_code
// invalid_schema_declaration (exit 2), like DeclarePrompt. See
// schema.DeclareFlagEnum for the full contract.
func DeclareFlagEnum(cmd *cobra.Command, flag string, values ...string) error {
	return isolatedschema.DeclareFlagEnum(cmd, flag, values...)
}

// DeclareFlagExample attaches one CLI-form example value to the named flag. See
// schema.DeclareFlagExample for the full contract.
func DeclareFlagExample(cmd *cobra.Command, flag string, example string) error {
	return isolatedschema.DeclareFlagExample(cmd, flag, example)
}

// Capability is a command's side-effect class from ax-go's fixed vocabulary.
// See schema.Capability.
type Capability = isolatedschema.Capability

// The six capability classes; each equals its schema package counterpart.
const (
	// CapabilityReadOnly marks a command that observes state only.
	CapabilityReadOnly = isolatedschema.CapabilityReadOnly
	// CapabilityCreate marks a command that creates new state.
	CapabilityCreate = isolatedschema.CapabilityCreate
	// CapabilityMutate marks a command that changes existing state.
	CapabilityMutate = isolatedschema.CapabilityMutate
	// CapabilityDelete marks a command that removes state.
	CapabilityDelete = isolatedschema.CapabilityDelete
	// CapabilityExternalNetwork marks a command that reaches a network endpoint
	// outside the host.
	CapabilityExternalNetwork = isolatedschema.CapabilityExternalNetwork
	// CapabilityAdmin marks a privileged or administrative operation.
	CapabilityAdmin = isolatedschema.CapabilityAdmin
)

// CapabilitySchema is a command's declared side-effect class and optional note.
type CapabilitySchema = isolatedschema.CapabilitySchema

// MCPToolAnnotations are the standard MCP tool hints derived from a capability
// class.
type MCPToolAnnotations = isolatedschema.MCPToolAnnotations

// DeclareCapability classifies cmd's side effects with one class from the fixed
// vocabulary plus an optional note. See schema.DeclareCapability for the full
// contract.
func DeclareCapability(cmd *cobra.Command, class Capability, note string) error {
	return isolatedschema.DeclareCapability(cmd, class, note)
}
