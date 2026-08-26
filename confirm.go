package ax

import (
	"context"
	"fmt"
)

// ConfirmationOutcome is the caller-visible result of a confirmation gate.
type ConfirmationOutcome uint8

const (
	// ConfirmationApproved means explicit approval was supplied and the caller
	// may proceed without presenting a prompt.
	ConfirmationApproved ConfirmationOutcome = iota
	// ConfirmationBlocked means the run is non-interactive and requires --yes.
	ConfirmationBlocked
	// ConfirmationPromptRequired means a human-mode caller may own the prompt.
	ConfirmationPromptRequired
)

// Confirm evaluates a stateless confirmation gate without performing I/O.
// Explicit approval always succeeds. Without approval, human mode returns
// ConfirmationPromptRequired; machine mode and a missing mode return
// ConfirmationBlocked with a validation error (exit code 2) naming --yes.
func Confirm(ctx context.Context, subject string) (ConfirmationOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ApprovalFromContext(ctx) {
		return ConfirmationApproved, nil
	}
	if mode, ok := ModeFromContext(ctx); ok && mode == ModeHuman {
		return ConfirmationPromptRequired, nil
	}
	return ConfirmationBlocked, NewError(
		ctx,
		"confirmation_required",
		fmt.Sprintf("confirmation required: %s", subject),
		WithActionableFix("pass --yes to confirm this operation"),
		WithErrorExitCode(ExitValidation),
	)
}
