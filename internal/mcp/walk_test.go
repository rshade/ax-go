package mcp

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

// walkPaths walks cmd with WalkCallableCommands and returns the visited
// command paths in visit order.
func walkPaths(cmd *cobra.Command) []string {
	var paths []string
	WalkCallableCommands(cmd, func(c *cobra.Command) {
		paths = append(paths, c.CommandPath())
	})
	return paths
}

// TestWalkCallableCommandsOrderIsDeterministic pins the visit order: pre-order
// depth-first over Cobra's default name-sorted children, stable across runs so
// the emitted tool list never permutes between invocations (output
// determinism is a core ax-go mandate).
func TestWalkCallableCommandsOrderIsDeterministic(t *testing.T) {
	root := &cobra.Command{Use: "demo", RunE: noopRunE}
	root.AddCommand(&cobra.Command{Use: "zeta", RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "alpha", RunE: noopRunE})
	mid := &cobra.Command{Use: "mid", RunE: noopRunE}
	mid.AddCommand(&cobra.Command{Use: "leaf", RunE: noopRunE})
	root.AddCommand(mid)

	want := []string{"demo", "demo alpha", "demo mid", "demo mid leaf", "demo zeta"}

	first := walkPaths(root)
	if !slices.Equal(first, want) {
		t.Errorf("visit order = %v, want %v", first, want)
	}
	for run := range 3 {
		if got := walkPaths(root); !slices.Equal(got, first) {
			t.Fatalf("run %d visit order = %v, want stable %v", run, got, first)
		}
	}
}

// TestWalkCallableCommandsReservedMatchingIsExactLeafName pins the reserved
// exclusion semantics: matching is on the exact leaf command name at any
// depth, so a nested __schema is excluded while lookalike names
// (completions, mcp-serverx, schema) stay callable.
func TestWalkCallableCommandsReservedMatchingIsExactLeafName(t *testing.T) {
	root := &cobra.Command{Use: "demo", RunE: noopRunE}
	group := &cobra.Command{Use: "group", RunE: noopRunE}
	group.AddCommand(&cobra.Command{Use: "__schema", RunE: noopRunE})
	group.AddCommand(&cobra.Command{Use: "mcp-server", RunE: noopRunE})
	root.AddCommand(group)
	root.AddCommand(&cobra.Command{Use: "completions", RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "mcp-serverx", RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "schema", RunE: noopRunE})

	paths := walkPaths(root)
	for _, excluded := range []string{"demo group __schema", "demo group mcp-server"} {
		if slices.Contains(paths, excluded) {
			t.Errorf("reserved command %q leaked into the walk; got %v", excluded, paths)
		}
	}
	for _, want := range []string{"demo completions", "demo mcp-serverx", "demo schema"} {
		if !slices.Contains(paths, want) {
			t.Errorf("lookalike command %q was wrongly excluded; got %v", want, paths)
		}
	}
}

// TestWalkCallableCommandsHiddenRootVisitsNothing pins the edge case where the
// root itself is hidden: the prune happens before the first visit, so the walk
// visits nothing at all.
func TestWalkCallableCommandsHiddenRootVisitsNothing(t *testing.T) {
	root := &cobra.Command{Use: "demo", Hidden: true, RunE: noopRunE}
	root.AddCommand(&cobra.Command{Use: "child", RunE: noopRunE})

	if paths := walkPaths(root); len(paths) != 0 {
		t.Errorf("walk of a hidden root visited %v, want nothing", paths)
	}
}

// TestBuildIsDeterministic asserts two Build runs over the same tree emit
// byte-comparable tool metadata: same tools, same order.
func TestBuildIsDeterministic(t *testing.T) {
	newTree := func() *cobra.Command {
		root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
		work := &cobra.Command{Use: "work", Short: "real work", RunE: noopRunE}
		work.Flags().String("target", "", "deploy target")
		work.Flags().Bool("force", false, "force the operation")
		root.AddCommand(work)
		root.AddCommand(&cobra.Command{Use: "admin", Short: "hidden", Hidden: true, RunE: noopRunE})
		return root
	}

	first := Build(newTree())
	second := Build(newTree())
	if len(first.Tools) == 0 {
		t.Fatal("Build returned no tools")
	}
	firstNames := make([]string, 0, len(first.Tools))
	secondNames := make([]string, 0, len(second.Tools))
	for i := range first.Tools {
		firstNames = append(firstNames, first.Tools[i].Name)
	}
	for i := range second.Tools {
		secondNames = append(secondNames, second.Tools[i].Name)
	}
	if !slices.Equal(firstNames, secondNames) {
		t.Errorf("tool order differs across runs: %v vs %v", firstNames, secondNames)
	}
}

// TestWalkCallableCommandsSkipRules pins the node-only and subtree skip rules:
// a non-runnable or excluded command is skipped without pruning its children,
// while a hidden or reserved command (help included) prunes its whole subtree.
func TestWalkCallableCommandsSkipRules(t *testing.T) {
	tests := []struct {
		name  string
		build func() *cobra.Command
		want  []string
	}{
		{
			name: "non-runnable group skipped, runnable child kept",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo", RunE: noopRunE}
				group := &cobra.Command{Use: "group"}
				group.AddCommand(&cobra.Command{Use: "child", RunE: noopRunE})
				root.AddCommand(group)
				return root
			},
			want: []string{"demo", "demo group child"},
		},
		{
			name: "non-runnable root skipped, descendants kept",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo"}
				root.AddCommand(&cobra.Command{Use: "alpha", RunE: noopRunE})
				group := &cobra.Command{Use: "group"}
				group.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
				root.AddCommand(group)
				return root
			},
			want: []string{"demo alpha", "demo group child"},
		},
		{
			name: "cobra default help command skipped",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo", RunE: noopRunE}
				root.AddCommand(&cobra.Command{Use: "work", RunE: noopRunE})
				root.InitDefaultHelpCmd()
				return root
			},
			want: []string{"demo", "demo work"},
		},
		{
			name: "adopter-defined help subtree pruned",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo", RunE: noopRunE}
				help := &cobra.Command{Use: "help", RunE: noopRunE}
				help.AddCommand(&cobra.Command{Use: "topics", RunE: noopRunE})
				root.AddCommand(help)
				return root
			},
			want: []string{"demo"},
		},
		{
			name: "excluded runnable root skipped, children kept",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo", RunE: noopRunE}
				MarkExcluded(root)
				root.AddCommand(&cobra.Command{Use: "alpha", RunE: noopRunE})
				root.AddCommand(&cobra.Command{Use: "beta", RunE: noopRunE})
				return root
			},
			want: []string{"demo alpha", "demo beta"},
		},
		{
			name: "excluded leaf skipped",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo", RunE: noopRunE}
				serve := &cobra.Command{Use: "serve", RunE: noopRunE}
				MarkExcluded(serve)
				root.AddCommand(serve)
				root.AddCommand(&cobra.Command{Use: "work", RunE: noopRunE})
				return root
			},
			want: []string{"demo", "demo work"},
		},
		{
			name: "excluded parent with excluded and plain children",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo", RunE: noopRunE}
				parent := &cobra.Command{Use: "parent", RunE: noopRunE}
				MarkExcluded(parent)
				skipped := &cobra.Command{Use: "skipped", RunE: noopRunE}
				MarkExcluded(skipped)
				parent.AddCommand(skipped)
				parent.AddCommand(&cobra.Command{Use: "kept", RunE: noopRunE})
				root.AddCommand(parent)
				return root
			},
			want: []string{"demo", "demo parent kept"},
		},
		{
			name: "hidden and excluded behaves like hidden",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo", RunE: noopRunE}
				admin := &cobra.Command{Use: "admin", Hidden: true, RunE: noopRunE}
				MarkExcluded(admin)
				admin.AddCommand(&cobra.Command{Use: "reset", RunE: noopRunE})
				root.AddCommand(admin)
				return root
			},
			want: []string{"demo"},
		},
		{
			name: "only runnable commands excluded yields nothing",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo"}
				leaf := &cobra.Command{Use: "leaf", RunE: noopRunE}
				MarkExcluded(leaf)
				root.AddCommand(leaf)
				return root
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := walkPaths(tt.build()); !slices.Equal(got, tt.want) {
				t.Errorf("visited %v, want %v", got, tt.want)
			}
		})
	}
}
