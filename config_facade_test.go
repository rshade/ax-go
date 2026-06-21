package ax

import (
	"context"
	"errors"
	"strings"
	"testing"

	isolatedconfig "github.com/rshade/ax-go/config"
	"github.com/rshade/ax-go/contract"
)

func TestRootConfigFacadeUsesIsolatedOptionType(t *testing.T) {
	var option isolatedconfig.Option = WithMaxConfigBytes(1)
	var got struct{}
	err := ParseConfig(context.Background(), strings.NewReader("{}"), &got, option)
	var contractErr *contract.Error
	if !errors.As(err, &contractErr) {
		t.Fatalf("error type = %T, want *contract.Error", err)
	}
	if contractErr.ErrorCode != "config_too_large" {
		t.Fatalf("ErrorCode = %q, want config_too_large", contractErr.ErrorCode)
	}
}

func TestRootConfigFacadeConstantsMatchIsolatedPackage(t *testing.T) {
	if DefaultMaxConfigBytes != isolatedconfig.DefaultMaxBytes {
		t.Fatalf("DefaultMaxConfigBytes = %d, want %d", DefaultMaxConfigBytes, isolatedconfig.DefaultMaxBytes)
	}
	if MaxConfigBytesCeiling != isolatedconfig.MaxBytesCeiling {
		t.Fatalf("MaxConfigBytesCeiling = %d, want %d", MaxConfigBytesCeiling, isolatedconfig.MaxBytesCeiling)
	}
}
