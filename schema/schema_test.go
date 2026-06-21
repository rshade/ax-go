package schema

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
)

func TestBuildSchemaReflectsCommandTree(t *testing.T) {
	root := newSchemaTestCommand()
	schema := BuildSchema(root, WithSchemaVersion("v0.1.0"))

	var stdout bytes.Buffer
	if err := contract.WriteJSON(&stdout, schema); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, filepath.Join("..", "testdata", "schema_ax.golden.json"), stdout.Bytes())

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
}

func TestBuildMCPSchemaGolden(t *testing.T) {
	root := newSchemaTestCommand()

	var stdout bytes.Buffer
	if err := contract.WriteJSON(&stdout, BuildMCPSchema(root)); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, filepath.Join("..", "testdata", "schema_mcp.golden.json"), stdout.Bytes())
}

func TestNewSchemaCommandRejectsUnknownFormat(t *testing.T) {
	root := newSchemaTestCommand()
	cmd := NewSchemaCommand(root)
	cmd.SetArgs([]string{"--as=xml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("NewSchemaCommand returned nil error for unknown format")
	}
	var contractErr *contract.Error
	if !errors.As(err, &contractErr) {
		t.Fatalf("error type = %T, want *contract.Error", err)
	}
	if contractErr.ErrorCode != "validation_error" {
		t.Fatalf("ErrorCode = %q, want validation_error", contractErr.ErrorCode)
	}
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

func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\ngot:  %s\nwant: %s", path, got, want)
	}
}
