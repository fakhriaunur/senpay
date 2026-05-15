package idempotency

import (
	"context"
)

// IdempotencyStore defines the interface for persisting idempotency key status.
// Implementations: PostgresIdempotencyStore (postgres.go).
type IdempotencyStore interface {
	// Insert creates a new idempotency key record with the given status.
	// Returns a unique constraint violation error if the key already exists.
	Insert(ctx context.Context, key string, status string) error

	// FindByKey retrieves the status of an idempotency key.
	// Returns empty string if the key is not found.
	FindByKey(ctx context.Context, key string) (string, error)

	// UpdateStatus updates the status of an existing idempotency key.
	// Returns error if the key doesn't exist.
	UpdateStatus(ctx context.Context, key string, status string) error
}
