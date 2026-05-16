package bank

import (
	"context"
	"errors"
	"fmt"
	"time"

	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ────────────────────────────────────────────────────────────────
// PostgreSQL VA Store
// ────────────────────────────────────────────────────────────────

// PostgresVAStore implements VAStore backed by PostgreSQL.
type PostgresVAStore struct {
	pool *pgxpool.Pool
}

// NewPostgresVAStore creates a new PostgresVAStore.
func NewPostgresVAStore(pool *pgxpool.Pool) *PostgresVAStore {
	return &PostgresVAStore{pool: pool}
}

// Insert creates a new VA top-up record.
func (s *PostgresVAStore) Insert(ctx context.Context, record VATopupRecord) error {
	const query = `
		INSERT INTO va_topup (
			id, idempotency_key, user_id, va_number, amount_sen,
			status, created_at, expires_at, paid_at, tx_log_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := s.pool.Exec(ctx, query,
		record.ID, record.IdempotencyKey, record.UserID, record.VANumber,
		record.AmountSen, record.Status, record.CreatedAt, record.ExpiresAt,
		record.PaidAt, record.TxLogID,
	)
	if err != nil {
		return fmt.Errorf("insert va_topup: %w", err)
	}
	return nil
}

// FindByID retrieves a VA top-up record by its ID.
func (s *PostgresVAStore) FindByID(ctx context.Context, id uuid.UUID) (*VATopupRecord, error) {
	const query = `
		SELECT id, idempotency_key, user_id, va_number, amount_sen,
		       status, created_at, expires_at, paid_at, tx_log_id
		FROM va_topup
		WHERE id = $1
	`
	record, err := s.scanRow(ctx, query, id)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// FindByVANumber retrieves a VA top-up record by its VA number.
func (s *PostgresVAStore) FindByVANumber(ctx context.Context, vaNumber string) (*VATopupRecord, error) {
	const query = `
		SELECT id, idempotency_key, user_id, va_number, amount_sen,
		       status, created_at, expires_at, paid_at, tx_log_id
		FROM va_topup
		WHERE va_number = $1
	`
	record, err := s.scanRow(ctx, query, vaNumber)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// MarkAsPaid updates a VA record status to "paid".
func (s *PostgresVAStore) MarkAsPaid(ctx context.Context, vaNumber string, txLogID uuid.UUID) error {
	const query = `
		UPDATE va_topup
		SET status = 'paid', paid_at = NOW(), tx_log_id = $2
		WHERE va_number = $1 AND status = 'active'
	`
	result, err := s.pool.Exec(ctx, query, vaNumber, txLogID)
	if err != nil {
		return fmt.Errorf("mark va as paid: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("va_topup not found or already paid: %s", vaNumber)
	}
	return nil
}

// FindByIdempotencyKey retrieves a VA top-up record by idempotency key.
func (s *PostgresVAStore) FindByIdempotencyKey(ctx context.Context, key string) (*VATopupRecord, error) {
	const query = `
		SELECT id, idempotency_key, user_id, va_number, amount_sen,
		       status, created_at, expires_at, paid_at, tx_log_id
		FROM va_topup
		WHERE idempotency_key = $1
	`
	record, err := s.scanRow(ctx, query, key)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// scanRow scans a single row from the query into a VATopupRecord.
func (s *PostgresVAStore) scanRow(ctx context.Context, query string, args ...interface{}) (*VATopupRecord, error) {
	var record VATopupRecord
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&record.ID, &record.IdempotencyKey, &record.UserID, &record.VANumber,
		&record.AmountSen, &record.Status, &record.CreatedAt, &record.ExpiresAt,
		&record.PaidAt, &record.TxLogID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // not found
		}
		return nil, fmt.Errorf("query va_topup: %w", err)
	}
	return &record, nil
}

// ────────────────────────────────────────────────────────────────
// PostgreSQL Withdraw Store
// ────────────────────────────────────────────────────────────────

// PostgresWithdrawStore implements WithdrawStore backed by PostgreSQL.
type PostgresWithdrawStore struct {
	pool *pgxpool.Pool
}

// NewPostgresWithdrawStore creates a new PostgresWithdrawStore.
func NewPostgresWithdrawStore(pool *pgxpool.Pool) *PostgresWithdrawStore {
	return &PostgresWithdrawStore{pool: pool}
}

// Insert creates a new withdraw record.
func (s *PostgresWithdrawStore) Insert(ctx context.Context, record WithdrawRecord) error {
	const query = `
		INSERT INTO withdraw_records (
			id, idempotency_key, user_id, bank_account, amount_sen,
			status, failure_reason, created_at, committed_at, reversed_at, tx_log_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := s.pool.Exec(ctx, query,
		record.ID, record.IdempotencyKey, record.UserID, record.BankAccount,
		record.AmountSen, record.Status, record.FailureReason, record.CreatedAt,
		record.CommittedAt, record.ReversedAt, record.TxLogID,
	)
	if err != nil {
		return fmt.Errorf("insert withdraw_record: %w", err)
	}
	return nil
}

// FindByID retrieves a withdraw record by its ID.
func (s *PostgresWithdrawStore) FindByID(ctx context.Context, id uuid.UUID) (*WithdrawRecord, error) {
	const query = `
		SELECT id, idempotency_key, user_id, bank_account, amount_sen,
		       status, failure_reason, created_at, committed_at, reversed_at, tx_log_id
		FROM withdraw_records
		WHERE id = $1
	`
	return s.scanWithdrawRow(ctx, query, id)
}

// FindByIdempotencyKey retrieves a withdraw record by idempotency key.
func (s *PostgresWithdrawStore) FindByIdempotencyKey(ctx context.Context, key string) (*WithdrawRecord, error) {
	const query = `
		SELECT id, idempotency_key, user_id, bank_account, amount_sen,
		       status, failure_reason, created_at, committed_at, reversed_at, tx_log_id
		FROM withdraw_records
		WHERE idempotency_key = $1
	`
	return s.scanWithdrawRow(ctx, query, key)
}

// UpdateStatus updates the status and related timestamps of a withdraw record.
func (s *PostgresWithdrawStore) UpdateStatus(ctx context.Context, id uuid.UUID, status types.TxStatus, failureReason *string, committedAt, reversedAt *time.Time) error {
	const query = `
		UPDATE withdraw_records
		SET status = $2, failure_reason = $3, committed_at = $4, reversed_at = $5
		WHERE id = $1
	`
	_, err := s.pool.Exec(ctx, query, id, status, failureReason, committedAt, reversedAt)
	if err != nil {
		return fmt.Errorf("update withdraw_record: %w", err)
	}
	return nil
}

// scanWithdrawRow scans a single row from the query into a WithdrawRecord.
func (s *PostgresWithdrawStore) scanWithdrawRow(ctx context.Context, query string, args ...interface{}) (*WithdrawRecord, error) {
	var record WithdrawRecord
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&record.ID, &record.IdempotencyKey, &record.UserID, &record.BankAccount,
		&record.AmountSen, &record.Status, &record.FailureReason, &record.CreatedAt,
		&record.CommittedAt, &record.ReversedAt, &record.TxLogID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // not found
		}
		return nil, fmt.Errorf("query withdraw_record: %w", err)
	}
	return &record, nil
}

// ensure interfaces are satisfied
var _ VAStore = (*PostgresVAStore)(nil)
var _ WithdrawStore = (*PostgresWithdrawStore)(nil)
