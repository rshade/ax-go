package mcp_test

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/mcp"
	axschema "github.com/rshade/ax-go/schema"
)

func newNonDeterministicCommand(use string) *cobra.Command {
	return &cobra.Command{Use: use, Short: use + " short description"}
}

func TestBuildEmptyRootHasExplicitNonDeterministicFields(t *testing.T) {
	root := newNonDeterministicCommand("root")

	got := mcp.Build(root)

	want := mcp.Schema{Tools: []mcp.Tool{
		{
			Name:                   "root",
			Description:            "root short description",
			NonDeterministicFields: []string{},
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

func TestBuildNonDeterministicFieldsMatchCommandSchema(t *testing.T) {
	type payload struct {
		GeneratedAt string `json:"generated_at" ax:"nondeterministic"`
	}

	root := newNonDeterministicCommand("root")
	raw := newNonDeterministicCommand("raw")
	root.AddCommand(raw)
	axschema.WithNonDeterministicFields[payload](root)

	mcpSchema := mcp.Build(root)
	commandSchema := axschema.BuildSchema(root)
	if !reflect.DeepEqual(
		mcpSchema.Tools[0].NonDeterministicFields,
		commandSchema.Command.NonDeterministicFields,
	) {
		t.Errorf(
			"registered MCP fields = %v, command schema fields = %v",
			mcpSchema.Tools[0].NonDeterministicFields,
			commandSchema.Command.NonDeterministicFields,
		)
	}
	if mcpSchema.Tools[1].NonDeterministicFields == nil {
		t.Fatal("unregistered MCP fields are nil, want explicit empty slice")
	}
	if len(mcpSchema.Tools[1].NonDeterministicFields) != 0 {
		t.Errorf("unregistered MCP fields = %v, want empty", mcpSchema.Tools[1].NonDeterministicFields)
	}
}
