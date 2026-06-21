package contract

import (
	"context"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

func TestImportIsolation(t *testing.T) {
	testutil.AssertContractPackageIsolated(
		context.Background(),
		t,
		"github.com/rshade/ax-go/contract",
	)
}
