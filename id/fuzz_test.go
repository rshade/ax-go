package id

import (
	"testing"

	"github.com/google/uuid"
)

func FuzzGeneratedIDs(f *testing.F) {
	f.Add(1)
	f.Fuzz(func(t *testing.T, _ int) {
		key := NewIdempotencyKey()
		parsedKey, err := uuid.Parse(key)
		if err != nil {
			t.Fatalf("NewIdempotencyKey %q is not a UUID: %v", key, err)
		}
		if parsedKey.Version() != 4 {
			t.Fatalf("NewIdempotencyKey version=%d, want 4", parsedKey.Version())
		}

		entityID, err := NewEntityID()
		if err != nil {
			t.Fatalf("NewEntityID returned error: %v", err)
		}
		parsedEntityID, err := uuid.Parse(entityID)
		if err != nil {
			t.Fatalf("NewEntityID %q is not a UUID: %v", entityID, err)
		}
		if parsedEntityID.Version() != 7 {
			t.Fatalf("NewEntityID version=%d, want 7", parsedEntityID.Version())
		}
	})
}
