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
