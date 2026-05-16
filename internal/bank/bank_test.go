//go:build integration

package bank

import (
	"context"
	"testing"

	"senpay/internal/auth"
	"senpay/internal/idempotency"
	"senpay/internal/store/storetest"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestWithdrawFlow_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup pool: %v", err)
	}
	defer cleanup()

	// Setup Redis.
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	defer redisClient.Close()

	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)

	// Setup stores.
	vaStore := NewPostgresVAStore(pool)
	withdrawStore := NewPostgresWithdrawStore(pool)

	// Setup stub adapter with success behavior.
	stub := NewStubAdapter()
	stub.SetBehavior(StubBehaviorSuccess)

	// Create a test user with balance.
	userID := setupTestUserWithBalance(ctx, t, pool, 10_000_000)

	// Create service.
	userStore := auth.NewPostgresUserStore(pool)
	svc := &Service{
		pool:          pool,
		vaStore:       vaStore,
		withdrawStore: withdrawStore,
		redisCache:    redisCache,
		paymentRail:   stub,
		userStore:     userStore,
	}

	// Execute withdraw.
	req := WithdrawHTTPRequest{
		IdempotencyKey: uuid.New().String(),
		AmountSen:      5_000_000,
		BankAccount:    "1234567890",
	}

	result, domainErr := svc.Withdraw(ctx, userID, req)
	if domainErr != nil {
		t.Fatalf("unexpected error: %v", domainErr)
	}

	if result == nil {
		t.Fatal("result must not be nil")
	}
	if result.Status != types.TxStatusCommitted.String() {
		t.Errorf("status: got %q, want %q", result.Status, types.TxStatusCommitted.String())
	}
	if result.AmountSen != 5_000_000 {
		t.Errorf("amount: got %d, want %d", result.AmountSen, 5_000_000)
	}
	if result.TxID == uuid.Nil {
		t.Error("tx_id must not be nil")
	}
	if result.BankAccount != "1234567890" {
		t.Errorf("bank_account: got %q, want %q", result.BankAccount, "1234567890")
	}

	// Verify balance was deducted.
	balance, version, err := getBalance(ctx, pool, userID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if balance != 5_000_000 {
		t.Errorf("balance: got %d, want %d", balance, 5_000_000)
	}
	if version != 2 {
		t.Errorf("version: got %d, want %d", version, 2)
	}

	// Verify withdraw record exists.
	wdRecord, err := withdrawStore.FindByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil {
		t.Fatalf("find withdraw record: %v", err)
	}
	if wdRecord == nil {
		t.Fatal("withdraw record must exist")
	}
	if wdRecord.Status != types.TxStatusCommitted {
		t.Errorf("wd status: got %q, want %q", wdRecord.Status, types.TxStatusCommitted)
	}
}

func TestWithdrawFlow_InsufficientBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup pool: %v", err)
	}
	defer cleanup()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	defer redisClient.Close()

	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)
	vaStore := NewPostgresVAStore(pool)
	withdrawStore := NewPostgresWithdrawStore(pool)
	stub := NewStubAdapter()

	userID := setupTestUserWithBalance(ctx, t, pool, 1_000)

	userStore := auth.NewPostgresUserStore(pool)
	svc := &Service{
		pool:          pool,
		vaStore:       vaStore,
		withdrawStore: withdrawStore,
		redisCache:    redisCache,
		paymentRail:   stub,
		userStore:     userStore,
	}

	req := WithdrawHTTPRequest{
		IdempotencyKey: uuid.New().String(),
		AmountSen:      5_000_000,
		BankAccount:    "1234567890",
	}

	_, domainErr := svc.Withdraw(ctx, userID, req)
	if domainErr == nil {
		t.Fatal("expected error for insufficient balance")
	}
	if domainErr.Code != types.ErrCodeInsufficientBalance {
		t.Errorf("code: got %q, want %q", domainErr.Code, types.ErrCodeInsufficientBalance)
	}
}

func TestWithdrawFlow_Idempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup pool: %v", err)
	}
	defer cleanup()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	defer redisClient.Close()

	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)
	vaStore := NewPostgresVAStore(pool)
	withdrawStore := NewPostgresWithdrawStore(pool)
	stub := NewStubAdapter()

	userID := setupTestUserWithBalance(ctx, t, pool, 10_000_000)

	userStore := auth.NewPostgresUserStore(pool)
	svc := &Service{
		pool:          pool,
		vaStore:       vaStore,
		withdrawStore: withdrawStore,
		redisCache:    redisCache,
		paymentRail:   stub,
		userStore:     userStore,
	}

	key := uuid.New().String()
	req := WithdrawHTTPRequest{
		IdempotencyKey: key,
		AmountSen:      5_000_000,
		BankAccount:    "1234567890",
	}

	// First request should succeed.
	result1, err1 := svc.Withdraw(ctx, userID, req)
	if err1 != nil {
		t.Fatalf("first request failed: %v", err1)
	}

	// Second request with same key should return cached result.
	result2, err2 := svc.Withdraw(ctx, userID, req)
	if err2 != nil {
		t.Fatalf("second request failed: %v", err2)
	}

	if result1.TxID != result2.TxID {
		t.Error("idempotent requests must return same tx_id")
	}

	// Balance should only be deducted once.
	balance, _, err := getBalance(ctx, pool, userID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if balance != 5_000_000 {
		t.Errorf("balance: got %d, want %d (should be deducted once)", balance, 5_000_000)
	}
}

// TestWithdrawFlow_Reversal tests that a bank rejection triggers a reversal.
func TestWithdrawFlow_Reversal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup pool: %v", err)
	}
	defer cleanup()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	defer redisClient.Close()

	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)
	vaStore := NewPostgresVAStore(pool)
	withdrawStore := NewPostgresWithdrawStore(pool)
	stub := NewStubAdapter()
	stub.SetBehavior(StubBehaviorRejection)

	userID := setupTestUserWithBalance(ctx, t, pool, 10_000_000)

	userStore := auth.NewPostgresUserStore(pool)
	svc := &Service{
		pool:          pool,
		vaStore:       vaStore,
		withdrawStore: withdrawStore,
		redisCache:    redisCache,
		paymentRail:   stub,
		userStore:     userStore,
	}

	req := WithdrawHTTPRequest{
		IdempotencyKey: uuid.New().String(),
		AmountSen:      5_000_000,
		BankAccount:    "1234567890",
	}

	_, domainErr := svc.Withdraw(ctx, userID, req)
	if domainErr == nil {
		t.Fatal("expected error for bank rejection")
	}
	if domainErr.Code != ErrBankRejection.Code {
		t.Errorf("code: got %q, want %q", domainErr.Code, ErrBankRejection.Code)
	}

	// Balance should be restored (reversed).
	balance, _, err := getBalance(ctx, pool, userID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if balance != 10_000_000 {
		t.Errorf("balance after reversal: got %d, want %d", balance, 10_000_000)
	}
}

// ─── Helpers ──────────────────────────────────────────────────

func setupTestUserWithBalance(ctx context.Context, t *testing.T, pool *pgxpool.Pool, initialBalance int64) uuid.UUID {
	t.Helper()

	userID := uuid.Must(uuid.NewV7())

	// Insert user.
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, phone, pin_hash, kyc_level, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, userID, "test-"+uuid.New().String()+"@test.com", "hash", types.KYCLevelVerified)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	// Insert initial balance.
	if initialBalance > 0 {
		_, err = pool.Exec(ctx, `
			INSERT INTO balance_snapshot (user_id, balance_sen, version, updated_at)
			VALUES ($1, $2, 1, NOW())
		`, userID, initialBalance)
		if err != nil {
			t.Fatalf("insert balance: %v", err)
		}
	}

	return userID
}

func getBalance(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) (int64, int, error) {
	var balance int64
	var version int
	err := pool.QueryRow(ctx, `
		SELECT balance_sen, version FROM balance_snapshot WHERE user_id = $1
	`, userID).Scan(&balance, &version)
	if err != nil {
		return 0, 0, err
	}
	return balance, version, nil
}
