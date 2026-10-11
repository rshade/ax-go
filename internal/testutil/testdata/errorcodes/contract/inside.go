package contract

import "context"

func inside(ctx context.Context) []*Error {
	return []*Error{
		NewError(ctx, ErrorCodeKnown, ""),
		NewError(ctx, "inside_literal", ""),
		NewError(ctx, unexportedCode, ""),
	}
}
