//go:build integration

package idempotency

import (
	"context"
	"testing"

	"senpay/internal/store/storetest"
)

func TestPostgres_IdempotencyStore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer cleanup()

	store := NewPostgresIdempotencyStore(pool)

	t.Run("Insert_FindByKey", func(t *testing.T) {
		key := "test-key-1"
		err := store.Insert(ctx, key, "in_flight")
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		status, err := store.FindByKey(ctx, key)
		if err != nil {
			t.Fatalf("FindByKey: %v", err)
		}
		if status != "in_flight" {
			t.Errorf("status: got %q, want %q", status, "in_flight")
		}
	})

	t.Run("Insert_DuplicateKey", func(t *testing.T) {
		key := "test-key-duplicate"
		err := store.Insert(ctx, key, "in_flight")
		if err != nil {
			t.Fatalf("Insert first: %v", err)
		}

		err = store.Insert(ctx, key, "completed")
		if err == nil {
			t.Fatal("expected error for duplicate key")
		}
	})

	t.Run("FindByKey_NotFound", func(t *testing.T) {
		status, err := store.FindByKey(ctx, "nonexistent-key")
		if err != nil {
			t.Fatalf("FindByKey: %v", err)
		}
		if status != "" {
			t.Errorf("status: got %q, want empty string", status)
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		key := "test-key-update"
		err := store.Insert(ctx, key, "in_flight")
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		err = store.UpdateStatus(ctx, key, "completed")
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}

		status, err := store.FindByKey(ctx, key)
		if err != nil {
			t.Fatalf("FindByKey: %v", err)
		}
		if status != "completed" {
			t.Errorf("status: got %q, want %q", status, "completed")
		}
	})

	t.Run("UpdateStatus_KeyNotFound", func(t *testing.T) {
		err := store.UpdateStatus(ctx, "nonexistent-key", "completed")
		if err == nil {
			t.Fatal("expected error for nonexistent key")
		}
	})

	t.Run("FullLifecycle", func(t *testing.T) {
		key := "test-key-lifecycle"

		// Insert as in_flight.
		err := store.Insert(ctx, key, "in_flight")
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}

		status1, _ := store.FindByKey(ctx, key)
		if status1 != "in_flight" {
			t.Errorf("status after insert: got %q, want %q", status1, "in_flight")
		}

		// Update to completed.
		err = store.UpdateStatus(ctx, key, "completed")
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}

		status2, _ := store.FindByKey(ctx, key)
		if status2 != "completed" {
			t.Errorf("status after update: got %q, want %q", status2, "completed")
		}

		// Update to failed.
		err = store.UpdateStatus(ctx, key, "failed")
		if err != nil {
			t.Fatalf("UpdateStatus to failed: %v", err)
		}

		status3, _ := store.FindByKey(ctx, key)
		if status3 != "failed" {
			t.Errorf("status after update: got %q, want %q", status3, "failed")
		}
	})

	t.Run("Concurrent_InsertUniqueKeys", func(t *testing.T) {
		// Verify that inserting with unique keys works fine concurrently.
		for i := 0; i < 5; i++ {
			key := "test-concurrent-" + itoa(i)
			err := store.Insert(ctx, key, "in_flight")
			if err != nil {
				t.Fatalf("Insert %s: %v", key, err)
			}
		}
	})
}

// itoa is a simple int-to-string converter.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
