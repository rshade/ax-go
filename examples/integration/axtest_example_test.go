package main

import (
	"context"
	"strings"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/axtest"
)

// TestConfirmCommandThroughAxtest demonstrates axtest's canonical testing
// pattern against this CLI's own shipped "confirm" command — not a toy
// fixture — proving the pattern replaces what a real project would otherwise
// hand-roll (research.md's two-example decision). Run recognizes --dry-run
// and --yes exactly as ax.Execute mounts them in production, and RunAndDecode
// reaches the typed payload with no hand-declared wrapper struct.
func TestConfirmCommandThroughAxtest(t *testing.T) {
	ctx := context.Background()
	blockedRoot, _ := newRootCommand(strings.NewReader(""), "test", ax.NewEntityID)

	blocked := axtest.Run(
		ctx, t,
		blockedRoot,
		[]string{"confirm", "--format=json"},
	)
	if blocked.ExitCode != ax.ExitValidation {
		t.Fatalf(
			"without --yes: exit code = %d, want %d (ExitValidation); stderr=%s",
			blocked.ExitCode, ax.ExitValidation, blocked.Stderr,
		)
	}

	approvedRoot, _ := newRootCommand(strings.NewReader(""), "test", ax.NewEntityID)
	data, exitCode := axtest.RunAndDecode[confirmationPayload](
		ctx, t,
		approvedRoot,
		[]string{"confirm", "--format=json", "--yes"},
	)
	if exitCode != ax.ExitSuccess {
		t.Fatalf("with --yes: exit code = %d, want %d (ExitSuccess)", exitCode, ax.ExitSuccess)
	}
	if !data.Confirmed {
		t.Error("Confirmed = false, want true")
	}
}
