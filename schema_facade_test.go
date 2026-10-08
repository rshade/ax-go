package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"

	isolatedschema "github.com/rshade/ax-go/schema"
)

func TestRootSchemaFacadeUsesIsolatedTypes(t *testing.T) {
	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app run",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	var option isolatedschema.Option = WithSchemaVersion("v0.1.0")
	var got isolatedschema.Schema = BuildSchema(root, option)
	if got.Tool != "app" {
		t.Fatalf("Tool = %q, want app", got.Tool)
	}

	var mcpSchema isolatedschema.MCPSchema = BuildMCPSchema(root)
	if len(mcpSchema.Tools) != 1 || mcpSchema.Tools[0].Name != "app" {
		t.Fatalf("MCP tools = %#v, want app tool", mcpSchema.Tools)
	}
}

func TestRootSchemaFacadeVersionMatchesIsolatedPackage(t *testing.T) {
	if SchemaVersion != isolatedschema.SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", SchemaVersion, isolatedschema.SchemaVersion)
	}
}

func TestRootSchemaFacadeDeclaresPromptsAndResources(t *testing.T) {
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	var prompt isolatedschema.Prompt = Prompt{
		Name:      "triage",
		Arguments: []PromptArgument{{Name: "window"}},
		Template:  "since {{window}}",
	}
	var resource isolatedschema.Resource = Resource{URI: "app://docs", Name: "docs", MIMEType: "text/markdown"}
	if err := DeclarePrompt(root, prompt); err != nil {
		t.Fatalf("DeclarePrompt = %v", err)
	}
	if err := DeclareResource(root, resource); err != nil {
		t.Fatalf("DeclareResource = %v", err)
	}

	var mcpSchema isolatedschema.MCPSchema = BuildMCPSchema(root)
	var prompts []MCPPrompt = mcpSchema.Prompts
	var resources []MCPResource = mcpSchema.Resources
	if len(prompts) != 1 || len(prompts[0].Arguments) != 1 || len(resources) != 1 {
		t.Fatalf("MCP prompts = %#v, resources = %#v", prompts, resources)
	}
	var arg MCPPromptArgument = prompts[0].Arguments[0]
	if arg.Name != "window" || resources[0].MIMEType != "text/markdown" {
		t.Fatalf("argument = %#v, resource = %#v", arg, resources[0])
	}

	var envelope *Error
	err := DeclarePrompt(nil, prompt)
	if !errors.As(err, &envelope) || envelope.ErrorCode != "invalid_schema_declaration" {
		t.Fatalf("DeclarePrompt(nil) = %v, want invalid_schema_declaration", err)
	}
	err = DeclareResource(nil, resource)
	if !errors.As(err, &envelope) || ErrorExitCode(err) != ExitValidation {
		t.Fatalf("DeclareResource(nil) = %v, want exit %d", err, ExitValidation)
	}
}

// TestExecuteSchemaFailsClosedOnDuplicateDeclaration proves the duplicate
// contract end to end through Execute: exit 2, nothing on stdout, and one
// validation_error envelope on stderr.
func TestExecuteSchemaFailsClosedOnDuplicateDeclaration(t *testing.T) {
	for _, format := range []string{"--as=ax", "--as=mcp"} {
		t.Run(format, func(t *testing.T) {
			root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
			child := &cobra.Command{Use: "run", RunE: func(*cobra.Command, []string) error { return nil }}
			root.AddCommand(child)
			for _, cmd := range []*cobra.Command{root, child} {
				if err := DeclarePrompt(cmd, Prompt{Name: "triage", Template: "t"}); err != nil {
					t.Fatalf("DeclarePrompt = %v", err)
				}
			}
			root.SetArgs([]string{"__schema", format})

			var stdout, stderr bytes.Buffer
			code := Execute(
				context.Background(),
				root,
				WithStdout(&stdout),
				WithStderr(&stderr),
				WithEnv(func(string) string { return "" }),
				WithStdoutIsTTY(false),
			)

			if code != ExitValidation {
				t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitValidation, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			var envelope struct {
				ErrorCode string         `json:"error_code"`
				Context   map[string]any `json:"context"`
			}
			if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &envelope); err != nil {
				t.Fatalf("stderr is not one JSON envelope: %v; stderr=%s", err, stderr.String())
			}
			if envelope.ErrorCode != "validation_error" || envelope.Context["key"] != "triage" {
				t.Fatalf("envelope = %+v, want validation_error for triage", envelope)
			}
		})
	}
}
