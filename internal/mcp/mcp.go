package mcp

import (
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

const (
	jsonSchemaTypeKey    = "type"
	jsonSchemaBoolean    = "boolean"
	jsonSchemaInteger    = "integer"
	jsonSchemaNumber     = "number"
	jsonSchemaString     = "string"
	jsonSchemaObject     = "object"
	jsonSchemaArray      = "array"
	jsonSchemaProperties = "properties"
)

// Reserved command names never exposed as MCP tools on either the static
// (--as=mcp) or live (mcp-server) path: __schema and mcp-server are ax-go
// infrastructure, and Cobra's completion tree is shell ergonomics an MCP
// client cannot use. schemaCommandName and serverCommandName mirror
// internal/mcpserver's names (it re-exports serverCommandName as
// ServerCommandName); internal/mcp cannot import internal/mcpserver without an
// import cycle, so the literals are duplicated here and must stay in sync.
const (
	schemaCommandName     = "__schema"
	serverCommandName     = "mcp-server"
	completionCommandName = "completion"
)

// Schema is the internal MCP-compatible adapter shape.
type Schema struct {
	Tools []Tool
}

// Tool describes one command as an MCP-compatible tool.
type Tool struct {
	Name                   string
	Description            string
	InputSchema            map[string]any
	NonDeterministicFields []string
}

// Build adapts a Cobra command tree to MCP-compatible tool metadata. Hidden
// subtrees are pruned wholesale and the reserved __schema, mcp-server, and
// completion commands are excluded, so the static --as=mcp adapter and the live
// mcp-server advertise the same tool set.
func Build(root *cobra.Command) Schema {
	var tools []Tool
	WalkCallableCommands(root, func(cmd *cobra.Command) {
		tools = append(tools, BuildTool(cmd))
	})
	return Schema{Tools: tools}
}

// BuildTool describes a single command as an MCP-compatible tool: ToolName for
// the name, the command's Short for the description, and its flags as the
// input schema.
func BuildTool(cmd *cobra.Command) Tool {
	return Tool{
		Name:                   ToolName(cmd),
		Description:            cmd.Short,
		InputSchema:            inputSchema(cmd),
		NonDeterministicFields: internalschema.NonDeterministicFields(cmd.Annotations),
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
// internal/schema.BuildCommand's documented pruning), and reserved
// infrastructure commands (__schema, mcp-server, completion) are skipped.
func WalkCallableCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	if cmd.Hidden || isReservedCommand(cmd.Name()) {
		return
	}
	visit(cmd)
	for _, child := range cmd.Commands() {
		WalkCallableCommands(child, visit)
	}
}

// isReservedCommand reports whether name is a reserved infrastructure command
// that must never surface as an MCP tool on either the static or live path.
func isReservedCommand(name string) bool {
	switch name {
	case schemaCommandName, serverCommandName, completionCommandName:
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
	if itemType, ok := jsonSchemaArrayItemType(flag.Value.Type()); ok {
		property := map[string]any{
			jsonSchemaTypeKey: jsonSchemaArray,
			"description":     flag.Usage,
			"items":           map[string]any{jsonSchemaTypeKey: itemType},
		}
		if value, hasDefault := jsonSchemaArrayDefault(flag, itemType); hasDefault {
			property["default"] = value
		}
		return property
	}
	property := map[string]any{
		jsonSchemaTypeKey: jsonSchemaType(flag.Value.Type()),
		"description":     flag.Usage,
	}
	if value, hasDefault := jsonSchemaScalarDefault(flag); hasDefault {
		property["default"] = value
	}
	return property
}

// jsonSchemaScalarDefault converts a scalar flag's DefValue to the JSON type
// matching its schema type: booleans stay booleans, ints/uints and floats stay
// numbers, strings stay strings. It reports hasDefault=false when DefValue is
// empty (no default is emitted) or does not parse as the declared type, so the
// schema never advertises a string default for a non-string type (e.g. the
// boolean default "false", which is invalid JSON Schema).
func jsonSchemaScalarDefault(flag *pflag.Flag) (any, bool) {
	if flag.DefValue == "" {
		return nil, false
	}
	switch jsonSchemaType(flag.Value.Type()) {
	case jsonSchemaBoolean:
		parsed, err := strconv.ParseBool(flag.DefValue)
		return parsed, err == nil
	case jsonSchemaInteger:
		if strings.HasPrefix(flag.Value.Type(), "uint") {
			parsed, err := strconv.ParseUint(flag.DefValue, 10, 64)
			return parsed, err == nil
		}
		parsed, err := strconv.ParseInt(flag.DefValue, 10, 64)
		return parsed, err == nil
	case jsonSchemaNumber:
		parsed, err := strconv.ParseFloat(flag.DefValue, 64)
		return parsed, err == nil
	default:
		return flag.DefValue, true
	}
}

func jsonSchemaType(flagType string) string {
	switch flagType {
	case "bool":
		return jsonSchemaBoolean
	case "count", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return jsonSchemaInteger
	case "float32", "float64":
		return jsonSchemaNumber
	default:
		return jsonSchemaString
	}
}

func jsonSchemaArrayItemType(flagType string) (string, bool) {
	switch flagType {
	case "boolSlice":
		return jsonSchemaBoolean, true
	case "intSlice", "int32Slice", "int64Slice", "uintSlice":
		return jsonSchemaInteger, true
	case "float32Slice", "float64Slice":
		return jsonSchemaNumber, true
	case "durationSlice", "ipSlice", "ipNetSlice", "stringArray", "stringSlice":
		return jsonSchemaString, true
	default:
		return "", false
	}
}

func jsonSchemaArrayDefault(flag *pflag.Flag, itemType string) ([]any, bool) {
	slice, ok := flag.Value.(pflag.SliceValue)
	if !ok {
		return nil, false
	}
	source := slice.GetSlice()
	values := make([]any, 0, len(source))
	for _, value := range source {
		converted, convertedOK := convertArrayDefault(value, itemType, flag.Value.Type())
		if !convertedOK {
			return nil, false
		}
		values = append(values, converted)
	}
	return values, true
}

func convertArrayDefault(value, itemType, flagType string) (any, bool) {
	switch itemType {
	case jsonSchemaBoolean:
		parsed, err := strconv.ParseBool(value)
		return parsed, err == nil
	case jsonSchemaInteger:
		if flagType == "uintSlice" {
			parsed, err := strconv.ParseUint(value, 10, 64)
			return parsed, err == nil
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		return parsed, err == nil
	case jsonSchemaNumber:
		parsed, err := strconv.ParseFloat(value, 64)
		return parsed, err == nil
	default:
		return value, true
	}
}
