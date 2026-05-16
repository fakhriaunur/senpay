package transfer

import (
	"context"
	"sync"
	"testing"

	"senpay/internal/saga"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// serializationFailSaga simulates the saga coordinator exhausting retries on
// serialization failure (SQLSTATE 40001). It does NOT call the operation
// function (which requires real DB), but instead simulates the saga's behavior:
//
//   - Calls the compensation callback (as the real saga does after 3 failed retries)
//   - Returns ErrSerializationConflict (as the real saga does after exhaustion)
//
// This allows testing the Transfer method's error handling and idempotency
// cleanup without a real database connection.
type serializationFailSaga struct {
	mu               sync.Mutex
	compensateCalled bool
}

func (s *serializationFailSaga) Execute(_ context.Context, _ saga.Operation, comp saga.Compensation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if comp != nil {
		comp(context.Background(), &pgconn.PgError{Code: types.SQLSerializationError})
		s.compensateCalled = true
	}
	return &types.ErrSerializationConflict
}

// TestSerializationConflict verifies VAL-TRANSFER-015:
// When saga retries are exhausted on serialization failure, the Transfer method:
//   - Returns 409 SERIALIZATION_CONFLICT error with Indonesian message
//   - Cleans up the in-flight idempotency marker from Redis
//   - Cleans up any cached result data
//   - Returns nil TransferResult (no transfer committed)
//
// This is a unit test using mock dependencies — the saga coordinator is mocked
// to simulate exhaustion on serialization failure.
func TestSerializationConflict(t *testing.T) {
	cache := newMockRedisCache()
	users := newMockUserStore()
	sender := users.addUser("081111111111")
	_ = users.addUser("082222222222") // receiver

	mockSaga := &serializationFailSaga{}

	svc := &Service{
		pool:            nil, // not used — mock saga skips DB operations
		redisCache:      cache,
		natsClient:      nil,
		userStore:       users,
		sagaCoordinator: mockSaga,
	}

	req := TransferRequest{
		IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
		ToPhone:        "082222222222",
		AmountSen:      50000,
	}
	ctx := context.Background()

	// Do NOT pre-set the in-flight marker — let the Transfer flow acquire it
	// naturally via SetIfNotExist, then the mock saga returns serialization conflict.

	// Execute transfer — mock saga will fail with serialization conflict.
	result, domainErr := svc.Transfer(ctx, sender.ID, req)

	// Verify error: must be SERIALIZATION_CONFLICT.
	if domainErr == nil {
		t.Fatal("expected DomainError SERIALIZATION_CONFLICT, got nil")
	}
	if domainErr.Code != types.ErrCodeSerializationConflict {
		t.Errorf("expected code %q, got %q", types.ErrCodeSerializationConflict, domainErr.Code)
	}
	if domainErr.HTTPStatus != 409 {
		t.Errorf("expected HTTP 409, got %d", domainErr.HTTPStatus)
	}
	if domainErr.Message != "Silakan coba lagi" {
		t.Errorf("expected message 'Silakan coba lagi', got %q", domainErr.Message)
	}

	// Verify nil result — no transfer should have been committed.
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}

	// Verify compensation was called by the saga mock.
	if !mockSaga.compensateCalled {
		t.Error("expected saga compensation to be called after retry exhaustion")
	}

	// Verify idempotency key was cleaned up after saga exhaustion.
	val, err := cache.Get(ctx, req.IdempotencyKey)
	if err != nil {
		t.Errorf("cache.Get error: %v", err)
	}
	if val != "" {
		t.Errorf("expected idempotency key to be cleared after saga exhaustion, got %q", val)
	}

	// Verify cached result suffix is also cleaned up.
	val, err = cache.Get(ctx, req.IdempotencyKey+cachedResultSuffix)
	if err != nil {
		t.Errorf("cache.Get error: %v", err)
	}
	if val != "" {
		t.Errorf("expected cached result to be cleared after saga exhaustion, got %q", val)
	}
}
