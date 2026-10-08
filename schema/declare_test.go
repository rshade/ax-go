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

// assertDeclarationError pins the authoring-error contract shared with
// DeclarePrompt: an *contract.Error with invalid_schema_declaration, exit 2, and
// context {field, reason}.
func assertDeclarationError(t *testing.T, err error, field, reason string) {
	t.Helper()
	contractErr, ok := errors.AsType[*contract.Error](err)
	if !ok {
		t.Fatalf("error = %T %v, want *contract.Error", err, err)
	}
	if contractErr.ErrorCode != invalidDeclarationCode || contractErr.ExitCode() != contract.ExitValidation {
		t.Fatalf(
			"error = %q exit %d, want %s exit 2",
			contractErr.ErrorCode,
			contractErr.ExitCode(),
			invalidDeclarationCode,
		)
	}
	want := map[string]any{"field": field, "reason": reason}
	if !reflect.DeepEqual(contractErr.Context, want) {
		t.Fatalf("context = %#v, want %#v", contractErr.Context, want)
	}
	if contractErr.ActionableFix == "" {
		t.Fatalf("reason %q carries no actionable_fix", reason)
	}
}

func TestDeclareFlagEnum(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
		cmd.Flags().String("output", "json", "output format")
		cmd.Flags().Int("replicas", 1, "replica count")
		return cmd
	}

	cmd := newCmd()
	if err := DeclareFlagEnum(cmd, "output", "json", "table", "yaml"); err != nil {
		t.Fatalf("DeclareFlagEnum(output): %v", err)
	}
	if err := DeclareFlagEnum(cmd, "replicas", "1", "3", "5"); err != nil {
		t.Fatalf("DeclareFlagEnum(replicas): %v", err)
	}
	for _, bad := range []struct {
		flag          string
		values        []string
		field, reason string
	}{
		{"output", nil, "values", "required"},
		{"output", []string{"table"}, "default", "not_in_enum"},
		{"replicas", []string{"1", "x"}, "values", "invalid_value"},
		{"replicas", []string{"3", "03"}, "values", "duplicate"},
		{"missing", []string{"x"}, "flag", "flag_not_found"},
	} {
		assertDeclarationError(t, DeclareFlagEnum(cmd, bad.flag, bad.values...), bad.field, bad.reason)
	}
	assertDeclarationError(t, DeclareFlagEnum(nil, "output", "json"), "cmd", "nil_command")

	built := BuildSchema(cmd)
	if got := findFlagSchema(
		t,
		built.Command.Flags,
		"output",
	).Enum; !reflect.DeepEqual(
		got,
		[]string{"json", "table", "yaml"},
	) {
		t.Fatalf("output enum = %v", got)
	}
	if got := findFlagSchema(
		t,
		built.Command.Flags,
		"replicas",
	).Enum; !reflect.DeepEqual(
		got,
		[]string{"1", "3", "5"},
	) {
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

func TestDeclareFlagExample(t *testing.T) {
	cmd := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().Duration("timeout", 0, "deadline")
	cmd.Flags().Int("replicas", 1, "replica count")

	if err := DeclareFlagExample(cmd, "timeout", "45s"); err != nil {
		t.Fatalf("DeclareFlagExample: %v", err)
	}
	for _, bad := range []struct{ flag, example, field, reason string }{
		{"timeout", "", "example", "required"},
		{"replicas", "three", "example", "invalid_value"},
		{"missing", "x", "flag", "flag_not_found"},
	} {
		assertDeclarationError(t, DeclareFlagExample(cmd, bad.flag, bad.example), bad.field, bad.reason)
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

func TestDeclareCapability(t *testing.T) {
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	deploy := &cobra.Command{Use: "deploy", Short: "deploy", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(deploy)
	if err := DeclareCapability(deploy, CapabilityMutate, " idempotent by release name "); err != nil {
		t.Fatalf("DeclareCapability: %v", err)
	}
	for _, bad := range []Capability{"", "write", "Read-Only"} {
		assertDeclarationError(t, DeclareCapability(deploy, bad, ""), "class", "not_in_vocabulary")
	}
	assertDeclarationError(t, DeclareCapability(nil, CapabilityAdmin, ""), "cmd", "nil_command")

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
	if err := DeclareFlagEnum(cmd, "replicas", "1", "3"); err != nil {
		t.Fatalf("DeclareFlagEnum: %v", err)
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
