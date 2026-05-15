package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresIdempotencyStore implements IdempotencyStore backed by PostgreSQL.
type PostgresIdempotencyStore struct {
	pool *pgxpool.Pool
}

// NewPostgresIdempotencyStore creates a new PostgresIdempotencyStore.
func NewPostgresIdempotencyStore(pool *pgxpool.Pool) *PostgresIdempotencyStore {
	return &PostgresIdempotencyStore{pool: pool}
}

// Insert creates a new idempotency key record with the given status.
// Returns a unique constraint violation error if the key already exists.
func (s *PostgresIdempotencyStore) Insert(ctx context.Context, key string, status string) error {
	const query = `
		INSERT INTO idempotency_keys (key, status, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
	`
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, query, key, status, now)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("duplicate idempotency key: %s", key)
		}
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}

// FindByKey retrieves the status of an idempotency key.
// Returns empty string if the key is not found.
func (s *PostgresIdempotencyStore) FindByKey(ctx context.Context, key string) (string, error) {
	const query = `
		SELECT status FROM idempotency_keys WHERE key = $1
	`
	var status string
	err := s.pool.QueryRow(ctx, query, key).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("find idempotency key: %w", err)
	}
	return status, nil
}

// UpdateStatus updates the status of an existing idempotency key.
func (s *PostgresIdempotencyStore) UpdateStatus(ctx context.Context, key string, status string) error {
	const query = `
		UPDATE idempotency_keys
		SET status = $1, updated_at = $2
		WHERE key = $3
	`
	now := time.Now().UTC()
	result, err := s.pool.Exec(ctx, query, status, now, key)
	if err != nil {
		return fmt.Errorf("update idempotency key: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("idempotency key not found: %s", key)
	}
	return nil
}

// isUniqueViolation checks if a PostgreSQL error is a unique constraint violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
