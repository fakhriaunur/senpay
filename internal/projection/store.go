package projection

import (
	"context"

	"senpay/internal/types"

	"github.com/google/uuid"
)

// SnapshotStore defines the interface for persisting balance snapshots.
// Implementations: PostgresSnapshotStore (postgres.go).
type SnapshotStore interface {
	// Upsert creates or updates a balance snapshot with optimistic locking.
	// The snapshot's Version field is used for optimistic concurrency control:
	// if the version in the database doesn't match, the update is rejected.
	// On insert, version starts at 1.
	Upsert(ctx context.Context, snapshot types.BalanceSnapshot) error

	// FindByUserID retrieves the balance snapshot for a user.
	// Returns zero-value snapshot if not found (no error).
	FindByUserID(ctx context.Context, userID uuid.UUID) (types.BalanceSnapshot, error)
}
