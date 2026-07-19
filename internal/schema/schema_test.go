package schema

import (
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// noopRunE is a do-nothing command body; reflection never invokes it.
func noopRunE(*cobra.Command, []string) error { return nil }

// TestBuildCommandReflectsTree asserts BuildCommand copies the command
// metadata (Use/Short/Long/Example) and reflects the full visible subtree
// recursively, so the emitted schema describes the real command tree.
func TestBuildCommandReflectsTree(t *testing.T) {
	root := &cobra.Command{
		Use:     "demo",
		Short:   "demo root",
		Long:    "long root help",
		Example: "demo work",
		RunE:    noopRunE,
	}
	child := &cobra.Command{Use: "work", Short: "real work", RunE: noopRunE}
	child.AddCommand(&cobra.Command{Use: "deep", Short: "grandchild", RunE: noopRunE})
	root.AddCommand(child)

	got := BuildCommand(root)
	if got.Use != "demo" || got.Short != "demo root" || got.Long != "long root help" || got.Example != "demo work" {
		t.Errorf("metadata = %+v, want Use/Short/Long/Example copied from the root command", got)
	}
	if len(got.Commands) != 1 {
		t.Fatalf("Commands has %d entries, want 1", len(got.Commands))
	}
	if got.Commands[0].Use != "work" || got.Commands[0].Short != "real work" {
		t.Errorf("child = %+v, want the work command reflected", got.Commands[0])
	}
	if len(got.Commands[0].Commands) != 1 || got.Commands[0].Commands[0].Use != "deep" {
		t.Errorf("grandchildren = %+v, want the deep command reflected recursively", got.Commands[0].Commands)
	}
}

// TestBuildCommandPrunesHiddenSubtrees asserts a hidden command is pruned
// wholesale — its visible children disappear with it — at every depth, while
// visible siblings survive.
func TestBuildCommandPrunesHiddenSubtrees(t *testing.T) {
	root := &cobra.Command{Use: "demo", RunE: noopRunE}
	root.AddCommand(&cobra.Command{Use: "work", Short: "visible", RunE: noopRunE})
	admin := &cobra.Command{Use: "admin", Hidden: true, RunE: noopRunE}
	admin.AddCommand(&cobra.Command{Use: "reset", Short: "visible child of a hidden parent", RunE: noopRunE})
	root.AddCommand(admin)
	visible := &cobra.Command{Use: "parent", RunE: noopRunE}
	visible.AddCommand(&cobra.Command{Use: "nested-hidden", Hidden: true, RunE: noopRunE})
	root.AddCommand(visible)

	got := BuildCommand(root)
	var uses []string
	for _, child := range got.Commands {
		uses = append(uses, child.Use)
	}
	for _, excluded := range []string{"admin", "reset"} {
		for _, use := range uses {
			if use == excluded {
				t.Errorf("hidden command %q leaked into the schema; got children %v", excluded, uses)
			}
		}
	}
	if len(got.Commands) != 2 {
		t.Errorf("Commands = %v, want exactly the 2 visible children", uses)
	}
	for _, child := range got.Commands {
		if child.Use == "parent" && len(child.Commands) != 0 {
			t.Errorf("nested hidden child leaked under %q: %+v", child.Use, child.Commands)
		}
	}
}

// TestBuildCommandKeepsReservedNamedCommands pins the division of
// responsibility between the schema and MCP adapters: BuildCommand filters
// only on Hidden and performs no reserved-name filtering, so __schema,
// mcp-server, and completion commands DO appear here — excluding them from
// the MCP tool set is internal/mcp's job.
func TestBuildCommandKeepsReservedNamedCommands(t *testing.T) {
	root := &cobra.Command{Use: "demo", RunE: noopRunE}
	for _, name := range []string{"__schema", "mcp-server", "completion"} {
		root.AddCommand(&cobra.Command{Use: name, RunE: noopRunE})
	}

	got := BuildCommand(root)
	if len(got.Commands) != 3 {
		t.Fatalf("Commands has %d entries, want all 3 reserved-named commands present", len(got.Commands))
	}
}

// TestCollectFlags pins the Flag reflection shape for the common pflag types:
// name, shorthand, pflag type string, default value in its string form, usage,
// and the required bit.
func TestCollectFlags(t *testing.T) {
	cases := []struct {
		name     string
		register func(cmd *cobra.Command)
		want     Flag
	}{
		{
			name: "string flag with shorthand",
			register: func(cmd *cobra.Command) {
				cmd.Flags().StringP("name", "n", "world", "name to greet")
			},
			want: Flag{Name: "name", Shorthand: "n", Type: "string", Default: "world", Usage: "name to greet"},
		},
		{
			name: "bool flag",
			register: func(cmd *cobra.Command) {
				cmd.Flags().Bool("verbose", false, "verbose output")
			},
			want: Flag{Name: "verbose", Type: "bool", Default: "false", Usage: "verbose output"},
		},
		{
			name: "int flag",
			register: func(cmd *cobra.Command) {
				cmd.Flags().Int("times", 3, "repeat count")
			},
			want: Flag{Name: "times", Type: "int", Default: "3", Usage: "repeat count"},
		},
		{
			name: "duration flag",
			register: func(cmd *cobra.Command) {
				cmd.Flags().Duration("timeout", time.Minute, "call timeout")
			},
			want: Flag{Name: "timeout", Type: "duration", Default: "1m0s", Usage: "call timeout"},
		},
		{
			name: "required flag carries the required bit",
			register: func(cmd *cobra.Command) {
				cmd.Flags().String("target", "", "deploy target")
				if err := cmd.MarkFlagRequired("target"); err != nil {
					t.Fatalf("mark flag required: %v", err)
				}
			},
			want: Flag{Name: "target", Type: "string", Default: "", Usage: "deploy target", Required: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
			tc.register(cmd)

			flags := CollectFlags(cmd)
			if len(flags) != 1 {
				t.Fatalf("CollectFlags returned %d flags, want 1: %+v", len(flags), flags)
			}
			if !reflect.DeepEqual(flags[0], tc.want) {
				t.Errorf("flag = %+v, want %+v", flags[0], tc.want)
			}
		})
	}
}

// TestCollectFlagsInheritedAndDeduped asserts CollectFlags includes inherited
// persistent flags on a child command and dedupes a shadowed name, keeping the
// local (non-inherited) definition.
func TestCollectFlagsInheritedAndDeduped(t *testing.T) {
	root := &cobra.Command{Use: "demo", RunE: noopRunE}
	root.PersistentFlags().String("config", "inherited", "inherited config")
	root.PersistentFlags().String("token", "", "inherited token")
	child := &cobra.Command{Use: "child", RunE: noopRunE}
	child.Flags().String("config", "local", "local config")
	root.AddCommand(child)

	flags := CollectFlags(child)
	byName := map[string]Flag{}
	for _, flag := range flags {
		if _, dup := byName[flag.Name]; dup {
			t.Errorf("flag %q collected twice: %+v", flag.Name, flags)
		}
		byName[flag.Name] = flag
	}
	if len(flags) != 2 {
		t.Errorf("CollectFlags returned %d flags, want 2 (local config + inherited token): %+v", len(flags), flags)
	}
	if got := byName["config"].Default; got != "local" {
		t.Errorf("shadowed config default = %q, want the local definition %q", got, "local")
	}
	if _, present := byName["token"]; !present {
		t.Errorf("inherited flag %q missing: %+v", "token", flags)
	}
}

// TestIsRequiredFlag pins the required-annotation contract directly at the
// newly exported helper: local and persistent required annotations report
// true; unmarked, un-annotated, or explicitly false annotations report false.
func TestIsRequiredFlag(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T) *pflag.Flag
		want  bool
	}{
		{
			name: "MarkFlagRequired reports true",
			build: func(t *testing.T) *pflag.Flag {
				t.Helper()
				cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
				cmd.Flags().String("target", "", "t")
				if err := cmd.MarkFlagRequired("target"); err != nil {
					t.Fatalf("mark flag required: %v", err)
				}
				return cmd.Flags().Lookup("target")
			},
			want: true,
		},
		{
			name: "MarkPersistentFlagRequired reports true",
			build: func(t *testing.T) *pflag.Flag {
				t.Helper()
				cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
				cmd.PersistentFlags().String("token", "", "t")
				if err := cmd.MarkPersistentFlagRequired("token"); err != nil {
					t.Fatalf("mark persistent flag required: %v", err)
				}
				return cmd.PersistentFlags().Lookup("token")
			},
			want: true,
		},
		{
			name: "unmarked flag reports false",
			build: func(t *testing.T) *pflag.Flag {
				t.Helper()
				cmd := &cobra.Command{Use: "demo", RunE: noopRunE}
				cmd.Flags().String("tag", "", "t")
				return cmd.Flags().Lookup("tag")
			},
			want: false,
		},
		{
			name: "annotation explicitly false reports false",
			build: func(t *testing.T) *pflag.Flag {
				t.Helper()
				return &pflag.Flag{
					Name:        "tag",
					Annotations: map[string][]string{cobra.BashCompOneRequiredFlag: {"false"}},
				}
			},
			want: false,
		},
		{
			name: "nil annotations report false",
			build: func(t *testing.T) *pflag.Flag {
				t.Helper()
				return &pflag.Flag{Name: "tag"}
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRequiredFlag(tc.build(t)); got != tc.want {
				t.Errorf("IsRequiredFlag() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBuildCommandIsDeterministic asserts two BuildCommand runs over the same
// tree produce deep-equal schemas, with children in Cobra's default
// name-sorted order regardless of registration order — the ax __schema output
// is stable-by-contract.
func TestBuildCommandIsDeterministic(t *testing.T) {
	newTree := func() *cobra.Command {
		root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
		root.AddCommand(&cobra.Command{Use: "zeta", RunE: noopRunE})
		root.AddCommand(&cobra.Command{Use: "alpha", RunE: noopRunE})
		work := &cobra.Command{Use: "work", RunE: noopRunE}
		work.Flags().String("target", "", "deploy target")
		root.AddCommand(work)
		return root
	}

	first := BuildCommand(newTree())
	second := BuildCommand(newTree())
	if !reflect.DeepEqual(first, second) {
		t.Errorf("BuildCommand differs across runs:\nfirst:  %+v\nsecond: %+v", first, second)
	}
	var order []string
	for _, child := range first.Commands {
		order = append(order, child.Use)
	}
	if want := []string{"alpha", "work", "zeta"}; !reflect.DeepEqual(order, want) {
		t.Errorf("child order = %v, want name-sorted %v", order, want)
	}
}

// TestWalkCommandsVisitsEveryDescendant pins WalkCommands as the unfiltered
// walk: unlike BuildCommand it prunes nothing — hidden commands are visited
// too — in pre-order.
func TestWalkCommandsVisitsEveryDescendant(t *testing.T) {
	root := &cobra.Command{Use: "demo", RunE: noopRunE}
	hidden := &cobra.Command{Use: "admin", Hidden: true, RunE: noopRunE}
	hidden.AddCommand(&cobra.Command{Use: "reset", RunE: noopRunE})
	root.AddCommand(hidden)
	root.AddCommand(&cobra.Command{Use: "work", RunE: noopRunE})

	var visited []string
	WalkCommands(root, func(cmd *cobra.Command) {
		visited = append(visited, cmd.Name())
	})

	want := []string{"demo", "admin", "reset", "work"}
	if !reflect.DeepEqual(visited, want) {
		t.Errorf("visited = %v, want pre-order walk over every command (hidden included) %v", visited, want)
	}
}
