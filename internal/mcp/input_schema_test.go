package mcp

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// buildSingleTool builds cmd (a lone command) and returns its single Tool,
// failing the test when the walk does not produce exactly one.
func buildSingleTool(t *testing.T, cmd *cobra.Command) Tool {
	t.Helper()
	built := Build(cmd)
	if len(built.Tools) != 1 {
		t.Fatalf("Build returned %d tools, want 1", len(built.Tools))
	}
	return built.Tools[0]
}

// schemaProperty returns the inputSchema property map for flagName, failing
// the test when the shape is not a JSON Schema object.
func schemaProperty(t *testing.T, tool Tool, flagName string) map[string]any {
	t.Helper()
	props, ok := tool.InputSchema[jsonSchemaProperties].(map[string]any)
	if !ok {
		t.Fatalf("properties is %T, want map[string]any", tool.InputSchema[jsonSchemaProperties])
	}
	property, ok := props[flagName].(map[string]any)
	if !ok {
		t.Fatalf("property %q is %T, want map[string]any (properties: %v)", flagName, props[flagName], props)
	}
	return property
}

// TestInputSchemaFlagTypes pins the pflag-to-JSON-Schema type mapping for
// every flag family the adapter supports: scalars map to their natural JSON
// type, duration degrades to string, and slice flags emit an array with a
// typed items schema. MCP clients rely on these types to coerce arguments.
func TestInputSchemaFlagTypes(t *testing.T) {
	cases := []struct {
		name     string
		register func(cmd *cobra.Command)
		flagName string
		usage    string
		wantType string
		wantItem string
	}{
		{
			name:     "bool maps to boolean",
			register: func(cmd *cobra.Command) { cmd.Flags().Bool("verbose", false, "v") },
			flagName: "verbose", usage: "v", wantType: jsonSchemaBoolean,
		},
		{
			name:     "count maps to integer",
			register: func(cmd *cobra.Command) { cmd.Flags().Count("verbose", "v") },
			flagName: "verbose", usage: "v", wantType: jsonSchemaInteger,
		},
		{
			name:     "int maps to integer",
			register: func(cmd *cobra.Command) { cmd.Flags().Int("times", 0, "t") },
			flagName: "times", usage: "t", wantType: jsonSchemaInteger,
		},
		{
			name:     "uint64 maps to integer",
			register: func(cmd *cobra.Command) { cmd.Flags().Uint64("big", 0, "b") },
			flagName: "big", usage: "b", wantType: jsonSchemaInteger,
		},
		{
			name:     "float64 maps to number",
			register: func(cmd *cobra.Command) { cmd.Flags().Float64("ratio", 0, "r") },
			flagName: "ratio", usage: "r", wantType: jsonSchemaNumber,
		},
		{
			name:     "string maps to string",
			register: func(cmd *cobra.Command) { cmd.Flags().String("name", "", "n") },
			flagName: "name", usage: "n", wantType: jsonSchemaString,
		},
		{
			name:     "duration degrades to string",
			register: func(cmd *cobra.Command) { cmd.Flags().Duration("timeout", time.Second, "t") },
			flagName: "timeout", usage: "t", wantType: jsonSchemaString,
		},
		{
			name:     "stringSlice maps to array of string",
			register: func(cmd *cobra.Command) { cmd.Flags().StringSlice("tags", nil, "t") },
			flagName: "tags", usage: "t", wantType: jsonSchemaArray, wantItem: jsonSchemaString,
		},
		{
			name:     "stringArray maps to array of string",
			register: func(cmd *cobra.Command) { cmd.Flags().StringArray("tags", nil, "t") },
			flagName: "tags", usage: "t", wantType: jsonSchemaArray, wantItem: jsonSchemaString,
		},
		{
			name:     "durationSlice maps to array of string",
			register: func(cmd *cobra.Command) { cmd.Flags().DurationSlice("waits", nil, "w") },
			flagName: "waits", usage: "w", wantType: jsonSchemaArray, wantItem: jsonSchemaString,
		},
		{
			name:     "boolSlice maps to array of boolean",
			register: func(cmd *cobra.Command) { cmd.Flags().BoolSlice("switches", nil, "s") },
			flagName: "switches", usage: "s", wantType: jsonSchemaArray, wantItem: jsonSchemaBoolean,
		},
		{
			name:     "intSlice maps to array of integer",
			register: func(cmd *cobra.Command) { cmd.Flags().IntSlice("ports", nil, "p") },
			flagName: "ports", usage: "p", wantType: jsonSchemaArray, wantItem: jsonSchemaInteger,
		},
		{
			name:     "uintSlice maps to array of integer",
			register: func(cmd *cobra.Command) { cmd.Flags().UintSlice("ports", nil, "p") },
			flagName: "ports", usage: "p", wantType: jsonSchemaArray, wantItem: jsonSchemaInteger,
		},
		{
			name:     "float64Slice maps to array of number",
			register: func(cmd *cobra.Command) { cmd.Flags().Float64Slice("ratios", nil, "r") },
			flagName: "ratios", usage: "r", wantType: jsonSchemaArray, wantItem: jsonSchemaNumber,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
			tc.register(cmd)

			tool := buildSingleTool(t, cmd)
			if got := tool.InputSchema[jsonSchemaTypeKey]; got != jsonSchemaObject {
				t.Errorf("inputSchema type = %v, want %q", got, jsonSchemaObject)
			}
			property := schemaProperty(t, tool, tc.flagName)
			if got := property[jsonSchemaTypeKey]; got != tc.wantType {
				t.Errorf("property type = %v, want %q", got, tc.wantType)
			}
			if got := property["description"]; got != tc.usage {
				t.Errorf("property description = %v, want %q", got, tc.usage)
			}
			if tc.wantItem == "" {
				if _, present := property["items"]; present {
					t.Errorf("scalar property unexpectedly has items schema %v", property["items"])
				}
				return
			}
			items, ok := property["items"].(map[string]any)
			if !ok {
				t.Fatalf("items is %T, want map[string]any", property["items"])
			}
			if got := items[jsonSchemaTypeKey]; got != tc.wantItem {
				t.Errorf("items type = %v, want %q", got, tc.wantItem)
			}
		})
	}
}

// TestInputSchemaArrayDefaults pins the typed conversion of slice flag
// defaults: elements are converted to the JSON type matching the items schema
// (never left as their raw string form), and an empty slice default is
// emitted as an empty JSON array.
func TestInputSchemaArrayDefaults(t *testing.T) {
	cases := []struct {
		name     string
		register func(cmd *cobra.Command)
		flagName string
		want     []any
	}{
		{
			name:     "stringSlice default stays strings",
			register: func(cmd *cobra.Command) { cmd.Flags().StringSlice("tags", []string{"a", "b"}, "t") },
			flagName: "tags",
			want:     []any{"a", "b"},
		},
		{
			name:     "intSlice default becomes JSON numbers",
			register: func(cmd *cobra.Command) { cmd.Flags().IntSlice("ports", []int{80, 443}, "p") },
			flagName: "ports",
			want:     []any{int64(80), int64(443)},
		},
		{
			name:     "uintSlice default becomes JSON numbers",
			register: func(cmd *cobra.Command) { cmd.Flags().UintSlice("ports", []uint{53}, "p") },
			flagName: "ports",
			want:     []any{uint64(53)},
		},
		{
			name:     "float64Slice default becomes JSON numbers",
			register: func(cmd *cobra.Command) { cmd.Flags().Float64Slice("ratios", []float64{1.5}, "r") },
			flagName: "ratios",
			want:     []any{1.5},
		},
		{
			name:     "boolSlice default stays booleans",
			register: func(cmd *cobra.Command) { cmd.Flags().BoolSlice("switches", []bool{true, false}, "s") },
			flagName: "switches",
			want:     []any{true, false},
		},
		{
			name:     "empty stringSlice default is an empty array",
			register: func(cmd *cobra.Command) { cmd.Flags().StringSlice("tags", nil, "t") },
			flagName: "tags",
			want:     []any{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
			tc.register(cmd)

			property := schemaProperty(t, buildSingleTool(t, cmd), tc.flagName)
			got, present := property["default"]
			if !present {
				t.Fatalf("array default is missing; want %v", tc.want)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("default = %#v (%T), want %#v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// unconvertibleSliceValue implements pflag.SliceValue but reports elements
// that do not parse as its declared intSlice item type, simulating a custom
// flag whose stored form and declared type disagree.
type unconvertibleSliceValue struct{}

func (unconvertibleSliceValue) String() string      { return "[bogus]" }
func (unconvertibleSliceValue) Set(string) error    { return nil }
func (unconvertibleSliceValue) Type() string        { return "intSlice" }
func (unconvertibleSliceValue) Append(string) error { return nil }
func (unconvertibleSliceValue) Replace([]string) error {
	return nil
}
func (unconvertibleSliceValue) GetSlice() []string { return []string{"bogus"} }

// plainSliceTypedValue declares a slice type but does not implement
// pflag.SliceValue, so its default cannot be enumerated at all.
type plainSliceTypedValue struct{}

func (plainSliceTypedValue) String() string   { return "[]" }
func (plainSliceTypedValue) Set(string) error { return nil }
func (plainSliceTypedValue) Type() string     { return "float64Slice" }

// TestInputSchemaArrayDefaultOmittedWhenUnconvertible pins the fail-closed
// contract for slice defaults: when a flag's elements cannot be converted to
// the declared item type — or the value cannot be enumerated as a slice at
// all — the schema omits the default rather than advertising a bogus one.
func TestInputSchemaArrayDefaultOmittedWhenUnconvertible(t *testing.T) {
	cases := []struct {
		name     string
		value    pflag.Value
		flagName string
	}{
		{
			name:     "unparseable elements omit the default",
			value:    unconvertibleSliceValue{},
			flagName: "ports",
		},
		{
			name:     "non-slice value behind a slice type omits the default",
			value:    plainSliceTypedValue{},
			flagName: "ports",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
			cmd.Flags().Var(tc.value, tc.flagName, "custom value flag")

			property := schemaProperty(t, buildSingleTool(t, cmd), tc.flagName)
			if got, present := property["default"]; present {
				t.Errorf("default = %#v, want it omitted for an unconvertible value", got)
			}
			if got := property[jsonSchemaTypeKey]; got != jsonSchemaArray {
				t.Errorf("property type = %v, want %q (type mapping still applies)", got, jsonSchemaArray)
			}
		})
	}
}

// TestInputSchemaScalarDefaultEdgeCases pins the remaining scalar default
// conversions the starter tests did not cover: a count flag's zero default is
// a JSON number, a duration default survives as its string form, and a
// DefValue that does not parse as the declared type omits the default rather
// than advertising an invalid one.
func TestInputSchemaScalarDefaultEdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		register func(cmd *cobra.Command)
		flagName string
		corrupt  string
		want     any
		wantSet  bool
	}{
		{
			name:     "count zero default is a JSON number",
			register: func(cmd *cobra.Command) { cmd.Flags().Count("verbose", "v") },
			flagName: "verbose",
			want:     int64(0),
			wantSet:  true,
		},
		{
			name:     "duration default is its string form",
			register: func(cmd *cobra.Command) { cmd.Flags().Duration("timeout", time.Minute, "t") },
			flagName: "timeout",
			want:     "1m0s",
			wantSet:  true,
		},
		{
			name: "unparseable bool default is omitted",
			register: func(cmd *cobra.Command) {
				cmd.Flags().Bool("verbose", false, "v")
			},
			flagName: "verbose",
			corrupt:  "notabool",
			wantSet:  false,
		},
		{
			name: "unparseable int default is omitted",
			register: func(cmd *cobra.Command) {
				cmd.Flags().Int("times", 1, "t")
			},
			flagName: "times",
			corrupt:  "NaN",
			wantSet:  false,
		},
		{
			name: "unparseable float default is omitted",
			register: func(cmd *cobra.Command) {
				cmd.Flags().Float64("ratio", 1.5, "r")
			},
			flagName: "ratio",
			corrupt:  "big",
			wantSet:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
			tc.register(cmd)
			if tc.corrupt != "" {
				// Simulate a DefValue that no longer parses as the declared type
				// (e.g. a hand-edited generated flag); the schema must drop it.
				cmd.Flags().Lookup(tc.flagName).DefValue = tc.corrupt
			}

			property := schemaProperty(t, buildSingleTool(t, cmd), tc.flagName)
			got, present := property["default"]
			if present != tc.wantSet {
				t.Fatalf("default presence = %v, want %v (value %#v)", present, tc.wantSet, got)
			}
			if tc.wantSet && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("default = %#v (%T), want %#v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

// TestInputSchemaRequiredIsSorted pins the determinism contract of the
// required array: names are sorted regardless of registration order, and a
// required persistent flag inherited from the parent lands in the child's
// required set.
func TestInputSchemaRequiredIsSorted(t *testing.T) {
	cmd := &cobra.Command{Use: "deploy", RunE: noopRunE}
	cmd.Flags().String("zebra", "", "registered first")
	cmd.Flags().String("alpha", "", "registered second")
	for _, name := range []string{"zebra", "alpha"} {
		if err := cmd.MarkFlagRequired(name); err != nil {
			t.Fatalf("mark flag %q required: %v", name, err)
		}
	}

	tool := buildSingleTool(t, cmd)
	required, ok := tool.InputSchema["required"].([]string)
	if !ok {
		t.Fatalf("required is %T, want []string", tool.InputSchema["required"])
	}
	if want := []string{"alpha", "zebra"}; !slices.Equal(required, want) {
		t.Errorf("required = %v, want sorted %v", required, want)
	}
}

// TestInputSchemaInheritedFlags asserts inherited persistent flags surface in
// the child's input schema, an inherited required flag lands in the child's
// required array, and a local flag shadowing an inherited name appears exactly
// once with the local definition winning.
func TestInputSchemaInheritedFlags(t *testing.T) {
	root := &cobra.Command{Use: "demo", RunE: noopRunE}
	root.PersistentFlags().String("config", "", "inherited config")
	root.PersistentFlags().String("token", "", "inherited token")
	if err := root.MarkPersistentFlagRequired("token"); err != nil {
		t.Fatalf("mark persistent flag required: %v", err)
	}
	child := &cobra.Command{Use: "child", RunE: noopRunE}
	child.Flags().String("config", "", "local shadow wins")
	root.AddCommand(child)

	built := Build(root)
	childTool := toolByName(built.Tools, "demo-child")
	if childTool == nil {
		t.Fatalf("tools missing %q", "demo-child")
	}
	props, ok := childTool.InputSchema[jsonSchemaProperties].(map[string]any)
	if !ok {
		t.Fatalf("properties is %T, want map[string]any", childTool.InputSchema[jsonSchemaProperties])
	}
	if _, present := props["token"]; !present {
		t.Errorf("inherited flag %q missing from child schema: %v", "token", props)
	}
	config, ok := props["config"].(map[string]any)
	if !ok {
		t.Fatalf("config property is %T, want map[string]any", props["config"])
	}
	if got := config["description"]; got != "local shadow wins" {
		t.Errorf("shadowed flag description = %v, want the local definition %q", got, "local shadow wins")
	}
	required, ok := childTool.InputSchema["required"].([]string)
	if !ok {
		t.Fatalf("required is %T, want []string", childTool.InputSchema["required"])
	}
	if !slices.Contains(required, "token") {
		t.Errorf("inherited required flag missing from required %v", required)
	}
}

// TestBuildToolDescriptionUsesShort pins the tool description source: the
// command's Short string, even when Long is also set.
func TestBuildToolDescriptionUsesShort(t *testing.T) {
	cmd := &cobra.Command{Use: "demo", Short: "short help", Long: "much longer help text", RunE: noopRunE}

	tool := buildSingleTool(t, cmd)
	if tool.Description != "short help" {
		t.Errorf("Description = %q, want Short %q", tool.Description, "short help")
	}
}
