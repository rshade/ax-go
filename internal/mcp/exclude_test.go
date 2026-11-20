package mcp

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestIsExcludedRequiresCanonicalValue pins fail-closed parsing: only the
// exact canonical value excludes, so a typo can never silently drop a tool.
func TestIsExcludedRequiresCanonicalValue(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "nil annotations", annotations: nil, want: false},
		{name: "key absent", annotations: map[string]string{"other": "true"}, want: false},
		{name: "canonical value", annotations: map[string]string{ExcludeAnnotationKey: "true"}, want: true},
		{name: "uppercase value", annotations: map[string]string{ExcludeAnnotationKey: "TRUE"}, want: false},
		{name: "numeric value", annotations: map[string]string{ExcludeAnnotationKey: "1"}, want: false},
		{name: "empty value", annotations: map[string]string{ExcludeAnnotationKey: ""}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo", Annotations: tt.annotations}
			if got := IsExcluded(cmd); got != tt.want {
				t.Errorf("IsExcluded = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsExcludedNilCommand(t *testing.T) {
	if IsExcluded(nil) {
		t.Error("IsExcluded(nil) = true, want false")
	}
}

func TestMarkExcludedNilCommandIsNoop(t *testing.T) {
	MarkExcluded(nil)
}

func TestMarkExcludedAllocatesAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	MarkExcluded(cmd)

	if got := cmd.Annotations[ExcludeAnnotationKey]; got != "true" {
		t.Errorf("annotation value = %q, want %q", got, "true")
	}
	if !IsExcluded(cmd) {
		t.Error("IsExcluded after MarkExcluded = false, want true")
	}
}

func TestMarkExcludedPreservesExistingAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "demo", Annotations: map[string]string{"keep": "me"}}
	MarkExcluded(cmd)

	if got := cmd.Annotations["keep"]; got != "me" {
		t.Errorf("existing annotation = %q, want %q", got, "me")
	}
	if len(cmd.Annotations) != 2 {
		t.Errorf("annotations = %v, want exactly the existing key plus the exclusion key", cmd.Annotations)
	}
}

// TestMarkExcludedOverwritesNonCanonicalValue asserts marking a command that
// carries a typo'd value repairs it to the canonical value.
func TestMarkExcludedOverwritesNonCanonicalValue(t *testing.T) {
	cmd := &cobra.Command{Use: "demo", Annotations: map[string]string{ExcludeAnnotationKey: "yes"}}
	MarkExcluded(cmd)

	if !IsExcluded(cmd) {
		t.Error("IsExcluded after MarkExcluded = false, want true")
	}
}

// TestIsFlagExcludedRequiresCanonicalValue pins fail-closed parsing for pflag
// annotations: only a single-element slice equal to the canonical value
// excludes, so a typo or malformed slice can never silently drop a flag.
func TestIsFlagExcludedRequiresCanonicalValue(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string][]string
		want        bool
	}{
		{name: "nil annotations", annotations: nil, want: false},
		{name: "key absent", annotations: map[string][]string{"other": {"true"}}, want: false},
		{name: "canonical value", annotations: map[string][]string{ExcludeAnnotationKey: {"true"}}, want: true},
		{name: "uppercase value", annotations: map[string][]string{ExcludeAnnotationKey: {"TRUE"}}, want: false},
		{name: "numeric value", annotations: map[string][]string{ExcludeAnnotationKey: {"1"}}, want: false},
		{name: "empty string value", annotations: map[string][]string{ExcludeAnnotationKey: {""}}, want: false},
		{name: "empty slice", annotations: map[string][]string{ExcludeAnnotationKey: {}}, want: false},
		{
			name:        "multiple elements",
			annotations: map[string][]string{ExcludeAnnotationKey: {"true", "true"}},
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &pflag.Flag{Name: "watch", Annotations: tt.annotations}
			if got := IsFlagExcluded(flag); got != tt.want {
				t.Errorf("IsFlagExcluded = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFlagExcludedNilFlag(t *testing.T) {
	if IsFlagExcluded(nil) {
		t.Error("IsFlagExcluded(nil) = true, want false")
	}
}

func TestMarkFlagExcludedNoopCases(t *testing.T) {
	MarkFlagExcluded(nil, "watch")

	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Bool("watch", false, "watch")
	MarkFlagExcluded(cmd, "")
	MarkFlagExcluded(cmd, "unknown")

	if IsFlagExcluded(cmd.Flags().Lookup("watch")) {
		t.Error("watch flag unexpectedly excluded after no-op calls")
	}
}

func TestMarkFlagExcludedLocalAndPersistent(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Bool("watch", false, "local watch")
	cmd.PersistentFlags().Bool("interactive", false, "persistent interactive")

	MarkFlagExcluded(cmd, "watch")
	MarkFlagExcluded(cmd, "interactive")

	if !IsFlagExcluded(cmd.Flags().Lookup("watch")) {
		t.Error("local watch flag was not marked excluded")
	}
	if !IsFlagExcluded(cmd.PersistentFlags().Lookup("interactive")) {
		t.Error("persistent interactive flag was not marked excluded")
	}
}

func TestMarkFlagExcludedPreservesExistingAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Bool("watch", false, "watch")
	flag := cmd.Flags().Lookup("watch")
	flag.Annotations = map[string][]string{"keep": {"me"}}

	MarkFlagExcluded(cmd, "watch")

	if got := flag.Annotations["keep"]; len(got) != 1 || got[0] != "me" {
		t.Errorf("existing annotation = %v, want [me]", got)
	}
	if !IsFlagExcluded(flag) {
		t.Error("IsFlagExcluded after MarkFlagExcluded = false, want true")
	}
	if len(flag.Annotations) != 2 {
		t.Errorf("annotations = %v, want 2 keys", flag.Annotations)
	}
}

func TestMarkFlagExcludedOverwritesNonCanonicalValue(t *testing.T) {
	cmd := &cobra.Command{Use: "demo"}
	cmd.Flags().Bool("watch", false, "watch")
	flag := cmd.Flags().Lookup("watch")
	flag.Annotations = map[string][]string{ExcludeAnnotationKey: {"yes"}}

	MarkFlagExcluded(cmd, "watch")

	if !IsFlagExcluded(flag) {
		t.Error("IsFlagExcluded after MarkFlagExcluded = false, want true")
	}
}

// TestMarkFlagExcludedSkipsInheritedFlag pins that excluding a name the
// command only inherits is a no-op whether or not Cobra has already merged
// the ancestor's persistent flags into the child's flag set: the shared
// *pflag.Flag must never be marked through a descendant, or the exclusion
// would leak onto the ancestor and every sibling.
func TestMarkFlagExcludedSkipsInheritedFlag(t *testing.T) {
	for _, tc := range []struct {
		name     string
		discover bool
	}{
		{name: "before flag discovery", discover: false},
		{name: "after flag discovery", discover: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "demo"}
			root.PersistentFlags().Bool("watch", false, "watch")
			child := &cobra.Command{Use: "child"}
			sibling := &cobra.Command{Use: "sibling"}
			root.AddCommand(child, sibling)
			if tc.discover {
				_ = child.InheritedFlags()
			}

			MarkFlagExcluded(child, "watch")

			if IsFlagExcluded(root.PersistentFlags().Lookup("watch")) {
				t.Error("inherited watch flag was marked excluded through a child")
			}
		})
	}
}

// TestMarkFlagExcludedOwnPersistentAfterDiscovery pins that a command's own
// persistent flag is still excludable after Cobra merged it into Flags().
func TestMarkFlagExcludedOwnPersistentAfterDiscovery(t *testing.T) {
	root := &cobra.Command{Use: "demo"}
	child := &cobra.Command{Use: "child"}
	child.PersistentFlags().Bool("watch", false, "watch")
	root.AddCommand(child)
	_ = child.InheritedFlags()

	MarkFlagExcluded(child, "watch")

	if !IsFlagExcluded(child.PersistentFlags().Lookup("watch")) {
		t.Error("child's own persistent watch flag was not marked excluded")
	}
}

// TestReservedFlagsNeverExcluded pins that the ax agent-safety and output
// flags stay available to MCP clients: MarkFlagExcluded ignores them, and a
// hand-set canonical annotation does not exclude them either.
func TestReservedFlagsNeverExcluded(t *testing.T) {
	for _, name := range []string{"format", "dry-run", "yes", "idempotency-key", "strict"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo"}
			cmd.PersistentFlags().String(name, "", name)
			flag := cmd.PersistentFlags().Lookup(name)

			MarkFlagExcluded(cmd, name)
			if _, ok := flag.Annotations[ExcludeAnnotationKey]; ok {
				t.Errorf("MarkFlagExcluded set the exclusion annotation on reserved flag %q", name)
			}

			flag.Annotations = map[string][]string{ExcludeAnnotationKey: {"true"}}
			if IsFlagExcluded(flag) {
				t.Errorf("IsFlagExcluded(%q) = true for a reserved flag, want false", name)
			}
		})
	}
}
