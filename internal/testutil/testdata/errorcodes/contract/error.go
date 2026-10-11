package contract

import "context"

type Error struct {
	ErrorCode string
}

func NewError(ctx context.Context, code, message string) *Error {
	_, _ = ctx, message
	return &Error{ErrorCode: code}
}
