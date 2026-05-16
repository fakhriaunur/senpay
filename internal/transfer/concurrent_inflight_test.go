package transfer

import (
	"context"
	"sync"
	"testing"
	"time"

	"senpay/internal/saga"
	"senpay/internal/types"

	"github.com/google/uuid"
)

// blockingMockRedisCache provides synchronization primitives for testing
// concurrent in-flight idempotency behavior.
//
// Sequence:
//  1. First goroutine calls SetIfNotExist → stores "in_flight", closes acquired, blocks on proceed
//  2. Test waits on acquired, then second goroutine calls Get → sees "in_flight" → 202
//  3. Test closes proceed to release first goroutine
type blockingMockRedisCache struct {
	mu   sync.Mutex
	data map[string]string

	acquired chan struct{} // closed when first goroutine acquires the in-flight marker
	proceed  chan struct{} // closed when test signals first goroutine to continue
}

func newBlockingMockRedisCache() *blockingMockRedisCache {
	return &blockingMockRedisCache{
		data:     make(map[string]string),
		acquired: make(chan struct{}),
		proceed:  make(chan struct{}),
	}
}

func (c *blockingMockRedisCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data[key], nil
}

func (c *blockingMockRedisCache) SetIfNotExist(_ context.Context, key string, status string, _ time.Duration) (bool, error) {
	c.mu.Lock()
	if _, exists := c.data[key]; exists {
		c.mu.Unlock()
		return false, nil
	}
	c.data[key] = status
	c.mu.Unlock() // release before blocking to avoid deadlock

	close(c.acquired) // signal test that marker is set
	<-c.proceed       // block until test releases

	return true, nil
}

func (c *blockingMockRedisCache) Set(_ context.Context, key string, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
	return nil
}

func (c *blockingMockRedisCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

// mockSagaExecutor satisfies sagaExecutor by returning success immediately
// without executing the operation. Avoids real database dependency.
type mockSagaExecutor struct{}

func (m *mockSagaExecutor) Execute(_ context.Context, _ saga.Operation, _ saga.Compensation) error {
	return nil
}

// TestConcurrentInFlight verifies VAL-TRANSFER-007:
// In-flight idempotency marker returns HTTP 202 REQUEST_IN_FLIGHT for
// concurrent requests with the same idempotency key.
//
// Test harness:
//   - Goroutine A acquires idempotency key via SetIfNotExist, then blocks on barrier
//   - Goroutine B sends same idempotency key, receives 202 REQUEST_IN_FLIGHT
//   - Barrier released after B's response is verified
//   - Test is deterministic: exact ordering enforced by sync primitives
func TestConcurrentInFlight(t *testing.T) {
	cache := newBlockingMockRedisCache()
	users := newMockUserStore()
	sender := users.addUser("081111111111")
	_ = users.addUser("082222222222") // receiver

	svc := &Service{
		pool:            nil, // not used — mock saga skips DB operations
		redisCache:      cache,
		natsClient:      nil,
		userStore:       users,
		sagaCoordinator: &mockSagaExecutor{},
	}

	req := TransferRequest{
		IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
		ToPhone:        "082222222222",
		AmountSen:      50000,
	}
	ctx := context.Background()

	// --- Goroutine A: acquires in-flight marker, then blocks ---
	var (
		resultA *TransferResult
		errA    *types.DomainError
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		resultA, errA = svc.Transfer(ctx, sender.ID, req)
	}()

	// Wait for A to acquire the in-flight marker.
	<-cache.acquired

	// --- Goroutine B (main goroutine): same key → 202 ---
	resultB, errB := svc.Transfer(ctx, sender.ID, req)

	// Verify B receives REQUEST_IN_FLIGHT.
	if errB == nil {
		t.Fatal("expected DomainError REQUEST_IN_FLIGHT for goroutine B, got nil")
	}
	if errB.Code != types.ErrCodeRequestInFlight {
		t.Errorf("expected code %q, got %q", types.ErrCodeRequestInFlight, errB.Code)
	}
	if errB.HTTPStatus != 202 {
		t.Errorf("expected HTTP 202, got %d", errB.HTTPStatus)
	}
	if resultB != nil {
		t.Errorf("expected nil result for goroutine B, got %+v", resultB)
	}
	// Per VAL-TRANSFER-007, error message is in Indonesian.
	if errB.Message != "Permintaan sedang diproses" {
		t.Errorf("expected message %q, got %q", "Permintaan sedang diproses", errB.Message)
	}

	// --- Release barrier so goroutine A can complete ---
	close(cache.proceed)
	wg.Wait()

	// A should have completed without error.
	if errA != nil {
		t.Errorf("expected no error for goroutine A, got %v", errA)
	}
	_ = resultA // result may be nil with mock saga; behavior verified via B's response
}
