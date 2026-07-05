package schema_test

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/schema"
)

func newCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   use + " short",
		Long:    use + " long",
		Example: use + " --flag value",
	}
}

func TestBuildCommand_FieldsCopied(t *testing.T) {
	root := newCmd("root")

	got := schema.BuildCommand(root)

	want := schema.Command{
		Use:     "root",
		Short:   "root short",
		Long:    "root long",
		Example: "root --flag value",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildCommand() = %#v, want %#v", got, want)
	}
}

func TestBuildCommand_HiddenChildExcluded(t *testing.T) {
	root := newCmd("root")
	visible := newCmd("visible")
	hidden := newCmd("hidden")
	hidden.Hidden = true
	root.AddCommand(visible, hidden)

	got := schema.BuildCommand(root)

	var uses []string
	for _, child := range got.Commands {
		uses = append(uses, child.Use)
	}
	want := []string{"visible"}
	if !reflect.DeepEqual(uses, want) {
		t.Errorf("child uses = %v, want %v", uses, want)
	}
}

func TestBuildCommand_RecursiveTree(t *testing.T) {
	root := newCmd("root")
	child := newCmd("child")
	grandchild := newCmd("grandchild")
	child.AddCommand(grandchild)
	root.AddCommand(child)

	got := schema.BuildCommand(root)

	if len(got.Commands) != 1 || got.Commands[0].Use != "child" {
		t.Fatalf("Commands = %#v, want single child %q", got.Commands, "child")
	}
	grandchildren := got.Commands[0].Commands
	if len(grandchildren) != 1 || grandchildren[0].Use != "grandchild" {
		t.Fatalf("grandchildren = %#v, want single grandchild %q", grandchildren, "grandchild")
	}
}

func TestBuildCommand_FlagsPopulated(t *testing.T) {
	root := newCmd("root")
	root.Flags().StringP("name", "n", "default", "the name")

	got := schema.BuildCommand(root)

	want := []schema.Flag{
		{Name: "name", Shorthand: "n", Type: "string", Default: "default", Usage: "the name"},
	}
	if !reflect.DeepEqual(got.Flags, want) {
		t.Errorf("Flags = %#v, want %#v", got.Flags, want)
	}
}

func TestCollectFlags_LocalShadowsInherited(t *testing.T) {
	root := newCmd("root")
	root.PersistentFlags().String("verbose", "info", "inherited usage")
	child := newCmd("child")
	child.Flags().String("verbose", "debug", "local usage")
	root.AddCommand(child)

	got := schema.CollectFlags(child)

	want := []schema.Flag{
		{Name: "verbose", Type: "string", Default: "debug", Usage: "local usage"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CollectFlags() = %#v, want %#v", got, want)
	}
}

func TestCollectFlags_LocalAndInherited(t *testing.T) {
	root := newCmd("root")
	root.PersistentFlags().String("config", "path.json", "config path")
	child := newCmd("child")
	child.Flags().Bool("force", false, "force the action")
	root.AddCommand(child)

	got := schema.CollectFlags(child)

	names := make([]string, 0, len(got))
	for _, f := range got {
		names = append(names, f.Name)
	}
	want := []string{"force", "config"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("flag names = %v, want %v", names, want)
	}
}

func TestCollectFlags_RequiredFlag(t *testing.T) {
	root := newCmd("root")
	root.Flags().String("required-flag", "", "must be set")
	root.Flags().String("optional-flag", "", "may be set")
	if err := root.MarkFlagRequired("required-flag"); err != nil {
		t.Fatalf("MarkFlagRequired: %v", err)
	}

	got := schema.CollectFlags(root)

	required := map[string]bool{}
	for _, f := range got {
		required[f.Name] = f.Required
	}
	if !required["required-flag"] {
		t.Error("required-flag: Required = false, want true")
	}
	if required["optional-flag"] {
		t.Error("optional-flag: Required = true, want false")
	}
}

func TestWalkCommands_VisitsEveryDescendantIncludingHidden(t *testing.T) {
	root := newCmd("root")
	child := newCmd("child")
	hidden := newCmd("hidden")
	hidden.Hidden = true
	grandchild := newCmd("grandchild")
	child.AddCommand(grandchild)
	root.AddCommand(child, hidden)

	var visited []string
	schema.WalkCommands(root, func(cmd *cobra.Command) {
		visited = append(visited, cmd.Use)
	})

	want := []string{"root", "child", "grandchild", "hidden"}
	if !reflect.DeepEqual(visited, want) {
		t.Errorf("visited = %v, want %v", visited, want)
	}
}
