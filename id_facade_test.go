package ax

import (
	"testing"

	"github.com/google/uuid"

	isolatedid "github.com/rshade/ax-go/id"
)

func TestRootIDFacadeMatchesIsolatedPackageStrategy(t *testing.T) {
	rootKey := NewIdempotencyKey()
	isolatedKey := isolatedid.NewIdempotencyKey()
	assertUUIDVersion(t, rootKey, 4)
	assertUUIDVersion(t, isolatedKey, 4)

	rootEntityID, err := NewEntityID()
	if err != nil {
		t.Fatalf("NewEntityID returned error: %v", err)
	}
	isolatedEntityID, err := isolatedid.NewEntityID()
	if err != nil {
		t.Fatalf("isolated NewEntityID returned error: %v", err)
	}
	assertUUIDVersion(t, rootEntityID, 7)
	assertUUIDVersion(t, isolatedEntityID, 7)
}

func assertUUIDVersion(t *testing.T, value string, want uuid.Version) {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("%q is not a UUID: %v", value, err)
	}
	if parsed.Version() != want {
		t.Fatalf("UUID version = %d, want %d", parsed.Version(), want)
	}
}
