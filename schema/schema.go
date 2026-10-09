package schema

import (
	"fmt"
	"reflect"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/mcp"
	internalschema "github.com/rshade/ax-go/internal/schema"
)

// SchemaVersion is the current SemVer version for ax-native schemas.
const SchemaVersion = contract.ErrorSchemaVersion

const (
	schemaCommandName = "__schema"
	traceIDField      = "trace_id"
)

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
	// KnownCodes lists, sorted, every error_code ax-go itself can return from
	// a command run, including from helper packages such as config. It is not
	// exhaustive for the CLI: the adopting CLI's own codes are not listed.
	// Authoring-time codes (invalid_schema_declaration) and this repository's
	// gate-tool codes are excluded. contract.KnownErrorCodes is the source and
	// states the full scope rule.
	KnownCodes             []string `json:"known_codes"`
	NonDeterministicFields []string `json:"non_deterministic_fields"`
}

// CommandSchema describes a Cobra command and its direct children.
type CommandSchema struct {
	Use       string          `json:"use"`
	Short     string          `json:"short,omitempty"`
	Long      string          `json:"long,omitempty"`
	Example   string          `json:"example,omitempty"`
	Flags     []FlagSchema    `json:"flags,omitempty"`
	Commands  []CommandSchema `json:"commands,omitempty"`
	Prompts   []Prompt        `json:"prompts,omitempty"`
	Resources []Resource      `json:"resources,omitempty"`
	// Capability is the command's declared side-effect class (see
	// DeclareCapability). It is nil, and omitted, when the command is
	// unclassified; an agent must not assume any class for it.
	Capability             *CapabilitySchema `json:"capability,omitempty"`
	NonDeterministicFields []string          `json:"non_deterministic_fields"`
}

// CapabilitySchema is a command's declared side-effect class and its optional
// free-form note. Class is always one of the six Capability constants; Note is
// descriptive only and is omitted when empty.
type CapabilitySchema struct {
	Class Capability `json:"class"`
	Note  string     `json:"note,omitempty"`
}

// FlagSchema describes a command flag.
type FlagSchema struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage,omitempty"`
	Required  bool   `json:"required,omitempty"`
	// Enum lists the only values the flag accepts, in CLI string form and the
	// author's order (see DeclareFlagEnum). It is absent when none is declared.
	Enum []string `json:"enum,omitempty"`
	// Example is one value in CLI form, exactly as an agent would pass it on
	// argv (see DeclareFlagExample). It is absent when none is declared.
	Example string `json:"example,omitempty"`
}

// Option configures BuildSchema and NewSchemaCommand.
type Option func(*options)

type options struct {
	version string
}

// WithSchemaVersion sets the tool version reported by __schema.
func WithSchemaVersion(version string) Option {
	return func(cfg *options) {
		cfg.version = version
	}
}

// WithNonDeterministicFields registers cmd as emitting the standard success
// envelope for T. It adds the built-in meta.* locators and records exported
// fields of T marked ax:"nondeterministic" as data.* locators. Reflection runs
// once at registration time; a nil command is ignored.
func WithNonDeterministicFields[T any](cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	internalschema.RegisterEnvelope(cmd, internalschema.DataLocators(reflect.TypeFor[T]()))
}

// BuildSchema reflects a Cobra command tree into the ax-native schema. It
// cannot fail: when a prompt name or resource URI is declared on more than one
// command it keeps the first in walk order, and it omits a hand-corrupted
// declaration annotation. The __schema command (NewSchemaCommand) is where
// both cases fail closed with validation_error (exit 2).
func BuildSchema(root *cobra.Command, opts ...Option) Schema {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}

	return Schema{
		SchemaVersion: SchemaVersion,
		Tool:          root.Name(),
		Version:       cfg.version,
		ModeDetection: contract.ModeDetectionRule,
		Command:       convertCommandSchema(internalschema.BuildCommand(root), internalschema.NewDeduper()),
		ErrorEnvelope: ErrorSchemaInfo{
			SchemaVersion: contract.ErrorSchemaVersion,
			Required: []string{
				"error_code",
				"message",
				traceIDField,
				"tool",
				"version",
				"schema_version",
			},
			Optional: []string{
				"actionable_fix",
				"context",
				"suggestions",
			},
			KnownCodes:             contract.KnownErrorCodes(),
			NonDeterministicFields: []string{traceIDField},
		},
	}
}

// NewSchemaCommand builds the reserved __schema command.
func NewSchemaCommand(root *cobra.Command, opts ...Option) *cobra.Command {
	var as string
	cmd := &cobra.Command{
		Use:   schemaCommandName,
		Short: "Emit the AX machine-discoverability schema",
		Example: fmt.Sprintf("  %s __schema\n  %s __schema --as=mcp",
			root.Name(), root.Name()),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if conflict := internalschema.FindCorrupt(root); conflict != nil {
				return internalschema.TreeDeclarationError(cmd.Context(), conflict)
			}
			if conflict := internalschema.FindDuplicate(root); conflict != nil {
				return internalschema.TreeDeclarationError(cmd.Context(), conflict)
			}
			switch as {
			case "", "ax":
				return contract.WriteJSON(cmd.OutOrStdout(), BuildSchema(root, opts...))
			case "mcp":
				return contract.WriteJSON(cmd.OutOrStdout(), BuildMCPSchema(root))
			default:
				return contract.NewError(
					cmd.Context(),
					"validation_error",
					fmt.Sprintf("unknown schema format %q", as),
					contract.WithErrorExitCode(contract.ExitValidation),
				)
			}
		},
	}
	cmd.Flags().StringVar(&as, "as", "ax", "schema format: ax or mcp")
	return cmd
}

// MCPSchema is the lightweight MCP-compatible adapter shape. Prompts and
// Resources aggregate the declarations of the root and every command not under
// a hidden child, in pre-order walk order then declaration order, and are
// omitted when none exist.
type MCPSchema struct {
	Tools     []MCPTool     `json:"tools"`
	Prompts   []MCPPrompt   `json:"prompts,omitempty"`
	Resources []MCPResource `json:"resources,omitempty"`
}

// MCPTool describes one command as an MCP-compatible tool.
type MCPTool struct {
	Name                   string         `json:"name"`
	Description            string         `json:"description,omitempty"`
	InputSchema            map[string]any `json:"inputSchema"`
	NonDeterministicFields []string       `json:"nonDeterministicFields"`
	// Capability is the ax-specific side-effect class; nil when unclassified.
	Capability *CapabilitySchema `json:"capability,omitempty"`
	// Annotations are the standard MCP tool hints derived from Capability, so a
	// generic MCP client can apply its own safety policy; nil when unclassified.
	Annotations *MCPToolAnnotations `json:"annotations,omitempty"`
}

// MCPToolAnnotations are the standard MCP tool-annotation hints derived from a
// command's capability class. The mapping is conservative: read-only sets
// ReadOnlyHint; create sets DestructiveHint false; mutate, delete and admin set
// DestructiveHint true; external-network sets only OpenWorldHint true. A nil
// pointer is omitted and means the MCP default.
type MCPToolAnnotations struct {
	ReadOnlyHint    bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	OpenWorldHint   *bool `json:"openWorldHint,omitempty"`
}

// BuildMCPSchema adapts the command tree to the MCP adapter shape: tools plus
// declared prompts and resources. Like BuildSchema it cannot fail, keeping the
// first of a duplicated key; __schema --as=mcp is where duplicates and corrupt
// annotations fail closed.
func BuildMCPSchema(root *cobra.Command) MCPSchema {
	mcpSchema := mcp.Build(root)
	tools := make([]MCPTool, 0, len(mcpSchema.Tools))
	for _, tool := range mcpSchema.Tools {
		tools = append(tools, MCPTool{
			Name:                   tool.Name,
			Description:            tool.Description,
			InputSchema:            tool.InputSchema,
			NonDeterministicFields: tool.NonDeterministicFields,
			Capability:             capabilitySchema(tool.CapabilityClass, tool.CapabilityNote),
			Annotations:            toolAnnotations(tool.Hints),
		})
	}
	return MCPSchema{
		Tools:     tools,
		Prompts:   convertSlice(mcpSchema.Prompts, toMCPPrompt),
		Resources: convertSlice(mcpSchema.Resources, toMCPResource),
	}
}

// convertCommandSchema converts command pre-order, the order
// internalschema.WalkDeclarationCommands visits, so dedup keeps the same first
// declaration of a key that BuildMCPSchema keeps.
func convertCommandSchema(command internalschema.Command, dedup *internalschema.Deduper) CommandSchema {
	schema := CommandSchema{
		Use:      command.Use,
		Short:    command.Short,
		Long:     command.Long,
		Example:  command.Example,
		Flags:    convertFlagSchemas(command.Flags),
		Commands: make([]CommandSchema, 0, len(command.Commands)),
		Prompts: convertSlice(
			dedup.FirstPrompts(internalschema.Prompts(command.Annotations)),
			fromInternalPrompt,
		),
		Resources: convertSlice(
			dedup.FirstResources(internalschema.Resources(command.Annotations)),
			fromInternalResource,
		),
		NonDeterministicFields: internalschema.NonDeterministicFields(command.Annotations),
	}
	if class, note, ok := internalschema.CommandCapability(command.Annotations); ok {
		schema.Capability = capabilitySchema(class, note)
	}

	for _, child := range command.Commands {
		schema.Commands = append(schema.Commands, convertCommandSchema(child, dedup))
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
			Enum:      flag.Enum,
			Example:   flag.Example,
		})
	}

	return flags
}

func capabilitySchema(class, note string) *CapabilitySchema {
	if class == "" {
		return nil
	}
	return &CapabilitySchema{Class: Capability(class), Note: note}
}

func toolAnnotations(hints *mcp.Hints) *MCPToolAnnotations {
	if hints == nil {
		return nil
	}
	return &MCPToolAnnotations{
		ReadOnlyHint:    hints.ReadOnly,
		DestructiveHint: hints.Destructive,
		OpenWorldHint:   hints.OpenWorld,
	}
}
