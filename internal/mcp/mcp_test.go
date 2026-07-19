package mcp

import (
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

// noopRunE is a do-nothing command body; Build never invokes it.
func noopRunE(*cobra.Command, []string) error { return nil }

// toolByName returns the built tool with the given name, or nil.
func toolByName(tools []Tool, name string) *Tool {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

// TestBuildJoinsCommandPathSegmentsWithHyphens is the regression test for
// space-joined tool names: the MCP tool-name rule ^[a-zA-Z0-9_.-]+$ forbids
// spaces, so every emitted tool name must match it and multi-segment command
// paths must be joined with "-".
func TestBuildJoinsCommandPathSegmentsWithHyphens(t *testing.T) {
	namePattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

	root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
	root.AddCommand(&cobra.Command{Use: "greet", Short: "greet someone", RunE: noopRunE})
	group := &cobra.Command{Use: "group", Short: "a command group", RunE: noopRunE}
	group.AddCommand(&cobra.Command{Use: "child", Short: "group child", RunE: noopRunE})
	root.AddCommand(group)

	built := Build(root)
	if len(built.Tools) == 0 {
		t.Fatal("Build returned no tools")
	}
	names := make([]string, 0, len(built.Tools))
	for _, tool := range built.Tools {
		if !namePattern.MatchString(tool.Name) {
			t.Errorf("tool name %q violates the MCP name rule %s", tool.Name, namePattern)
		}
		names = append(names, tool.Name)
	}
	for _, want := range []string{"demo", "demo-greet", "demo-group", "demo-group-child"} {
		if !slices.Contains(names, want) {
			t.Errorf("tools missing %q; got %v", want, names)
		}
	}
}

// TestBuildExcludesReservedCommands asserts the reserved infrastructure
// commands — __schema, mcp-server, and Cobra's completion tree — never surface
// as MCP tools, so the static --as=mcp adapter and the live mcp-server share
// one exclusion set.
func TestBuildExcludesReservedCommands(t *testing.T) {
	root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
	root.AddCommand(&cobra.Command{Use: "work", Short: "real work", RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "__schema", Short: "schema", RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "mcp-server", Short: "server", RunE: noopRunE})
	completion := &cobra.Command{Use: "completion", Short: "completion scripts", RunE: noopRunE}
	completion.AddCommand(&cobra.Command{Use: "bash", Short: "bash script", RunE: noopRunE})
	root.AddCommand(completion)

	built := Build(root)
	for _, tool := range built.Tools {
		if tool.Name != "demo" && tool.Name != "demo-work" {
			t.Errorf("reserved command leaked into the tool set as %q", tool.Name)
		}
	}
}

// TestBuildPrunesHiddenSubtrees asserts a hidden command prunes its whole
// subtree — including visible children — matching
// internal/schema.BuildCommand's documented subtree pruning (previously the
// MCP walk kept visible children of hidden parents, diverging from the ax
// schema).
func TestBuildPrunesHiddenSubtrees(t *testing.T) {
	root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
	root.AddCommand(&cobra.Command{Use: "work", Short: "real work", RunE: noopRunE})
	admin := &cobra.Command{Use: "admin", Short: "hidden group", Hidden: true, RunE: noopRunE}
	admin.AddCommand(&cobra.Command{Use: "reset", Short: "visible child of a hidden group", RunE: noopRunE})
	root.AddCommand(admin)
	root.AddCommand(&cobra.Command{Use: "secret", Short: "hidden leaf", Hidden: true, RunE: noopRunE})

	built := Build(root)
	names := make([]string, 0, len(built.Tools))
	for _, tool := range built.Tools {
		names = append(names, tool.Name)
	}
	for _, excluded := range []string{"demo-admin", "demo-admin-reset", "demo-secret"} {
		if slices.Contains(names, excluded) {
			t.Errorf("hidden command %q leaked into the tool set; got %v", excluded, names)
		}
	}
	if !slices.Contains(names, "demo-work") {
		t.Errorf("visible command %q missing from the tool set; got %v", "demo-work", names)
	}
}

// TestFlagPropertyEmitsTypedScalarDefaults pins the JSON type of a scalar
// flag's inputSchema default: a boolean flag's default is a JSON boolean, an
// int/uint/float flag's a JSON number, and a string flag's a JSON string —
// never the raw pflag DefValue string ("false" is not a valid boolean
// default). An empty DefValue omits the default entirely.
func TestFlagPropertyEmitsTypedScalarDefaults(t *testing.T) {
	cases := []struct {
		name     string
		register func(cmd *cobra.Command)
		flagName string
		want     any
		wantSet  bool
	}{
		{
			name:     "bool default is a JSON boolean",
			register: func(cmd *cobra.Command) { cmd.Flags().Bool("verbose", false, "verbose output") },
			flagName: "verbose",
			want:     false,
			wantSet:  true,
		},
		{
			name:     "int default is a JSON number",
			register: func(cmd *cobra.Command) { cmd.Flags().Int("times", 1, "repeat count") },
			flagName: "times",
			want:     int64(1),
			wantSet:  true,
		},
		{
			name:     "uint default is a JSON number",
			register: func(cmd *cobra.Command) { cmd.Flags().Uint("workers", 2, "worker count") },
			flagName: "workers",
			want:     uint64(2),
			wantSet:  true,
		},
		{
			name:     "float default is a JSON number",
			register: func(cmd *cobra.Command) { cmd.Flags().Float64("ratio", 1.5, "mix ratio") },
			flagName: "ratio",
			want:     1.5,
			wantSet:  true,
		},
		{
			name:     "string default is a JSON string",
			register: func(cmd *cobra.Command) { cmd.Flags().String("name", "world", "name to greet") },
			flagName: "name",
			want:     "world",
			wantSet:  true,
		},
		{
			name:     "empty string default is omitted",
			register: func(cmd *cobra.Command) { cmd.Flags().String("config", "", "config file") },
			flagName: "config",
			wantSet:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
			tc.register(cmd)

			built := Build(cmd)
			if len(built.Tools) != 1 {
				t.Fatalf("Build returned %d tools, want 1", len(built.Tools))
			}
			props, ok := built.Tools[0].InputSchema[jsonSchemaProperties].(map[string]any)
			if !ok {
				t.Fatalf("properties is %T, want map[string]any", built.Tools[0].InputSchema[jsonSchemaProperties])
			}
			property, ok := props[tc.flagName].(map[string]any)
			if !ok {
				t.Fatalf("%s schema is %T, want map[string]any", tc.flagName, props[tc.flagName])
			}
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

// TestBuildMarksRequiredFlags asserts a Cobra-required flag (MarkFlagRequired)
// lands in the inputSchema required array while optional flags stay out of it.
// Previously internal/schema collected the required bit but the MCP adapter
// never emitted inputSchema.required, so mandatory and optional arguments were
// indistinguishable to MCP clients.
func TestBuildMarksRequiredFlags(t *testing.T) {
	root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
	deploy := &cobra.Command{Use: "deploy", Short: "deploy", RunE: noopRunE}
	deploy.Flags().String("target", "", "deploy target")
	deploy.Flags().String("tag", "", "optional tag")
	if err := deploy.MarkFlagRequired("target"); err != nil {
		t.Fatalf("mark flag required: %v", err)
	}
	root.AddCommand(deploy)

	built := Build(root)

	deployTool := toolByName(built.Tools, "demo-deploy")
	if deployTool == nil {
		t.Fatalf("tools missing %q", "demo-deploy")
	}
	required, ok := deployTool.InputSchema["required"].([]string)
	if !ok {
		t.Fatalf("required is %T, want []string", deployTool.InputSchema["required"])
	}
	if !slices.Equal(required, []string{"target"}) {
		t.Errorf("required = %v, want %v (optional flags must stay out)", required, []string{"target"})
	}

	rootTool := toolByName(built.Tools, "demo")
	if rootTool == nil {
		t.Fatalf("tools missing %q", "demo")
	}
	if _, present := rootTool.InputSchema["required"]; present {
		t.Errorf("root inputSchema has unexpected required %v; the key must be absent when nothing is required",
			rootTool.InputSchema["required"])
	}
}
