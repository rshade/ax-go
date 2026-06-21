package ax

import (
	"testing"

	"github.com/spf13/cobra"

	isolatedschema "github.com/rshade/ax-go/schema"
)

func TestRootSchemaFacadeUsesIsolatedTypes(t *testing.T) {
	root := &cobra.Command{Use: "app", Short: "test app", Example: "app run"}
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
