package ax

import (
	"context"

	"github.com/rshade/ax-go/contract"
)

func NewError(ctx context.Context, code, message string) *contract.Error {
	return contract.NewError(ctx, code, message)
}

func rootUnqualified(ctx context.Context) *contract.Error {
	return NewError(ctx, "root_unqualified", "")
}
