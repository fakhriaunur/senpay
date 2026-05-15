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

// ────────────────────────────────────────────────────────────────
// Withdraw Records
// ────────────────────────────────────────────────────────────────

// WithdrawRecord represents a withdraw request tracked in PostgreSQL.
type WithdrawRecord struct {
	ID             uuid.UUID  `json:"id"`
	IdempotencyKey string     `json:"idempotency_key"`
	UserID         uuid.UUID  `json:"user_id"`
	BankAccount    string     `json:"bank_account"`
	AmountSen      int64      `json:"amount_sen"`
	Status         string     `json:"status"` // pending, committed, failed, timeout
	FailureReason  *string    `json:"failure_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CommittedAt    *time.Time `json:"committed_at,omitempty"`
	ReversedAt     *time.Time `json:"reversed_at,omitempty"`
	TxLogID        *uuid.UUID `json:"tx_log_id,omitempty"`
}

// WithdrawStore defines the interface for persisting withdraw records.
type WithdrawStore interface {
	// Insert creates a new withdraw record.
	Insert(ctx context.Context, record WithdrawRecord) error

	// FindByID retrieves a withdraw record by its ID.
	FindByID(ctx context.Context, id uuid.UUID) (*WithdrawRecord, error)

	// FindByIdempotencyKey retrieves a withdraw record by idempotency key.
	FindByIdempotencyKey(ctx context.Context, key string) (*WithdrawRecord, error)

	// UpdateStatus updates the status and related timestamps of a withdraw record.
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, failureReason *string, committedAt, reversedAt *time.Time) error
}
