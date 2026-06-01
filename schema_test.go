package ax

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildSchemaReflectsCommandTree(t *testing.T) {
	root := newSchemaTestCommand()
	schema := BuildSchema(root, WithSchemaVersion("v0.1.0"))

	var stdout bytes.Buffer
	if err := WriteJSON(&stdout, schema); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_ax.golden.json", stdout.Bytes())

	if schema.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", schema.SchemaVersion, SchemaVersion)
	}
	if schema.Tool != "app" {
		t.Fatalf("Tool = %q, want app", schema.Tool)
	}
	if schema.Version != "v0.1.0" {
		t.Fatalf("Version = %q, want v0.1.0", schema.Version)
	}
	if len(schema.Command.Commands) != 1 {
		t.Fatalf("Commands length = %d, want 1", len(schema.Command.Commands))
	}
	if schema.Command.Commands[0].Use != "run" {
		t.Fatalf("child Use = %q, want run", schema.Command.Commands[0].Use)
	}
	if len(schema.Command.Flags) != 1 || schema.Command.Flags[0].Name != "config" {
		t.Fatalf("root flags = %#v, want config flag", schema.Command.Flags)
	}
}

func TestBuildMCPSchemaGolden(t *testing.T) {
	root := newSchemaTestCommand()

	var stdout bytes.Buffer
	if err := WriteJSON(&stdout, BuildMCPSchema(root)); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_mcp.golden.json", stdout.Bytes())
}

func newSchemaTestCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app run --name demo",
	}
	root.PersistentFlags().String("config", "", "config file")
	run := &cobra.Command{
		Use:     "run",
		Short:   "run something",
		Example: "app run --name demo",
	}
	run.Flags().String("name", "", "name to use")
	root.AddCommand(run)

	return root
}
