package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestFlagConstants pins the exact spellings of the persistent agent-safety
// flag names. Both ax.Execute's flag installer and the MCP dispatcher resolve
// these constants as a single source of truth, so a typo here silently forks
// the two paths — this test makes such a typo fail loudly.
func TestFlagConstants(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "FlagFormat", got: FlagFormat, want: "format"},
		{name: "FlagDryRun", got: FlagDryRun, want: "dry-run"},
		{name: "FlagIdempotencyKey", got: FlagIdempotencyKey, want: "idempotency-key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestEnsurePersistentStringFlag asserts the install contract: the flag lands
// on PersistentFlags (so children inherit it) with the given default and
// usage, and a second install is a no-op that never overwrites an existing
// flag of the same name.
func TestEnsurePersistentStringFlag(t *testing.T) {
	root := &cobra.Command{Use: "demo"}
	EnsurePersistentStringFlag(root, FlagFormat, "json", "output mode")

	flag := root.PersistentFlags().Lookup(FlagFormat)
	if flag == nil {
		t.Fatalf("persistent flag %q was not installed", FlagFormat)
	}
	if flag.DefValue != "json" || flag.Usage != "output mode" {
		t.Errorf("installed flag = (DefValue %q, Usage %q), want (%q, %q)",
			flag.DefValue, flag.Usage, "json", "output mode")
	}

	EnsurePersistentStringFlag(root, FlagFormat, "human", "overwritten usage")
	flag = root.PersistentFlags().Lookup(FlagFormat)
	if flag.DefValue != "json" || flag.Usage != "output mode" {
		t.Errorf("second install overwrote the flag: (DefValue %q, Usage %q), want the original (%q, %q)",
			flag.DefValue, flag.Usage, "json", "output mode")
	}
}

// TestEnsurePersistentBoolFlag mirrors TestEnsurePersistentStringFlag for the
// bool variant: installs on PersistentFlags with the given default, and a
// repeat install is a no-op.
func TestEnsurePersistentBoolFlag(t *testing.T) {
	root := &cobra.Command{Use: "demo"}
	EnsurePersistentBoolFlag(root, FlagDryRun, false, "emit without side effects")

	flag := root.PersistentFlags().Lookup(FlagDryRun)
	if flag == nil {
		t.Fatalf("persistent flag %q was not installed", FlagDryRun)
	}
	if flag.DefValue != "false" || flag.Value.Type() != "bool" {
		t.Errorf("installed flag = (DefValue %q, Type %q), want (%q, %q)",
			flag.DefValue, flag.Value.Type(), "false", "bool")
	}

	EnsurePersistentBoolFlag(root, FlagDryRun, true, "overwritten usage")
	flag = root.PersistentFlags().Lookup(FlagDryRun)
	if flag.DefValue != "false" {
		t.Errorf("second install overwrote the default: DefValue %q, want %q", flag.DefValue, "false")
	}
}

// TestLookupFlagString asserts resolution order and the miss contract: a local
// flag wins, an inherited (parent persistent) flag resolves next, and an
// unknown name returns "" rather than panicking.
func TestLookupFlagString(t *testing.T) {
	cases := []struct {
		name     string
		flagName string
		build    func() *cobra.Command
		want     string
	}{
		{
			name:     "local flag resolves",
			flagName: FlagFormat,
			build: func() *cobra.Command {
				cmd := &cobra.Command{Use: "demo"}
				cmd.Flags().String(FlagFormat, "json", "output mode")
				return cmd
			},
			want: "json",
		},
		{
			name:     "inherited persistent flag resolves on the child",
			flagName: FlagIdempotencyKey,
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo"}
				root.PersistentFlags().String(FlagIdempotencyKey, "key-1", "idempotency key")
				child := &cobra.Command{Use: "child"}
				root.AddCommand(child)
				return child
			},
			want: "key-1",
		},
		{
			name:     "missing flag resolves to empty string",
			flagName: FlagFormat,
			build:    func() *cobra.Command { return &cobra.Command{Use: "demo"} },
			want:     "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LookupFlagString(tc.build(), tc.flagName); got != tc.want {
				t.Errorf("LookupFlagString(%q) = %q, want %q", tc.flagName, got, tc.want)
			}
		})
	}
}

// TestLookupFlagBool asserts the parse-based bool resolution: bool flags
// resolve their value (locally or inherited), a missing or unparseable flag
// resolves false, and a string flag holding a parseable bool resolves through
// strconv.ParseBool.
func TestLookupFlagBool(t *testing.T) {
	cases := []struct {
		name  string
		build func() *cobra.Command
		want  bool
	}{
		{
			name: "local true bool resolves true",
			build: func() *cobra.Command {
				cmd := &cobra.Command{Use: "demo"}
				cmd.Flags().Bool(FlagDryRun, true, "dry run")
				return cmd
			},
			want: true,
		},
		{
			name: "local false bool resolves false",
			build: func() *cobra.Command {
				cmd := &cobra.Command{Use: "demo"}
				cmd.Flags().Bool(FlagDryRun, false, "dry run")
				return cmd
			},
			want: false,
		},
		{
			name: "inherited true bool resolves true on the child",
			build: func() *cobra.Command {
				root := &cobra.Command{Use: "demo"}
				root.PersistentFlags().Bool(FlagDryRun, true, "dry run")
				child := &cobra.Command{Use: "child"}
				root.AddCommand(child)
				return child
			},
			want: true,
		},
		{
			name:  "missing flag resolves false",
			build: func() *cobra.Command { return &cobra.Command{Use: "demo"} },
			want:  false,
		},
		{
			name: "unparseable string flag resolves false",
			build: func() *cobra.Command {
				cmd := &cobra.Command{Use: "demo"}
				cmd.Flags().String(FlagDryRun, "notabool", "misdeclared")
				return cmd
			},
			want: false,
		},
		{
			name: "string flag holding a parseable bool resolves through ParseBool",
			build: func() *cobra.Command {
				cmd := &cobra.Command{Use: "demo"}
				cmd.Flags().String(FlagDryRun, "true", "misdeclared")
				return cmd
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LookupFlagBool(tc.build(), FlagDryRun); got != tc.want {
				t.Errorf("LookupFlagBool(%q) = %v, want %v", FlagDryRun, got, tc.want)
			}
		})
	}
}
