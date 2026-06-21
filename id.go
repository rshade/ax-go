package ax

import isolatedid "github.com/rshade/ax-go/id"

// NewIdempotencyKey returns a UUID v4 idempotency key.
func NewIdempotencyKey() string {
	return isolatedid.NewIdempotencyKey()
}

// NewEntityID returns a UUID v7 resource/entity identifier.
func NewEntityID() (string, error) {
	return isolatedid.NewEntityID()
}
