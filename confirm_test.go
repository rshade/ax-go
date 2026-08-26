package ax

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestConfirmDecision(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		want       ConfirmationOutcome
		wantCode   string
		wantExit   int
		wantAction string
	}{
		{
			name: "approved in machine mode",
			ctx:  WithApproval(WithMode(context.Background(), ModeJSON), true),
			want: ConfirmationApproved,
		},
		{
			name:       "blocked in machine mode",
			ctx:        WithMode(context.Background(), ModeJSON),
			want:       ConfirmationBlocked,
			wantCode:   "confirmation_required",
			wantExit:   ExitValidation,
			wantAction: "pass --yes to confirm this operation",
		},
		{
			name: "prompt required in human mode",
			ctx:  WithMode(context.Background(), ModeHuman),
			want: ConfirmationPromptRequired,
		},
		{
			name: "approval wins in human mode",
			ctx:  WithApproval(WithMode(context.Background(), ModeHuman), true),
			want: ConfirmationApproved,
		},
		{
			name:     "missing mode fails closed",
			ctx:      context.Background(),
			want:     ConfirmationBlocked,
			wantCode: "confirmation_required",
			wantExit: ExitValidation,
		},
		{
			name:     "nil context fails closed",
			ctx:      nil,
			want:     ConfirmationBlocked,
			wantCode: "confirmation_required",
			wantExit: ExitValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Confirm(tt.ctx, "delete the record")
			if got != tt.want {
				t.Fatalf("Confirm outcome = %v, want %v", got, tt.want)
			}
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("Confirm error = %v, want nil", err)
				}
				return
			}

			var axErr *Error
			if !errors.As(err, &axErr) {
				t.Fatalf("Confirm error = %T %v, want *Error", err, err)
			}
			if axErr.ErrorCode != tt.wantCode || axErr.ExitCode() != tt.wantExit {
				t.Fatalf(
					"error = (%q, %d), want (%q, %d)",
					axErr.ErrorCode,
					axErr.ExitCode(),
					tt.wantCode,
					tt.wantExit,
				)
			}
			if tt.wantAction != "" && axErr.ActionableFix != tt.wantAction {
				t.Errorf("actionable_fix = %q, want %q", axErr.ActionableFix, tt.wantAction)
			}
		})
	}
}

func TestConfirmIsStateFreeAndPerformsNoIO(t *testing.T) {
	ctx := WithMode(context.Background(), ModeJSON)
	var stdin, stdout, stderr bytes.Buffer
	stdin.WriteString("input")
	stdout.WriteString("payload")
	stderr.WriteString("diagnostic")

	first, firstErr := Confirm(ctx, "first")
	second, secondErr := Confirm(ctx, "second")
	if first != ConfirmationBlocked || second != ConfirmationBlocked || firstErr == nil ||
		secondErr == nil {
		t.Fatalf(
			"repeated Confirm results = (%v, %v), (%v, %v); want blocked errors",
			first,
			firstErr,
			second,
			secondErr,
		)
	}
	if stdin.String() != "input" || stdout.String() != "payload" ||
		stderr.String() != "diagnostic" {
		t.Fatal("Confirm changed an I/O buffer")
	}
}

func TestConfirmHumanModeLeavesPromptToCaller(t *testing.T) {
	outcome, err := Confirm(WithMode(context.Background(), ModeHuman), "delete the record")
	if err != nil {
		t.Fatalf("Confirm error = %v, want nil", err)
	}
	if outcome != ConfirmationPromptRequired {
		t.Fatalf("Confirm outcome = %v, want ConfirmationPromptRequired", outcome)
	}
	if outcome == ConfirmationApproved || outcome == ConfirmationBlocked {
		t.Fatal("prompt-required outcome is not distinct")
	}
}
