package good

import (
	"context"

	root "github.com/rshade/ax-go"
	c "github.com/rshade/ax-go/contract"
)

func aliased(ctx context.Context) []*c.Error {
	return []*c.Error{
		c.NewError(ctx, c.ErrorCodeKnown, ""),
		root.NewError(ctx, c.ErrorCodeKnown, ""),
	}
}
