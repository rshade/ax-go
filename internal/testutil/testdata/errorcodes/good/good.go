package good

import (
	"context"

	"github.com/rshade/ax-go"
	"github.com/rshade/ax-go/contract"
)

func emit(ctx context.Context) []*contract.Error {
	e := &contract.Error{}
	e.ErrorCode = contract.ErrorCodeKnown
	return []*contract.Error{
		contract.NewError(ctx, contract.ErrorCodeKnown, ""),
		ax.NewError(ctx, contract.ErrorCodeKnown, ""),
		{ErrorCode: contract.ErrorCodeKnown},
		e,
	}
}

func parenthesized(ctx context.Context) *contract.Error {
	return (contract.NewError)(ctx, (contract.ErrorCodeKnown), "")
}
