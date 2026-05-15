package auth

import (
	"context"
	"errors"
	"fmt"

	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresUserStore implements UserRepository backed by PostgreSQL.
type PostgresUserStore struct {
	pool *pgxpool.Pool
}

// NewPostgresUserStore creates a new PostgresUserStore.
func NewPostgresUserStore(pool *pgxpool.Pool) *PostgresUserStore {
	return &PostgresUserStore{pool: pool}
}

// Insert creates a new user record.
func (s *PostgresUserStore) Insert(ctx context.Context, user types.User) error {
	const query = `
		INSERT INTO users (id, phone, pin_hash, kyc_level, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := s.pool.Exec(ctx, query,
		user.ID, user.Phone, user.PINHash, user.KYCLevel, user.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// FindByPhone retrieves a user by their phone number.
func (s *PostgresUserStore) FindByPhone(ctx context.Context, phone string) (types.User, error) {
	const query = `
		SELECT id, phone, pin_hash, kyc_level, created_at
		FROM users
		WHERE phone = $1
	`
	return scanUser(s.pool.QueryRow(ctx, query, phone))
}

// FindByID retrieves a user by their UUID.
func (s *PostgresUserStore) FindByID(ctx context.Context, id uuid.UUID) (types.User, error) {
	const query = `
		SELECT id, phone, pin_hash, kyc_level, created_at
		FROM users
		WHERE id = $1
	`
	return scanUser(s.pool.QueryRow(ctx, query, id))
}

// UpdateKYCLevel updates the KYC level for a user.
func (s *PostgresUserStore) UpdateKYCLevel(ctx context.Context, id uuid.UUID, level types.KYCLevel) error {
	const query = `
		UPDATE users
		SET kyc_level = $1
		WHERE id = $2
	`
	result, err := s.pool.Exec(ctx, query, level, id)
	if err != nil {
		return fmt.Errorf("update kyc level: %w", err)
	}
	if result.RowsAffected() == 0 {
		return types.ErrUserNotFound
	}
	return nil
}

// scanUser scans a user row from the database.
func scanUser(row pgx.Row) (types.User, error) {
	var u types.User
	err := row.Scan(&u.ID, &u.Phone, &u.PINHash, &u.KYCLevel, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.User{}, types.ErrUserNotFound
		}
		return types.User{}, fmt.Errorf("scan user: %w", err)
	}
	return u, nil
}
