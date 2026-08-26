package axtest_test

import (
	"testing"

	"github.com/spf13/cobra"

	ax "github.com/rshade/ax-go"
)

// greetResult is the toy command's success payload, shared across every
// story's tests as the type parameter for axtest.Decode and
// axtest.RunAndDecode.
type greetResult struct {
	Approved bool `json:"approved"`
	DryRun   bool `json:"dry_run"`
}

// newGreetCommand returns a fresh toy command tree with one confirmation-gated
// "greet" subcommand, mirroring execute_test.go's
// TestExecuteApprovalAndDryRunAreOrthogonal fixture exactly rather than
// inventing new confirmation-gating logic. It returns a fresh *cobra.Command
// tree on every call — never a shared package-level variable (Constitution
// Principle X, no mutable package-level state) — so each test gets an
// unmounted tree with no flag state left over from a prior call.
func newGreetCommand(t testing.TB) *cobra.Command {
	t.Helper()

	root := &cobra.Command{Use: "greeter"}
	root.AddCommand(&cobra.Command{
		Use: "greet",
		RunE: func(cmd *cobra.Command, _ []string) error {
			outcome, err := ax.Confirm(cmd.Context(), "greet")
			if err != nil {
				return err
			}
			return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), greetResult{
				Approved: outcome == ax.ConfirmationApproved,
				DryRun:   ax.DryRunFromContext(cmd.Context()),
			}))
		},
	})
	return root
}
