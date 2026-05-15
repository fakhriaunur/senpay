//go:build integration

package projection

import (
	"context"
	"testing"
	"time"

	"senpay/internal/store/storetest"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgres_SnapshotStore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer cleanup()

	store := NewPostgresSnapshotStore(pool)

	// Create a test user.
	userID := createTestUser(ctx, t, pool)

	t.Run("Upsert_Insert_FindByUserID", func(t *testing.T) {
		snap := types.NewBalanceSnapshot(userID)
		err := store.Upsert(ctx, snap)
		if err != nil {
			t.Fatalf("Upsert (insert): %v", err)
		}

		got, err := store.FindByUserID(ctx, userID)
		if err != nil {
			t.Fatalf("FindByUserID: %v", err)
		}
		if got.BalanceSen != 0 {
			t.Errorf("BalanceSen: got %d, want 0", got.BalanceSen)
		}
		if got.Version != 1 {
			t.Errorf("Version: got %d, want 1", got.Version)
		}
	})

	t.Run("Upsert_Update_OptimisticLock", func(t *testing.T) {
		userID2 := createTestUser(ctx, t, pool)

		// Insert.
		snap := types.NewBalanceSnapshot(userID2)
		err := store.Upsert(ctx, snap)
		if err != nil {
			t.Fatalf("Upsert (insert): %v", err)
		}

		// Fetch to get current version.
		current, err := store.FindByUserID(ctx, userID2)
		if err != nil {
			t.Fatalf("FindByUserID: %v", err)
		}

		// Update with correct version.
		current.BalanceSen = 100000
		current.UpdatedAt = time.Now().UTC()
		err = store.Upsert(ctx, current)
		if err != nil {
			t.Fatalf("Upsert (update): %v", err)
		}

		// Verify update.
		updated, err := store.FindByUserID(ctx, userID2)
		if err != nil {
			t.Fatalf("FindByUserID: %v", err)
		}
		if updated.BalanceSen != 100000 {
			t.Errorf("BalanceSen: got %d, want 100000", updated.BalanceSen)
		}
		if updated.Version != 2 {
			t.Errorf("Version: got %d, want 2", updated.Version)
		}
	})

	t.Run("Upsert_OptimisticLockFailure", func(t *testing.T) {
		userID3 := createTestUser(ctx, t, pool)

		// Insert snapshot (version = 1).
		snap := types.NewBalanceSnapshot(userID3)
		err := store.Upsert(ctx, snap)
		if err != nil {
			t.Fatalf("Upsert (insert): %v", err)
		}

		// Fetch to get the current version (version = 1 after insert).
		latest, err := store.FindByUserID(ctx, userID3)
		if err != nil {
			t.Fatalf("FindByUserID: %v", err)
		}

		// First update succeeds, bumps version to 2.
		latest.BalanceSen = 50000
		latest.UpdatedAt = time.Now().UTC()
		err = store.Upsert(ctx, latest)
		if err != nil {
			t.Fatalf("first update: %v", err)
		}

		// Try update with stale version (version = 1 instead of 2).
		snap.BalanceSen = 75000
		err = store.Upsert(ctx, snap) // snap still has Version = 1, but DB has Version = 2
		if err == nil {
			t.Fatal("expected optimistic lock failure for stale version")
		}
	})

	t.Run("FindByUserID_NotFound", func(t *testing.T) {
		nonexistentID := uuid.Must(uuid.NewV7())
		snap, err := store.FindByUserID(ctx, nonexistentID)
		if err != nil {
			t.Fatalf("FindByUserID: %v", err)
		}
		if snap.UserID != uuid.Nil {
			t.Errorf("expected zero-value snapshot, got user_id %v", snap.UserID)
		}
	})
}

// createTestUser inserts a test user and returns their ID.
func createTestUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, phone, pin_hash, kyc_level, created_at) VALUES ($1, $2, $3, $4, $5)`,
		id, id.String()+"@phone", "$2a$12$testhash", types.KYCLevelBasic, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}
