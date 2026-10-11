package bad

import (
	"context"

	"github.com/rshade/ax-go"
	"github.com/rshade/ax-go/contract"
)

const localCode = "local_code"

func emit(ctx context.Context, code string) []*contract.Error {
	e := &contract.Error{}
	e.ErrorCode = "assign_literal"
	construct := contract.NewError
	_ = construct
	return []*contract.Error{
		contract.NewError(ctx, "string_literal", ""),
		contract.NewError(ctx, localCode, ""),
		contract.NewError(ctx, contract.ErrorCodeOrphan, ""),
		contract.NewError(ctx, contract.ModeLike, ""),
		contract.NewError(ctx, code, ""),
		contract.NewError(ctx, "con"+"cat", ""),
		ax.NewError(ctx, "root_literal", ""),
		{ErrorCode: "field_literal"},
		e,
	}
}

func pair() (string, error) { return "pair_code", nil }

func tuple(e *contract.Error) {
	e.ErrorCode, _ = pair()
}
