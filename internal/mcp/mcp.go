package mcp

import (
	"strconv"

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
				jsonSchemaTypeKey:    jsonSchemaObject,
				jsonSchemaProperties: flagProperties(cmd),
			},
		})
	})
	return Schema{Tools: tools}
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
	return map[string]any{
		jsonSchemaTypeKey: jsonSchemaType(flag.Value.Type()),
		"description":     flag.Usage,
		"default":         flag.DefValue,
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
