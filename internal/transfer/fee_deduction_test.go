//go:build integration

package transfer

import (
	"context"
	"os"
	"testing"

	"senpay/internal/auth"
	"senpay/internal/fee"
	"senpay/internal/idempotency"
	"senpay/internal/store/storetest"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// defaultTestFeeCfg matches fees.yaml default values.
var defaultTestFeeCfg = fee.FeeConfig{
	FlatFeeBasicSen: 2500,
	RateVerifiedPct: 0.7,
	MinFeeSen:       1000,
}

// TestFeeDeduction_UnverifiedSender verifies VAL-CROSS-033:
// Fee deduction invariant — sender balance decreases by amount + fee,
// receiver balance increases by amount only.
//
// Basic KYC sender: fee = flat_fee_basic_sen (2,500 sen).
func TestFeeDeduction_UnverifiedSender(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup pool: %v", err)
	}
	defer cleanup()

	redisClient := newTestRedis(ctx, t)
	defer redisClient.Close()

	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)
	userStore := auth.NewPostgresUserStore(pool)

	// Create sender (basic KYC) with sufficient balance.
	sender := setupUser(ctx, t, pool, "081111111111", types.KYCLevelBasic, 1_000_000_00) // Rp 1,000,000
	// Create receiver (any KYC) with zero balance.
	receiver := setupUser(ctx, t, pool, "082222222222", types.KYCLevelVerified, 0)

	svc := NewService(pool, redisCache, nil, userStore, defaultTestFeeCfg)

	amountSen := int64(500_000) // Rp 5,000
	req := TransferRequest{
		IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
		ToPhone:        "082222222222",
		AmountSen:      amountSen,
	}

	result, domainErr := svc.Transfer(ctx, sender.ID, req)
	if domainErr != nil {
		t.Fatalf("transfer failed: code=%s message=%s", domainErr.Code, domainErr.Message)
	}
	if result == nil {
		t.Fatal("transfer result must not be nil")
	}
	if result.Status != types.TxStatusCommitted.String() {
		t.Errorf("status: got %q, want %q", result.Status, types.TxStatusCommitted.String())
	}

	// Verify fee amount: basic KYC = flat 2,500 sen.
	expectedFee := int64(2500)
	if result.FeeSen != expectedFee {
		t.Errorf("fee_sen: got %d, want %d", result.FeeSen, expectedFee)
	}

	// Verify sender balance: initial - (amount + fee).
	expectedSenderBalance := int64(1_000_000_00) - (amountSen + expectedFee)
	if result.SenderBalanceSen != expectedSenderBalance {
		t.Errorf("sender_balance_sen: got %d, want %d (initial=%d - (amount=%d + fee=%d))",
			result.SenderBalanceSen, expectedSenderBalance, 1_000_000_00, amountSen, expectedFee)
	}

	// Verify receiver balance: initial + amount.
	expectedReceiverBalance := int64(0) + amountSen
	if result.ReceiverBalanceSen != expectedReceiverBalance {
		t.Errorf("receiver_balance_sen: got %d, want %d (initial=%d + amount=%d)",
			result.ReceiverBalanceSen, expectedReceiverBalance, 0, amountSen)
	}

	// Verify persisted balances via DB.
	senderBal, _, err := getTransferBalance(ctx, pool, sender.ID)
	if err != nil {
		t.Fatalf("get sender balance: %v", err)
	}
	if senderBal != expectedSenderBalance {
		t.Errorf("sender DB balance: got %d, want %d", senderBal, expectedSenderBalance)
	}

	receiverBal, _, err := getTransferBalance(ctx, pool, receiver.ID)
	if err != nil {
		t.Fatalf("get receiver balance: %v", err)
	}
	if receiverBal != expectedReceiverBalance {
		t.Errorf("receiver DB balance: got %d, want %d", receiverBal, expectedReceiverBalance)
	}
}

// TestFeeDeduction_VerifiedSender verifies VAL-CROSS-033 for verified KYC sender:
// Fee = max(amount * rate_verified_pct / 100, min_fee_sen).
func TestFeeDeduction_VerifiedSender(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup pool: %v", err)
	}
	defer cleanup()

	redisClient := newTestRedis(ctx, t)
	defer redisClient.Close()

	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)
	userStore := auth.NewPostgresUserStore(pool)

	// Create sender (verified KYC) with sufficient balance.
	sender := setupUser(ctx, t, pool, "083333333333", types.KYCLevelVerified, 10_000_000_00) // Rp 10,000,000
	_ = setupUser(ctx, t, pool, "084444444444", types.KYCLevelVerified, 0)

	svc := NewService(pool, redisCache, nil, userStore, defaultTestFeeCfg)

	// Transfer 1,000,000 sen (Rp 10,000).
	// Expected fee = max(1,000,000 * 0.7 / 100, 1000) = max(7000, 1000) = 7000 sen.
	amountSen := int64(1_000_000)
	req := TransferRequest{
		IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
		ToPhone:        "084444444444",
		AmountSen:      amountSen,
	}

	result, domainErr := svc.Transfer(ctx, sender.ID, req)
	if domainErr != nil {
		t.Fatalf("transfer failed: code=%s message=%s", domainErr.Code, domainErr.Message)
	}

	// Expected fee: 0.7% of 1,000,000 = 7,000 sen (above the 1,000 floor).
	expectedFee := int64(7000)
	if result.FeeSen != expectedFee {
		t.Errorf("fee_sen: got %d, want %d", result.FeeSen, expectedFee)
	}

	// Verify sender balance.
	expectedSenderBalance := int64(10_000_000_00) - (amountSen + expectedFee)
	if result.SenderBalanceSen != expectedSenderBalance {
		t.Errorf("sender_balance_sen: got %d, want %d", result.SenderBalanceSen, expectedSenderBalance)
	}

	// Verify receiver balance.
	expectedReceiverBalance := int64(0) + amountSen
	if result.ReceiverBalanceSen != expectedReceiverBalance {
		t.Errorf("receiver_balance_sen: got %d, want %d", result.ReceiverBalanceSen, expectedReceiverBalance)
	}
}

// TestFeeDeduction_VerifiedFloor verifies that verified KYC fee correctly
// applies the minimum fee floor for small transfers.
func TestFeeDeduction_VerifiedFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup pool: %v", err)
	}
	defer cleanup()

	redisClient := newTestRedis(ctx, t)
	defer redisClient.Close()

	redisCache := idempotency.NewRedisIdempotencyCache(redisClient)
	userStore := auth.NewPostgresUserStore(pool)

	// Create sender (verified KYC) with balance.
	sender := setupUser(ctx, t, pool, "085555555555", types.KYCLevelVerified, 1_000_000_00)
	_ = setupUser(ctx, t, pool, "086666666666", types.KYCLevelVerified, 0)

	svc := NewService(pool, redisCache, nil, userStore, defaultTestFeeCfg)

	// Transfer 100,000 sen (Rp 1,000).
	// 0.7% of 100,000 = 700 sen, but floor is 1,000 sen → fee = 1,000 sen.
	amountSen := int64(100_000)
	req := TransferRequest{
		IdempotencyKey: uuid.Must(uuid.NewV7()).String(),
		ToPhone:        "086666666666",
		AmountSen:      amountSen,
	}

	result, domainErr := svc.Transfer(ctx, sender.ID, req)
	if domainErr != nil {
		t.Fatalf("transfer failed: code=%s message=%s", domainErr.Code, domainErr.Message)
	}

	// Expected fee: floor = 1,000 sen (0.7% of 100k = 700, below min 1,000).
	expectedFee := int64(1000)
	if result.FeeSen != expectedFee {
		t.Errorf("fee_sen: got %d, want %d (floor)", result.FeeSen, expectedFee)
	}

	expectedSenderBalance := int64(1_000_000_00) - (amountSen + expectedFee)
	if result.SenderBalanceSen != expectedSenderBalance {
		t.Errorf("sender_balance_sen: got %d, want %d", result.SenderBalanceSen, expectedSenderBalance)
	}

	expectedReceiverBalance := int64(0) + amountSen
	if result.ReceiverBalanceSen != expectedReceiverBalance {
		t.Errorf("receiver_balance_sen: got %d, want %d", result.ReceiverBalanceSen, expectedReceiverBalance)
	}
}

// ─── Helpers ──────────────────────────────────────────────────

// setupUser creates a test user with given phone, KYC level, and initial balance.
func setupUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, phone string, kycLevel types.KYCLevel, initialBalance int64) types.User {
	t.Helper()

	userID := uuid.Must(uuid.NewV7())

	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, phone, pin_hash, kyc_level, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, userID, phone, "test-hash", kycLevel)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if initialBalance > 0 {
		_, err = pool.Exec(ctx, `
			INSERT INTO balance_snapshot (user_id, balance_sen, version, updated_at)
			VALUES ($1, $2, 1, NOW())
		`, userID, initialBalance)
		if err != nil {
			t.Fatalf("insert balance: %v", err)
		}
	}

	return types.User{
		ID:       userID,
		Phone:    phone,
		KYCLevel: kycLevel,
	}
}

// getTransferBalance retrieves a user's balance from balance_snapshot.
func getTransferBalance(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) (int64, int, error) {
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

// newTestRedis creates a Redis client for testing.
func newTestRedis(ctx context.Context, t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_URL")
	if addr == "" {
		addr = os.Getenv("REDIS_URL")
	}
	if addr == "" {
		addr = "redis://localhost:6379"
	}

	opts, err := redis.ParseURL(addr)
	if err != nil {
		t.Fatalf("parse redis URL: %v", err)
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}

	// Flush DB to ensure clean state.
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("redis flush: %v", err)
	}

	return client
}
