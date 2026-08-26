# Public API Contract

## Approval context

```go
func WithApproval(ctx context.Context, granted bool) context.Context
func ApprovalFromContext(ctx context.Context) bool
```

`WithApproval` returns a derived context carrying the per-run decision.
`ApprovalFromContext` returns false when the carrier is absent. The root `ax`
package exposes these functions and the import-isolated `contract` package owns
the carrier implementation.

## Confirmation gate

```go
type ConfirmationOutcome uint8

const (
	ConfirmationApproved ConfirmationOutcome = iota
	ConfirmationBlocked
	ConfirmationPromptRequired
)

func Confirm(ctx context.Context, subject string) (ConfirmationOutcome, error)
```

Contract:

- Approval true returns `ConfirmationApproved, nil` in either mode.
- Approval false plus `ModeJSON` returns `ConfirmationBlocked` and an `*ax.Error`
  with `error_code=confirmation_required`, `actionable_fix` naming `--yes`, and
  exit code 2.
- Approval false plus `ModeHuman` returns `ConfirmationPromptRequired, nil`.
- Missing mode is fail-closed and follows the blocked result.
- The function performs no stdin/stdout/pager/editor/spinner I/O and never calls
  a prompt callback.
- The caller owns any human prompt and the subsequent effect.

The exported enum and constants need doc comments and the primary gate gets a
verified `ExampleConfirm`.
