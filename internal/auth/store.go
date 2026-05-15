package auth

import (
	"context"

	"senpay/internal/types"

	"github.com/google/uuid"
)

// UserRepository defines the interface for persisting user data.
// Implementations: PostgresUserStore (postgres.go).
type UserRepository interface {
	// Insert creates a new user record.
	Insert(ctx context.Context, user types.User) error

	// FindByPhone retrieves a user by their phone number.
	// Returns ErrUserNotFound if no user exists with the given phone.
	FindByPhone(ctx context.Context, phone string) (types.User, error)

	// FindByID retrieves a user by their UUID.
	// Returns ErrUserNotFound if no user exists with the given ID.
	FindByID(ctx context.Context, id uuid.UUID) (types.User, error)

	// UpdateKYCLevel updates the KYC level for a user.
	// Returns ErrUserNotFound if no user exists with the given ID.
	UpdateKYCLevel(ctx context.Context, id uuid.UUID, level string) error
}
