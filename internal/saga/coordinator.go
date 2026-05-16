// Package saga provides a saga coordinator with retry and compensation.
//
// Patterns:
//   - Retry up to 3 times on transient failures (serialization, deadlock, connection)
//   - Exponential backoff between retries
//   - Compensation callback when all retries exhausted
//   - Permanent errors (DomainError) returned immediately without retry
package saga

import (
	"context"
	"errors"
	"strings"
	"time"

	"senpay/internal/types"

	"github.com/jackc/pgx/v5/pgconn"
)

// SagaCoordinator handles retry and compensation for operations
// that may fail transiently (e.g., serialization conflicts).
type SagaCoordinator struct {
	maxRetries int
	backoff    time.Duration
}

// NewSagaCoordinator creates a new SagaCoordinator with default settings:
// 3 retries with 100ms initial backoff, doubling each retry.
func NewSagaCoordinator() *SagaCoordinator {
	return &SagaCoordinator{
		maxRetries: 3,
		backoff:    100 * time.Millisecond,
	}
}

// Operation is a function that performs a business operation.
// It should return nil on success, a *types.DomainError for permanent errors,
// or a transient error (serialization, connection) for retryable failures.
type Operation func(ctx context.Context) error

// Compensation is a function that reverts the effects of a failed operation
// after all retries are exhausted.
type Compensation func(ctx context.Context, originalErr error)

// DefaultMaxRetries is the default number of retry attempts.
const DefaultMaxRetries = 3

// DefaultBackoff is the initial backoff duration between retries.
const DefaultBackoff = 100 * time.Millisecond

// Execute runs the operation with retry and compensation.
// Returns nil on success.
// Returns *types.DomainError for permanent errors (application errors).
// Returns ErrSerializationConflict if all retries are exhausted on transient errors.
func (s *SagaCoordinator) Execute(ctx context.Context, op Operation, compensate Compensation) error {
	var lastErr error

	for i := 0; i < s.maxRetries; i++ {
		err := op(ctx)
		if err == nil {
			return nil
		}

		// Check if this is a permanent domain error — don't retry.
		if domainErr, ok := AsDomainError(err); ok {
			return domainErr
		}

		// Check if transient — only transient errors are retried.
		if !isTransient(err) {
			return err
		}

		lastErr = err

		// Exponential backoff before retry (100ms, 200ms, skip last retry).
		if i < s.maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.backoff * (1 << i)):
			}
		}
	}

	// All retries exhausted — compensate.
	if compensate != nil {
		compensate(ctx, lastErr)
	}

	return &types.ErrSerializationConflict
}

// AsDomainError extracts a *types.DomainError from an error chain.
// Handles both value and pointer DomainError types.
func AsDomainError(err error) (*types.DomainError, bool) {
	if err == nil {
		return nil, false
	}
	// Check for *DomainError (pointer).
	var dePtr *types.DomainError
	if errors.As(err, &dePtr) {
		return dePtr, true
	}
	// Check for DomainError (value).
	var de types.DomainError
	if errors.As(err, &de) {
		return &de, true
	}
	return nil, false
}

// isTransient checks if an error is transient and should be retried.
// Transient errors include:
//   - PostgreSQL serialization errors (SQLSTATE 40001)
//   - PostgreSQL deadlock errors (SQLSTATE 40P01)
//   - Connection errors
func isTransient(err error) bool {
	if err == nil {
		return false
	}

	// PostgreSQL serialization/deadlock errors.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == types.SQLSerializationError || pgErr.Code == types.SQLDeadlockError {
			return true
		}
	}

	// Connection errors.
	errStr := err.Error()
	if strings.Contains(errStr, "connection") || strings.Contains(errStr, "connect") {
		return true
	}

	return false
}
