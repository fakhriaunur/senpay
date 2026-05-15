package bank

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ────────────────────────────────────────────────────────────────
// VA Store Interface
// ────────────────────────────────────────────────────────────────

// VATopupRecord represents a VA top-up record stored in PostgreSQL.
type VATopupRecord struct {
	ID             uuid.UUID  `json:"id"`
	IdempotencyKey string     `json:"idempotency_key"`
	UserID         uuid.UUID  `json:"user_id"`
	VANumber       string     `json:"va_number"`
	AmountSen      int64      `json:"amount_sen"`
	Status         string     `json:"status"` // active, paid, expired
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	TxLogID        *uuid.UUID `json:"tx_log_id,omitempty"`
}

// VAStore defines the interface for persisting VA top-up records.
type VAStore interface {
	// Insert creates a new VA top-up record.
	Insert(ctx context.Context, record VATopupRecord) error

	// FindByID retrieves a VA top-up record by its ID.
	FindByID(ctx context.Context, id uuid.UUID) (*VATopupRecord, error)

	// FindByVANumber retrieves a VA top-up record by its VA number.
	FindByVANumber(ctx context.Context, vaNumber string) (*VATopupRecord, error)

	// MarkAsPaid updates a VA record status to "paid" and sets the paid_at and tx_log_id.
	MarkAsPaid(ctx context.Context, vaNumber string, txLogID uuid.UUID) error

	// FindByIdempotencyKey retrieves a VA top-up record by idempotency key.
	FindByIdempotencyKey(ctx context.Context, key string) (*VATopupRecord, error)
}
