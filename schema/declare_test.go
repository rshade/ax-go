package schema

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
)

func findFlagSchema(t *testing.T, flags []FlagSchema, name string) FlagSchema {
	t.Helper()
	for _, flag := range flags {
		if flag.Name == name {
			return flag
		}
	}
	t.Fatalf("flag %q not in schema", name)
	return FlagSchema{}
}

func mcpProperty(t *testing.T, tool MCPTool, name string) map[string]any {
	t.Helper()
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties is %T", tool.InputSchema["properties"])
	}
	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("property %q missing", name)
	}
	return property
}

func TestWithFlagEnum(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
		cmd.Flags().String("output", "json", "output format")
		cmd.Flags().Int("replicas", 1, "replica count")
		return cmd
	}

	cmd := newCmd()
	if err := WithFlagEnum(cmd, "output", "json", "table", "yaml"); err != nil {
		t.Fatalf("WithFlagEnum(output): %v", err)
	}
	if err := WithFlagEnum(cmd, "replicas", "1", "3", "5"); err != nil {
		t.Fatalf("WithFlagEnum(replicas): %v", err)
	}
	for _, bad := range [][]string{nil, {"table"}, {"1", "x"}} {
		flag := "output"
		if len(bad) == 2 {
			flag = "replicas"
		}
		if err := WithFlagEnum(cmd, flag, bad...); !errors.Is(err, ErrInvalidDeclaration) {
			t.Fatalf("WithFlagEnum(%s, %v) error = %v, want ErrInvalidDeclaration", flag, bad, err)
		}
	}
	if err := WithFlagEnum(nil, "output", "json"); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("WithFlagEnum(nil) error = %v, want ErrInvalidDeclaration", err)
	}

	built := BuildSchema(cmd)
	if got := findFlagSchema(t, built.Command.Flags, "output").Enum; !reflect.DeepEqual(got, []string{"json", "table", "yaml"}) {
		t.Fatalf("output enum = %v", got)
	}
	if got := findFlagSchema(t, built.Command.Flags, "replicas").Enum; !reflect.DeepEqual(got, []string{"1", "3", "5"}) {
		t.Fatalf("replicas enum = %v", got)
	}

	tool := BuildMCPSchema(cmd).Tools[0]
	if got := mcpProperty(t, tool, "replicas")["enum"]; !reflect.DeepEqual(got, []any{int64(1), int64(3), int64(5)}) {
		t.Fatalf("MCP replicas enum = %#v, want typed integers", got)
	}

	var first, second bytes.Buffer
	if err := contract.WriteJSON(&first, BuildSchema(cmd)); err != nil {
		t.Fatal(err)
	}
	if err := contract.WriteJSON(&second, BuildSchema(cmd)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("BuildSchema not deterministic:\n%s\n%s", first.String(), second.String())
	}
}

func TestWithFlagExample(t *testing.T) {
	cmd := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().Duration("timeout", 0, "deadline")
	cmd.Flags().Int("replicas", 1, "replica count")

	if err := WithFlagExample(cmd, "timeout", "45s"); err != nil {
		t.Fatalf("WithFlagExample: %v", err)
	}
	for _, bad := range []struct{ flag, example string }{
		{"timeout", ""},
		{"replicas", "three"},
		{"missing", "x"},
	} {
		if err := WithFlagExample(cmd, bad.flag, bad.example); !errors.Is(err, ErrInvalidDeclaration) {
			t.Fatalf("WithFlagExample(%s, %q) error = %v, want ErrInvalidDeclaration", bad.flag, bad.example, err)
		}
	}

	if got := findFlagSchema(t, BuildSchema(cmd).Command.Flags, "timeout").Example; got != "45s" {
		t.Fatalf("FlagSchema.Example = %q, want 45s", got)
	}
	if got := findFlagSchema(t, BuildSchema(cmd).Command.Flags, "replicas").Example; got != "" {
		t.Fatalf("undeclared FlagSchema.Example = %q, want empty", got)
	}
	tool := BuildMCPSchema(cmd).Tools[0]
	if got := mcpProperty(t, tool, "timeout")["examples"]; !reflect.DeepEqual(got, []any{"45s"}) {
		t.Fatalf("MCP examples = %#v, want [45s]", got)
	}

	var first, second bytes.Buffer
	if err := contract.WriteJSON(&first, BuildMCPSchema(cmd)); err != nil {
		t.Fatal(err)
	}
	if err := contract.WriteJSON(&second, BuildMCPSchema(cmd)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("BuildMCPSchema not deterministic:\n%s\n%s", first.String(), second.String())
	}
}

func TestWithCapability(t *testing.T) {
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	deploy := &cobra.Command{Use: "deploy", Short: "deploy", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(deploy)
	if err := WithCapability(deploy, CapabilityMutate, " idempotent by release name "); err != nil {
		t.Fatalf("WithCapability: %v", err)
	}
	for _, bad := range []Capability{"", "write", "Read-Only"} {
		if err := WithCapability(deploy, bad, ""); !errors.Is(err, ErrInvalidDeclaration) {
			t.Fatalf("WithCapability(%q) error = %v, want ErrInvalidDeclaration", bad, err)
		}
	}
	if err := WithCapability(nil, CapabilityAdmin, ""); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("WithCapability(nil) error = %v, want ErrInvalidDeclaration", err)
	}

	built := BuildSchema(root)
	if built.Command.Capability != nil {
		t.Fatalf("root Capability = %+v, want nil", built.Command.Capability)
	}
	want := &CapabilitySchema{Class: CapabilityMutate, Note: "idempotent by release name"}
	if got := built.Command.Commands[0].Capability; !reflect.DeepEqual(got, want) {
		t.Fatalf("deploy Capability = %+v, want %+v", got, want)
	}

	tools := BuildMCPSchema(root).Tools
	if tools[0].Capability != nil || tools[0].Annotations != nil {
		t.Fatalf("unclassified tool = %+v, want nil Capability and Annotations", tools[0])
	}
	yes := true
	if !reflect.DeepEqual(tools[1].Capability, want) {
		t.Fatalf("tool Capability = %+v, want %+v", tools[1].Capability, want)
	}
	if !reflect.DeepEqual(tools[1].Annotations, &MCPToolAnnotations{DestructiveHint: &yes}) {
		t.Fatalf("tool Annotations = %+v, want destructiveHint true", tools[1].Annotations)
	}

	var commandJSON, toolJSON bytes.Buffer
	if err := contract.WriteJSON(&commandJSON, built.Command.Commands[0]); err != nil {
		t.Fatal(err)
	}
	wantCommand := `{"use":"deploy","short":"deploy","capability":{"class":"mutate","note":"idempotent by release name"},` +
		`"non_deterministic_fields":[]}` + "\n"
	if commandJSON.String() != wantCommand {
		t.Fatalf("command JSON =\n%s\nwant\n%s", commandJSON.String(), wantCommand)
	}
	tools[1].InputSchema = nil
	if err := contract.WriteJSON(&toolJSON, tools[1]); err != nil {
		t.Fatal(err)
	}
	wantTool := `{"name":"app-deploy","description":"deploy","inputSchema":null,"nonDeterministicFields":[],` +
		`"capability":{"class":"mutate","note":"idempotent by release name"},"annotations":{"destructiveHint":true}}` + "\n"
	if toolJSON.String() != wantTool {
		t.Fatalf("tool JSON =\n%s\nwant\n%s", toolJSON.String(), wantTool)
	}
}

// TestDerivedFieldsSurviveEnumDeclaration asserts declaring an enum does not
// displace the derived facts: default and required still come from Cobra on
// both surfaces (US4-AS3, FR-006).
func TestDerivedFieldsSurviveEnumDeclaration(t *testing.T) {
	cmd := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().Int("replicas", 3, "replica count")
	if err := cmd.MarkFlagRequired("replicas"); err != nil {
		t.Fatal(err)
	}
	if err := WithFlagEnum(cmd, "replicas", "1", "3"); err != nil {
		t.Fatalf("WithFlagEnum: %v", err)
	}

	flag := findFlagSchema(t, BuildSchema(cmd).Command.Flags, "replicas")
	if flag.Default != "3" || !flag.Required || !reflect.DeepEqual(flag.Enum, []string{"1", "3"}) {
		t.Fatalf("FlagSchema = %+v, want default 3, required, enum [1 3]", flag)
	}

	tool := BuildMCPSchema(cmd).Tools[0]
	if !reflect.DeepEqual(tool.InputSchema["required"], []string{"replicas"}) {
		t.Fatalf("inputSchema.required = %#v, want [replicas]", tool.InputSchema["required"])
	}
	if got := mcpProperty(t, tool, "replicas")["default"]; got != int64(3) {
		t.Fatalf("MCP default = %#v, want int64 3", got)
	}
}
