package saga

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"senpay/internal/types"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSagaCoordinator_Execute_Success(t *testing.T) {
	t.Parallel()

	coordinator := NewSagaCoordinator()
	var called int

	err := coordinator.Execute(context.Background(),
		func(ctx context.Context) error {
			called++
			return nil
		},
		nil,
	)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if called != 1 {
		t.Fatalf("expected 1 call, got %d", called)
	}
}

func TestSagaCoordinator_Execute_PermanentDomainError(t *testing.T) {
	t.Parallel()

	coordinator := NewSagaCoordinator()
	var called int

	err := coordinator.Execute(context.Background(),
		func(ctx context.Context) error {
			called++
			return &types.ErrInsufficientBalance
		},
		nil,
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var domainErr *types.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if domainErr.Code != types.ErrCodeInsufficientBalance {
		t.Fatalf("expected code %q, got %q", types.ErrCodeInsufficientBalance, domainErr.Code)
	}
	if called != 1 {
		t.Fatalf("expected exactly 1 call (no retry for permanent error), got %d", called)
	}
}

func TestSagaCoordinator_Execute_TransientThenSuccess(t *testing.T) {
	t.Parallel()

	coordinator := &SagaCoordinator{
		maxRetries: 3,
		backoff:    1 * time.Millisecond, // fast for tests
	}

	var callCount int

	err := coordinator.Execute(context.Background(),
		func(ctx context.Context) error {
			callCount++
			if callCount < 3 {
				// Return a transient serialization error
				return &pgconn.PgError{Code: "40001"}
			}
			return nil
		},
		nil,
	)

	if err != nil {
		t.Fatalf("expected nil after retry success, got %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls (2 retries + 1 success), got %d", callCount)
	}
}

func TestSagaCoordinator_Execute_TransientExhaustion(t *testing.T) {
	t.Parallel()

	coordinator := &SagaCoordinator{
		maxRetries: 3,
		backoff:    1 * time.Millisecond,
	}

	var compensated bool
	var callCount int

	err := coordinator.Execute(context.Background(),
		func(ctx context.Context) error {
			callCount++
			return &pgconn.PgError{Code: "40001"}
		},
		func(ctx context.Context, originalErr error) {
			compensated = true
		},
	)

	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
	var domainErr *types.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if domainErr.Code != types.ErrCodeSerializationConflict {
		t.Fatalf("expected code %q, got %q", types.ErrCodeSerializationConflict, domainErr.Code)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got %d", callCount)
	}
	if !compensated {
		t.Fatal("expected compensation to be called")
	}
}

func TestSagaCoordinator_Execute_DeadlockTransient(t *testing.T) {
	t.Parallel()

	coordinator := &SagaCoordinator{
		maxRetries: 3,
		backoff:    1 * time.Millisecond,
	}

	var callCount int

	err := coordinator.Execute(context.Background(),
		func(ctx context.Context) error {
			callCount++
			if callCount == 1 {
				return &pgconn.PgError{Code: "40P01"} // deadlock
			}
			return nil
		},
		nil,
	)

	if err != nil {
		t.Fatalf("expected nil after deadlock retry, got %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls (1 deadlock + 1 success), got %d", callCount)
	}
}

func TestSagaCoordinator_Execute_NonTransientError(t *testing.T) {
	t.Parallel()

	coordinator := &SagaCoordinator{
		maxRetries: 3,
		backoff:    1 * time.Millisecond,
	}

	var callCount int
	nonTransientErr := fmt.Errorf("some permanent system error")

	err := coordinator.Execute(context.Background(),
		func(ctx context.Context) error {
			callCount++
			return nonTransientErr
		},
		nil,
	)

	if !errors.Is(err, nonTransientErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected exactly 1 call (no retry for non-transient), got %d", callCount)
	}
}

func TestSagaCoordinator_Execute_ConnectionTransient(t *testing.T) {
	t.Parallel()

	coordinator := &SagaCoordinator{
		maxRetries: 3,
		backoff:    1 * time.Millisecond,
	}

	var callCount int

	err := coordinator.Execute(context.Background(),
		func(ctx context.Context) error {
			callCount++
			if callCount == 1 {
				return fmt.Errorf("connection refused")
			}
			return nil
		},
		nil,
	)

	if err != nil {
		t.Fatalf("expected nil after connection retry, got %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls, got %d", callCount)
	}
}

func TestSagaCoordinator_Execute_ContextCancelled(t *testing.T) {
	t.Parallel()

	coordinator := &SagaCoordinator{
		maxRetries: 3,
		backoff:    50 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancelled

	err := coordinator.Execute(ctx,
		func(ctx context.Context) error {
			// Should not be called because context is already cancelled
			return &pgconn.PgError{Code: "40001"}
		},
		nil,
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestIsTransient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "serialization_40001", err: &pgconn.PgError{Code: "40001"}, want: true},
		{name: "deadlock_40P01", err: &pgconn.PgError{Code: "40P01"}, want: true},
		{name: "other_pg_error", err: &pgconn.PgError{Code: "23505"}, want: false},
		{name: "connection_refused", err: fmt.Errorf("connection refused"), want: true},
		{name: "connection_timeout", err: fmt.Errorf("connect timeout"), want: true},
		{name: "random_error", err: fmt.Errorf("something else"), want: false},
		{name: "domain_error", err: &types.ErrInvalidAmount, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransient(tt.err)
			if got != tt.want {
				t.Errorf("isTransient(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
