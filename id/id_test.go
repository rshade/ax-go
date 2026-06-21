package id

import (
	"testing"

	"github.com/google/uuid"
)

func TestIDStrategies(t *testing.T) {
	idempotencyKey := NewIdempotencyKey()
	parsedKey, err := uuid.Parse(idempotencyKey)
	if err != nil {
		t.Fatalf("idempotency key is not a UUID: %v", err)
	}
	if parsedKey.Version() != 4 {
		t.Fatalf("idempotency key version = %d, want 4", parsedKey.Version())
	}

	entityID, err := NewEntityID()
	if err != nil {
		t.Fatalf("NewEntityID returned error: %v", err)
	}
	parsedEntityID, err := uuid.Parse(entityID)
	if err != nil {
		t.Fatalf("entity ID is not a UUID: %v", err)
	}
	if parsedEntityID.Version() != 7 {
		t.Fatalf("entity ID version = %d, want 7", parsedEntityID.Version())
	}
}
