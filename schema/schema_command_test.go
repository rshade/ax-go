package schema

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestNewSchemaCommandExampleUsesRootName asserts the __schema help example is
// built from the real root command name rather than a hardcoded placeholder.
func TestNewSchemaCommandExampleUsesRootName(t *testing.T) {
	root := &cobra.Command{Use: "mycli", Short: "test cli"}
	cmd := NewSchemaCommand(root)

	for _, want := range []string{"mycli __schema", "mycli __schema --as=mcp"} {
		if !strings.Contains(cmd.Example, want) {
			t.Errorf("Example missing %q; got %q", want, cmd.Example)
		}
	}
	if strings.Contains(cmd.Example, "app __schema") {
		t.Errorf("Example still uses the hardcoded placeholder root name: %q", cmd.Example)
	}
}
