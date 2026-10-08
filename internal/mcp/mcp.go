package mcp

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

const (
	jsonSchemaTypeKey    = "type"
	jsonSchemaBoolean    = internalschema.JSONBoolean
	jsonSchemaInteger    = internalschema.JSONInteger
	jsonSchemaNumber     = internalschema.JSONNumber
	jsonSchemaString     = internalschema.JSONString
	jsonSchemaObject     = "object"
	jsonSchemaArray      = "array"
	jsonSchemaProperties = "properties"
)

// Reserved command names never exposed as MCP tools on either the static
// (--as=mcp) or live (mcp-server) path: __schema and mcp-server are ax-go
// infrastructure, and Cobra's completion and help commands are shell
// ergonomics that return prose, not a machine payload. schemaCommandName and
// serverCommandName mirror internal/mcpserver's names (it re-exports
// serverCommandName as ServerCommandName); internal/mcp cannot import
// internal/mcpserver without an import cycle, so the literals are duplicated
// here and must stay in sync.
const (
	schemaCommandName     = "__schema"
	serverCommandName     = "mcp-server"
	completionCommandName = "completion"
	helpCommandName       = "help"
)

// ExcludeAnnotationKey is the Cobra annotation that removes a single command
// from the MCP tool set on both the static and live paths. Only the exact
// canonical value excludeAnnotationValue excludes; any other value is ignored
// so a typo can never silently drop a tool.
const (
	ExcludeAnnotationKey   = "github.com/rshade/ax-go/mcp/exclude"
	excludeAnnotationValue = "true"
)

// Schema is the internal MCP-compatible adapter shape. Prompts and Resources
// are static declarations aggregated over internalschema.WalkDeclarationCommands
// (the root plus every command not under a hidden child); they are nil when
// nothing is declared.
type Schema struct {
	Tools     []Tool
	Prompts   []internalschema.Prompt
	Resources []internalschema.Resource
}

// Tool describes one command as an MCP-compatible tool.
type Tool struct {
	Name                   string
	Description            string
	InputSchema            map[string]any
	NonDeterministicFields []string
	// CapabilityClass and CapabilityNote are the command's declared side-effect
	// class and note; both are empty when the command is unclassified.
	CapabilityClass string
	CapabilityNote  string
	// Hints are the standard MCP tool annotations derived from
	// CapabilityClass; nil when the command is unclassified.
	Hints *Hints
}

// Hints are the standard MCP tool-annotation hints derived from a capability
// class. A nil pointer means the hint is omitted, which MCP clients read as
// the spec default.
type Hints struct {
	ReadOnly    bool
	Destructive *bool
	OpenWorld   *bool
}

// Build adapts a Cobra command tree to MCP-compatible tool metadata using the
// WalkCallableCommands rules, so the static --as=mcp adapter and the live
// mcp-server advertise the same tool set. Prompts and resources come from
// internalschema.CollectDeclarations instead: exclusion and runnability govern
// tools only, so a group or excluded command may still carry declarations.
func Build(root *cobra.Command) Schema {
	var tools []Tool
	WalkCallableCommands(root, func(cmd *cobra.Command) {
		tools = append(tools, BuildTool(cmd))
	})
	prompts, resources := internalschema.CollectDeclarations(root)
	return Schema{Tools: tools, Prompts: prompts, Resources: resources}
}

// BuildTool describes a single command as an MCP-compatible tool: ToolName for
// the name, the command's Short for the description, and its flags as the
// input schema.
func BuildTool(cmd *cobra.Command) Tool {
	tool := Tool{
		Name:                   ToolName(cmd),
		Description:            cmd.Short,
		InputSchema:            inputSchema(cmd),
		NonDeterministicFields: internalschema.NonDeterministicFields(cmd.Annotations),
	}
	if class, note, ok := internalschema.CommandCapability(cmd.Annotations); ok {
		tool.CapabilityClass = class
		tool.CapabilityNote = note
		tool.Hints = capabilityHints(class)
	}
	return tool
}

// capabilityHints maps a capability class to MCP hints conservatively: any
// class that may overwrite or remove state is destructive, create is
// explicitly not, read-only is the only class claiming readOnlyHint, and
// external-network only sets openWorldHint because it says nothing about state.
// idempotentHint is never set; ax-go cannot know it.
func capabilityHints(class string) *Hints {
	yes, no := true, false
	switch class {
	case "read-only":
		return &Hints{ReadOnly: true}
	case "create":
		return &Hints{Destructive: &no}
	case "mutate", "delete", "admin":
		return &Hints{Destructive: &yes}
	case "external-network":
		return &Hints{OpenWorld: &yes}
	default:
		return nil
	}
}

// ToolName returns the MCP tool name for cmd: its Cobra command path with
// segments joined by "-", so the name always matches the MCP tool-name rule
// ^[a-zA-Z0-9_.-]+$ (the space-joined command path does not).
func ToolName(cmd *cobra.Command) string {
	return strings.Join(strings.Fields(cmd.CommandPath()), "-")
}

// WalkCallableCommands visits cmd and every descendant that may surface as an
// MCP tool. A hidden command prunes its whole subtree (matching
// internal/schema.BuildCommand's documented pruning), as does a reserved
// command (__schema, mcp-server, completion, help). A command that is not
// runnable (a pure group with neither Run nor RunE) or that carries the
// exclusion annotation is skipped on its own, and its children are still
// walked.
func WalkCallableCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	if cmd.Hidden || isReservedCommand(cmd.Name()) {
		return
	}
	if cmd.Runnable() && !IsExcluded(cmd) {
		visit(cmd)
	}
	for _, child := range cmd.Commands() {
		WalkCallableCommands(child, visit)
	}
}

// IsExcluded reports whether cmd carries ExcludeAnnotationKey with its
// canonical value. A nil command is never excluded.
func IsExcluded(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations[ExcludeAnnotationKey] == excludeAnnotationValue
}

// MarkExcluded sets ExcludeAnnotationKey on cmd, allocating its annotation map
// if needed and leaving other annotations intact. It is a no-op on nil.
func MarkExcluded(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[ExcludeAnnotationKey] = excludeAnnotationValue
}

// isReservedCommand reports whether name is a reserved command that must never
// surface as an MCP tool on either the static or live path.
func isReservedCommand(name string) bool {
	switch name {
	case schemaCommandName, serverCommandName, completionCommandName, helpCommandName:
		return true
	default:
		return false
	}
}

// inputSchema builds the JSON Schema object for cmd's flags: every flag is a
// property, and required flags (Cobra's required annotation) are listed in the
// required array so MCP clients can tell mandatory arguments from optional
// ones.
func inputSchema(cmd *cobra.Command) map[string]any {
	schema := map[string]any{
		jsonSchemaTypeKey:    jsonSchemaObject,
		jsonSchemaProperties: flagProperties(cmd),
	}
	if required := requiredFlags(cmd); len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func flagProperties(cmd *cobra.Command) map[string]any {
	properties := map[string]any{}
	for _, flag := range collectFlags(cmd) {
		properties[flag.Name] = flagProperty(flag)
	}
	return properties
}

func collectFlags(cmd *cobra.Command) []*pflag.Flag {
	seen := map[string]struct{}{}
	var flags []*pflag.Flag

	add := func(flag *pflag.Flag) {
		if _, ok := seen[flag.Name]; ok {
			return
		}
		seen[flag.Name] = struct{}{}
		flags = append(flags, flag)
	}

	cmd.NonInheritedFlags().VisitAll(add)
	cmd.InheritedFlags().VisitAll(add)
	return flags
}

// requiredFlags returns the sorted names of cmd's flags marked required (via
// cobra.MarkFlagRequired). Sorted order keeps the emitted schema deterministic.
func requiredFlags(cmd *cobra.Command) []string {
	var required []string
	for _, flag := range collectFlags(cmd) {
		if internalschema.IsRequiredFlag(flag) {
			required = append(required, flag.Name)
		}
	}
	slices.Sort(required)
	return required
}

func flagProperty(flag *pflag.Flag) map[string]any {
	flagType := flag.Value.Type()
	if itemType, ok := internalschema.JSONSchemaArrayItemType(flagType); ok {
		property := map[string]any{
			jsonSchemaTypeKey: jsonSchemaArray,
			"description":     flag.Usage,
			"items":           map[string]any{jsonSchemaTypeKey: itemType},
		}
		if value, hasDefault := jsonSchemaArrayDefault(flag, itemType); hasDefault {
			property["default"] = value
		}
		addExamples(property, flagType, flag)
		return property
	}
	property := map[string]any{
		jsonSchemaTypeKey: internalschema.JSONSchemaType(flagType),
		"description":     flag.Usage,
	}
	if value, hasDefault := internalschema.ScalarJSON(flagType, flag.DefValue); hasDefault {
		property["default"] = value
	}
	if enum, ok := enumJSON(flagType, internalschema.FlagEnum(flag)); ok {
		property["enum"] = enum
	}
	addExamples(property, flagType, flag)
	return property
}

// addExamples sets the JSON-Schema "examples" array (one typed element) when
// the flag declares an example that converts; otherwise the key is omitted.
func addExamples(property map[string]any, flagType string, flag *pflag.Flag) {
	example := internalschema.FlagExample(flag)
	if example == "" {
		return
	}
	if value, ok := internalschema.ExampleJSON(flagType, example); ok {
		property["examples"] = []any{value}
	}
}

// enumJSON converts a declared enum to JSON values typed like the property, in
// author order. Integer members are canonicalised first ("03" becomes 3). ok is
// false when no enum is declared or any member fails to convert, so a partial
// set is never advertised.
func enumJSON(flagType string, allowed []string) ([]any, bool) {
	if len(allowed) == 0 {
		return nil, false
	}
	values := make([]any, 0, len(allowed))
	for _, member := range allowed {
		if internalschema.JSONSchemaType(flagType) == jsonSchemaString {
			values = append(values, member)
			continue
		}
		canonical, err := internalschema.CanonicalEnumValue(flagType, member)
		converted, ok := internalschema.ScalarJSON(flagType, canonical)
		if err != nil || !ok {
			return nil, false
		}
		values = append(values, converted)
	}
	return values, true
}

func jsonSchemaArrayDefault(flag *pflag.Flag, itemType string) ([]any, bool) {
	slice, ok := flag.Value.(pflag.SliceValue)
	if !ok {
		return nil, false
	}
	source := slice.GetSlice()
	values := make([]any, 0, len(source))
	for _, value := range source {
		converted, convertedOK := internalschema.ArrayItemJSON(value, itemType, flag.Value.Type())
		if !convertedOK {
			return nil, false
		}
		values = append(values, converted)
	}
	return values, true
}
