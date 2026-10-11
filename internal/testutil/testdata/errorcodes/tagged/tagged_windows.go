//go:build ax_no_grpc

package tagged

import (
	"context"

	"github.com/rshade/ax-go/contract"
)

func emit(ctx context.Context) *contract.Error {
	return contract.NewError(ctx, "tagged_literal", "")
}
