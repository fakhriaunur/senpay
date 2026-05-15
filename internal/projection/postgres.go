package projection

import (
	"context"
	"errors"
	"fmt"

	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSnapshotStore implements SnapshotStore backed by PostgreSQL.
type PostgresSnapshotStore struct {
	pool *pgxpool.Pool
}

// NewPostgresSnapshotStore creates a new PostgresSnapshotStore.
func NewPostgresSnapshotStore(pool *pgxpool.Pool) *PostgresSnapshotStore {
	return &PostgresSnapshotStore{pool: pool}
}

// Upsert creates or updates a balance snapshot with optimistic locking.
func (s *PostgresSnapshotStore) Upsert(ctx context.Context, snapshot types.BalanceSnapshot) error {
	// Try update first (optimistic lock on version).
	const updateQuery = `
		UPDATE balance_snapshot
		SET balance_sen = $1, version = version + 1, updated_at = $2
		WHERE user_id = $3 AND version = $4
	`
	result, err := s.pool.Exec(ctx, updateQuery,
		snapshot.BalanceSen, snapshot.UpdatedAt,
		snapshot.UserID, snapshot.Version,
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot update: %w", err)
	}

	if result.RowsAffected() > 0 {
		return nil // optimistic lock succeeded
	}

	// Update didn't match — either row doesn't exist or version mismatch.
	// Try insert (first time).
	const insertQuery = `
		INSERT INTO balance_snapshot (user_id, balance_sen, version, updated_at)
		VALUES ($1, $2, 1, $3)
		ON CONFLICT (user_id) DO NOTHING
	`
	result, err = s.pool.Exec(ctx, insertQuery,
		snapshot.UserID, snapshot.BalanceSen, snapshot.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot insert: %w", err)
	}

	if result.RowsAffected() > 0 {
		return nil // insert succeeded (first time)
	}

	// Row exists but version didn't match — optimistic lock failure.
	return fmt.Errorf("optimistic lock failure for user %s: expected version %d",
		snapshot.UserID, snapshot.Version)
}

// FindByUserID retrieves the balance snapshot for a user.
func (s *PostgresSnapshotStore) FindByUserID(ctx context.Context, userID uuid.UUID) (types.BalanceSnapshot, error) {
	const query = `
		SELECT user_id, balance_sen, version, updated_at
		FROM balance_snapshot
		WHERE user_id = $1
	`
	var snap types.BalanceSnapshot
	err := s.pool.QueryRow(ctx, query, userID).Scan(
		&snap.UserID, &snap.BalanceSen, &snap.Version, &snap.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.BalanceSnapshot{}, nil // not found, return zero value
		}
		return types.BalanceSnapshot{}, fmt.Errorf("find snapshot: %w", err)
	}
	return snap, nil
}
