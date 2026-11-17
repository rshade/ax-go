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
