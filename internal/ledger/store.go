package ledger

import (
	"context"

	"senpay/internal/types"

	"github.com/google/uuid"
)

// LedgerStore defines the interface for persisting transaction log entries.
// Implementations: PostgresTxLogStore (postgres.go).
type LedgerStore interface {
	// Append inserts a new transaction log entry within a SERIALIZABLE isolation transaction.
	// Returns a DomainError if the idempotency_key already exists (duplicate).
	Append(ctx context.Context, tx types.Transaction) error

	// QueryByUserID retrieves transaction log entries for a user with cursor-based pagination.
	// Results are ordered by created_at DESC (newest first).
	//
	// Parameters:
	//   - userID: the user's UUID
	//   - cursor: opaque cursor string from previous response (empty string for first page)
	//   - limit: maximum number of items to return (default 20)
	//
	// Returns:
	//   - transactions: slice of Transaction entries
	//   - next_cursor: opaque string for the next page (empty string if no more results)
	//   - error: any error encountered
	QueryByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]types.Transaction, string, error)

	// FindByID retrieves a single transaction log entry by its ID.
	// Returns a DomainError with USER_NOT_FOUND if the transaction does not exist.
	FindByID(ctx context.Context, id uuid.UUID) (types.Transaction, error)
}
