package bad

import (
	"context"

	"github.com/rshade/ax-go/contract"
)

func excluded(ctx context.Context) *contract.Error {
	return contract.NewError(ctx, "excluded_literal", "")
}
