package cli_test

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/cli"
)

func newRootCmd() *cobra.Command {
	return &cobra.Command{Use: "root"}
}

func TestEnsurePersistentStringFlag(t *testing.T) {
	t.Run("adds the flag when absent", func(t *testing.T) {
		root := newRootCmd()
		cli.EnsurePersistentStringFlag(root, cli.FlagFormat, "default", "usage text")

		flag := root.PersistentFlags().Lookup(cli.FlagFormat)
		if flag == nil {
			t.Fatal("expected flag to be added, got nil")
		}
		if flag.DefValue != "default" {
			t.Errorf("DefValue = %q, want %q", flag.DefValue, "default")
		}
		if flag.Usage != "usage text" {
			t.Errorf("Usage = %q, want %q", flag.Usage, "usage text")
		}
	})

	t.Run("is a no-op when the flag already exists", func(t *testing.T) {
		root := newRootCmd()
		root.PersistentFlags().String(cli.FlagFormat, "original", "original usage")

		cli.EnsurePersistentStringFlag(root, cli.FlagFormat, "override", "override usage")

		flag := root.PersistentFlags().Lookup(cli.FlagFormat)
		if flag.DefValue != "original" {
			t.Errorf("DefValue = %q, want unchanged %q", flag.DefValue, "original")
		}
		if flag.Usage != "original usage" {
			t.Errorf("Usage = %q, want unchanged %q", flag.Usage, "original usage")
		}
	})
}

func TestEnsurePersistentBoolFlag(t *testing.T) {
	t.Run("adds the flag when absent", func(t *testing.T) {
		root := newRootCmd()
		cli.EnsurePersistentBoolFlag(root, cli.FlagDryRun, true, "usage text")

		flag := root.PersistentFlags().Lookup(cli.FlagDryRun)
		if flag == nil {
			t.Fatal("expected flag to be added, got nil")
		}
		if flag.DefValue != "true" {
			t.Errorf("DefValue = %q, want %q", flag.DefValue, "true")
		}
	})

	t.Run("is a no-op when the flag already exists", func(t *testing.T) {
		root := newRootCmd()
		root.PersistentFlags().Bool(cli.FlagDryRun, false, "original usage")

		cli.EnsurePersistentBoolFlag(root, cli.FlagDryRun, true, "override usage")

		flag := root.PersistentFlags().Lookup(cli.FlagDryRun)
		if flag.DefValue != "false" {
			t.Errorf("DefValue = %q, want unchanged %q", flag.DefValue, "false")
		}
	})
}

func TestLookupFlagString(t *testing.T) {
	t.Run("resolves a local flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}
		cmd.Flags().String("name", "local-value", "usage")

		if got := cli.LookupFlagString(cmd, "name"); got != "local-value" {
			t.Errorf("LookupFlagString() = %q, want %q", got, "local-value")
		}
	})

	t.Run("resolves an inherited persistent flag", func(t *testing.T) {
		root := newRootCmd()
		root.PersistentFlags().String(cli.FlagFormat, "inherited-value", "usage")
		child := &cobra.Command{Use: "child"}
		root.AddCommand(child)

		if got := cli.LookupFlagString(child, cli.FlagFormat); got != "inherited-value" {
			t.Errorf("LookupFlagString() = %q, want %q", got, "inherited-value")
		}
	})

	t.Run("returns empty string for an unknown flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}

		if got := cli.LookupFlagString(cmd, "missing"); got != "" {
			t.Errorf("LookupFlagString() = %q, want empty string", got)
		}
	})
}

func TestLookupFlagBool(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *cobra.Command
		want  bool
	}{
		{
			name: "true local flag",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "child"}
				cmd.Flags().Bool(cli.FlagDryRun, true, "usage")
				return cmd
			},
			want: true,
		},
		{
			name: "false local flag",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "child"}
				cmd.Flags().Bool(cli.FlagDryRun, false, "usage")
				return cmd
			},
			want: false,
		},
		{
			name: "inherited persistent flag",
			setup: func() *cobra.Command {
				root := newRootCmd()
				root.PersistentFlags().Bool(cli.FlagDryRun, true, "usage")
				child := &cobra.Command{Use: "child"}
				root.AddCommand(child)
				return child
			},
			want: true,
		},
		{
			name: "missing flag defaults to false",
			setup: func() *cobra.Command {
				return &cobra.Command{Use: "child"}
			},
			want: false,
		},
		{
			name: "non-boolean flag value defaults to false",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "child"}
				cmd.Flags().String(cli.FlagDryRun, "not-a-bool", "usage")
				return cmd
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.setup()
			if got := cli.LookupFlagBool(cmd, cli.FlagDryRun); got != tc.want {
				t.Errorf("LookupFlagBool() = %v, want %v", got, tc.want)
			}
		})
	}
}
