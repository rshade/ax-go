package mcp_test

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/mcp"
)

func newCmd(use string) *cobra.Command {
	return &cobra.Command{Use: use, Short: use + " short description"}
}

func TestBuild_EmptyRootNoFlags(t *testing.T) {
	root := newCmd("root")

	got := mcp.Build(root)

	want := mcp.Schema{Tools: []mcp.Tool{
		{
			Name:        "root",
			Description: "root short description",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build() = %#v, want %#v", got, want)
	}
}

func TestBuild_HiddenCommandExcluded(t *testing.T) {
	root := newCmd("root")
	visible := newCmd("visible")
	hidden := newCmd("hidden")
	hidden.Hidden = true
	root.AddCommand(visible, hidden)

	got := mcp.Build(root)

	var names []string
	for _, tool := range got.Tools {
		names = append(names, tool.Name)
	}
	want := []string{"root", "root visible"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("tool names = %v, want %v", names, want)
	}
}

func TestBuild_RecursiveTree(t *testing.T) {
	root := newCmd("root")
	child := newCmd("child")
	grandchild := newCmd("grandchild")
	child.AddCommand(grandchild)
	root.AddCommand(child)

	got := mcp.Build(root)

	var names []string
	for _, tool := range got.Tools {
		names = append(names, tool.Name)
	}
	want := []string{"root", "root child", "root child grandchild"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("tool names = %v, want %v", names, want)
	}
}

func propertiesOf(t *testing.T, schema mcp.Schema, toolName string) map[string]any {
	t.Helper()
	for _, tool := range schema.Tools {
		if tool.Name == toolName {
			props, ok := tool.InputSchema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("tool %q InputSchema properties is not a map[string]any: %#v",
					toolName, tool.InputSchema["properties"])
			}
			return props
		}
	}
	t.Fatalf("tool %q not found in schema %#v", toolName, schema)
	return nil
}

func TestBuild_ScalarFlagTypeMapping(t *testing.T) {
	tests := []struct {
		name     string
		register func(cmd *cobra.Command)
		flagName string
		wantType string
	}{
		{"bool", func(cmd *cobra.Command) { cmd.Flags().Bool("f", true, "usage") }, "f", "boolean"},
		{"count", func(cmd *cobra.Command) { cmd.Flags().Count("f", "usage") }, "f", "integer"},
		{"int", func(cmd *cobra.Command) { cmd.Flags().Int("f", 1, "usage") }, "f", "integer"},
		{"int8", func(cmd *cobra.Command) { cmd.Flags().Int8("f", 1, "usage") }, "f", "integer"},
		{"int16", func(cmd *cobra.Command) { cmd.Flags().Int16("f", 1, "usage") }, "f", "integer"},
		{"int32", func(cmd *cobra.Command) { cmd.Flags().Int32("f", 1, "usage") }, "f", "integer"},
		{"int64", func(cmd *cobra.Command) { cmd.Flags().Int64("f", 1, "usage") }, "f", "integer"},
		{"uint", func(cmd *cobra.Command) { cmd.Flags().Uint("f", 1, "usage") }, "f", "integer"},
		{"uint8", func(cmd *cobra.Command) { cmd.Flags().Uint8("f", 1, "usage") }, "f", "integer"},
		{"uint16", func(cmd *cobra.Command) { cmd.Flags().Uint16("f", 1, "usage") }, "f", "integer"},
		{"uint32", func(cmd *cobra.Command) { cmd.Flags().Uint32("f", 1, "usage") }, "f", "integer"},
		{"uint64", func(cmd *cobra.Command) { cmd.Flags().Uint64("f", 1, "usage") }, "f", "integer"},
		{"float32", func(cmd *cobra.Command) { cmd.Flags().Float32("f", 1.5, "usage") }, "f", "number"},
		{"float64", func(cmd *cobra.Command) { cmd.Flags().Float64("f", 1.5, "usage") }, "f", "number"},
		{"string", func(cmd *cobra.Command) { cmd.Flags().String("f", "v", "usage") }, "f", "string"},
		{
			"unrecognized type falls back to string",
			func(cmd *cobra.Command) { cmd.Flags().Duration("f", time.Second, "usage") },
			"f", "string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newCmd("root")
			tc.register(root)

			schema := mcp.Build(root)
			props := propertiesOf(t, schema, "root")

			prop, ok := props[tc.flagName].(map[string]any)
			if !ok {
				t.Fatalf("property %q missing or wrong type: %#v", tc.flagName, props[tc.flagName])
			}
			if got := prop["type"]; got != tc.wantType {
				t.Errorf("type = %v, want %v", got, tc.wantType)
			}
			if got := prop["description"]; got != "usage" {
				t.Errorf("description = %v, want %q", got, "usage")
			}
			if _, hasDefault := prop["default"]; !hasDefault {
				t.Errorf("expected a default entry, got none")
			}
		})
	}
}

func TestBuild_SliceFlagTypeMapping(t *testing.T) {
	tests := []struct {
		name         string
		register     func(cmd *cobra.Command)
		flagName     string
		wantItemType string
		wantDefault  []any
	}{
		{
			"boolSlice", func(cmd *cobra.Command) { cmd.Flags().BoolSlice("f", []bool{true, false}, "usage") },
			"f", "boolean", []any{true, false},
		},
		{
			"intSlice", func(cmd *cobra.Command) { cmd.Flags().IntSlice("f", []int{1, 2}, "usage") },
			"f", "integer", []any{int64(1), int64(2)},
		},
		{
			"int32Slice", func(cmd *cobra.Command) { cmd.Flags().Int32Slice("f", []int32{1, 2}, "usage") },
			"f", "integer", []any{int64(1), int64(2)},
		},
		{
			"int64Slice", func(cmd *cobra.Command) { cmd.Flags().Int64Slice("f", []int64{1, 2}, "usage") },
			"f", "integer", []any{int64(1), int64(2)},
		},
		{
			"uintSlice", func(cmd *cobra.Command) { cmd.Flags().UintSlice("f", []uint{1, 2}, "usage") },
			"f", "integer", []any{uint64(1), uint64(2)},
		},
		{
			"float32Slice", func(cmd *cobra.Command) { cmd.Flags().Float32Slice("f", []float32{1.5, 2.5}, "usage") },
			"f", "number", []any{1.5, 2.5},
		},
		{
			"float64Slice", func(cmd *cobra.Command) { cmd.Flags().Float64Slice("f", []float64{1.5, 2.5}, "usage") },
			"f", "number", []any{1.5, 2.5},
		},
		{
			"stringSlice", func(cmd *cobra.Command) { cmd.Flags().StringSlice("f", []string{"a", "b"}, "usage") },
			"f", "string", []any{"a", "b"},
		},
		{
			"stringArray", func(cmd *cobra.Command) { cmd.Flags().StringArray("f", []string{"a", "b"}, "usage") },
			"f", "string", []any{"a", "b"},
		},
		{
			"durationSlice", func(cmd *cobra.Command) {
				cmd.Flags().DurationSlice("f", []time.Duration{time.Second}, "usage")
			},
			"f", "string", []any{"1s"},
		},
		{
			"ipSlice", func(cmd *cobra.Command) {
				cmd.Flags().IPSlice("f", []net.IP{net.ParseIP("127.0.0.1")}, "usage")
			},
			"f", "string", []any{"127.0.0.1"},
		},
		{
			"empty stringSlice default", func(cmd *cobra.Command) { cmd.Flags().StringSlice("f", nil, "usage") },
			"f", "string", []any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newCmd("root")
			tc.register(root)

			schema := mcp.Build(root)
			props := propertiesOf(t, schema, "root")

			prop, ok := props[tc.flagName].(map[string]any)
			if !ok {
				t.Fatalf("property %q missing or wrong type: %#v", tc.flagName, props[tc.flagName])
			}
			if got := prop["type"]; got != "array" {
				t.Errorf("type = %v, want %q", got, "array")
			}
			items, ok := prop["items"].(map[string]any)
			if !ok || items["type"] != tc.wantItemType {
				t.Errorf("items = %#v, want type %q", prop["items"], tc.wantItemType)
			}
			if got := prop["description"]; got != "usage" {
				t.Errorf("description = %v, want %q", got, "usage")
			}
			gotDefault, ok := prop["default"].([]any)
			if !ok {
				t.Fatalf("default missing or wrong type: %#v", prop["default"])
			}
			if !reflect.DeepEqual(gotDefault, tc.wantDefault) {
				t.Errorf("default = %#v, want %#v", gotDefault, tc.wantDefault)
			}
		})
	}
}

func TestBuild_PersistentFlagInherited(t *testing.T) {
	root := newCmd("root")
	root.PersistentFlags().String("verbose", "info", "verbosity level")
	child := newCmd("child")
	root.AddCommand(child)

	schema := mcp.Build(root)
	props := propertiesOf(t, schema, "root child")

	prop, ok := props["verbose"].(map[string]any)
	if !ok {
		t.Fatalf("expected inherited flag %q, got %#v", "verbose", props)
	}
	if prop["default"] != "info" {
		t.Errorf("default = %v, want %q", prop["default"], "info")
	}
}

func TestBuild_LocalFlagShadowsInherited(t *testing.T) {
	root := newCmd("root")
	root.PersistentFlags().String("verbose", "info", "inherited usage")
	child := newCmd("child")
	child.Flags().String("verbose", "debug", "local usage")
	root.AddCommand(child)

	schema := mcp.Build(root)
	props := propertiesOf(t, schema, "root child")

	if len(props) != 1 {
		t.Fatalf("expected exactly one property, got %d: %#v", len(props), props)
	}
	prop := props["verbose"].(map[string]any)
	if prop["default"] != "debug" {
		t.Errorf("default = %v, want local value %q", prop["default"], "debug")
	}
	if prop["description"] != "local usage" {
		t.Errorf("description = %v, want local usage %q", prop["description"], "local usage")
	}
}
