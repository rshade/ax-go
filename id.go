package ax

import "github.com/google/uuid"

// NewIdempotencyKey returns a UUID v4 idempotency key.
func NewIdempotencyKey() string {
	return uuid.NewString()
}

// NewEntityID returns a UUID v7 resource/entity identifier.
func NewEntityID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
