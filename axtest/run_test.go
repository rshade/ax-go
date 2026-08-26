package axtest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/axtest"
)

// TestRun covers spec.md's User Story 1 Acceptance Scenarios 1-4 against the
// shared newGreetCommand fixture: dry-run recognition, a blocked confirmation,
// an approved confirmation, and automatic idempotency-key generation (FR-004).
//
// The dry-run case still passes --yes: AGENTS.md states approval and --dry-run
// are orthogonal — a dry run of a confirmation-gated command is still blocked
// without approval — so this exercises the DryRun bit specifically, on a
// request that also happens to succeed.
func TestRun(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		wantExitCode       int
		wantStdoutEmpty    bool
		wantStderrContains string
		wantApproved       bool
		wantDryRun         bool
	}{
		{
			name:         "dry run with approval succeeds and reports no side effects",
			args:         []string{"greet", "--format=json", "--yes", "--dry-run"},
			wantExitCode: ax.ExitSuccess,
			wantApproved: true,
			wantDryRun:   true,
		},
		{
			name:               "confirmation-gated action without --yes is blocked",
			args:               []string{"greet", "--format=json"},
			wantExitCode:       ax.ExitValidation,
			wantStdoutEmpty:    true,
			wantStderrContains: "confirmation_required",
		},
		{
			name:         "confirmation-gated action with --yes is approved",
			args:         []string{"greet", "--format=json", "--yes"},
			wantExitCode: ax.ExitSuccess,
			wantApproved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := axtest.Run(context.Background(), t, newGreetCommand(t), tt.args)

			if result.ExitCode != tt.wantExitCode {
				t.Fatalf("ExitCode = %d, want %d; stderr=%s", result.ExitCode, tt.wantExitCode, result.Stderr)
			}
			if tt.wantStderrContains != "" && !bytes.Contains(result.Stderr, []byte(tt.wantStderrContains)) {
				t.Errorf("Stderr = %q, want substring %q", result.Stderr, tt.wantStderrContains)
			}
			if tt.wantStdoutEmpty {
				if len(result.Stdout) != 0 {
					t.Errorf("Stdout = %q, want empty", result.Stdout)
				}
				return
			}

			var envelope ax.Envelope[greetResult]
			if err := json.Unmarshal(result.Stdout, &envelope); err != nil {
				t.Fatalf("stdout was not a JSON envelope: %v; stdout=%s", err, result.Stdout)
			}
			if envelope.Data.Approved != tt.wantApproved {
				t.Errorf("Approved = %v, want %v", envelope.Data.Approved, tt.wantApproved)
			}
			if envelope.Data.DryRun != tt.wantDryRun {
				t.Errorf("DryRun = %v, want %v", envelope.Data.DryRun, tt.wantDryRun)
			}
			if envelope.Meta.IdempotencyKey == "" {
				t.Error("Meta.IdempotencyKey is empty, want an auto-generated key (FR-004)")
			}
		})
	}
}

// TestRunReturnsNormallyOnValidationFailure covers spec.md's Acceptance
// Scenario 5: a validation failure must not abort the test process itself.
// Reaching the assertion below already proves Run returned rather than
// panicking or calling t.Fatal.
func TestRunReturnsNormallyOnValidationFailure(t *testing.T) {
	result := axtest.Run(context.Background(), t, newGreetCommand(t), []string{"not-a-real-subcommand"})

	if result.ExitCode == ax.ExitSuccess {
		t.Fatalf("ExitCode = %d, want non-zero for an unknown subcommand", result.ExitCode)
	}
}

// TestRunSupportsRepeatedInvocationOnSameTree is the FR-008 regression guard:
// axtest.Run gets flag re-mounting safety for free by delegating to
// ax.Execute's existing cli.EnsurePersistentXFlag calls, but that must be
// asserted rather than assumed.
func TestRunSupportsRepeatedInvocationOnSameTree(t *testing.T) {
	ctx := context.Background()
	root := newGreetCommand(t)

	dryRun := axtest.Run(ctx, t, root, []string{"greet", "--format=json", "--yes", "--dry-run"})
	if dryRun.ExitCode != ax.ExitSuccess {
		t.Fatalf("first call: ExitCode = %d, want %d; stderr=%s", dryRun.ExitCode, ax.ExitSuccess, dryRun.Stderr)
	}
	var dryRunEnvelope ax.Envelope[greetResult]
	if err := json.Unmarshal(dryRun.Stdout, &dryRunEnvelope); err != nil {
		t.Fatalf("first call: stdout was not a JSON envelope: %v", err)
	}
	if !dryRunEnvelope.Data.DryRun {
		t.Error("first call: DryRun = false, want true")
	}

	// --dry-run=false is explicit: a reused tree's flag VALUES carry over from
	// the prior call (ordinary command-framework behavior, not Run's concern —
	// data-model.md's Execution Result lifecycle note), so the prior call's
	// --dry-run=true would otherwise still be set.
	approved := axtest.Run(ctx, t, root, []string{"greet", "--format=json", "--yes", "--dry-run=false"})
	if approved.ExitCode != ax.ExitSuccess {
		t.Fatalf("second call: ExitCode = %d, want %d; stderr=%s", approved.ExitCode, ax.ExitSuccess, approved.Stderr)
	}
	if strings.Contains(string(approved.Stderr), "unknown flag") {
		t.Fatalf("second call: agent-safety flags were not recognized on the reused tree; stderr=%s", approved.Stderr)
	}
	var approvedEnvelope ax.Envelope[greetResult]
	if err := json.Unmarshal(approved.Stdout, &approvedEnvelope); err != nil {
		t.Fatalf("second call: stdout was not a JSON envelope: %v", err)
	}
	if approvedEnvelope.Data.DryRun {
		t.Error("second call: DryRun = true, want false")
	}
}

// TestRunIsSafeForConcurrentUse is the regression guard for spec.md's Edge
// Case "multiple subtests run concurrently, each against its own command
// tree... must not rely on any shared or global state." go test -race only
// catches a race in a code path a test actually exercises concurrently, and no
// other test in this package calls Run from parallel subtests.
func TestRunIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	const concurrency = 8

	for i := range concurrency {
		t.Run(fmt.Sprintf("subtest-%d", i), func(t *testing.T) {
			t.Parallel()

			args := []string{"greet", "--format=json", "--yes"}
			wantDryRun := i%2 == 0
			if wantDryRun {
				args = append(args, "--dry-run")
			}

			result := axtest.Run(context.Background(), t, newGreetCommand(t), args)
			if result.ExitCode != ax.ExitSuccess {
				t.Fatalf("ExitCode = %d, want %d; stderr=%s", result.ExitCode, ax.ExitSuccess, result.Stderr)
			}
			var envelope ax.Envelope[greetResult]
			if err := json.Unmarshal(result.Stdout, &envelope); err != nil {
				t.Fatalf("stdout was not a JSON envelope: %v", err)
			}
			if envelope.Data.DryRun != wantDryRun {
				t.Errorf("DryRun = %v, want %v", envelope.Data.DryRun, wantDryRun)
			}
		})
	}
}
