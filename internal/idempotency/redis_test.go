//go:build integration

package idempotency

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTestAddr returns the Redis address to test against.
// Uses TEST_REDIS_URL env var, defaults to localhost:6379.
func redisTestAddr() string {
	if addr := os.Getenv("TEST_REDIS_URL"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

func newTestRedisClient(t *testing.T) (*redis.Client, func()) {
	t.Helper()

	client := redis.NewClient(&redis.Options{
		Addr: redisTestAddr(),
		DB:   1, // use DB 1 to avoid conflicts
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis connect: %v", err)
	}

	cleanup := func() {
		client.FlushDB(ctx)
		client.Close()
	}

	return client, cleanup
}

func TestRedis_SetIfNotExist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	cache := NewRedisIdempotencyCache(client)

	t.Run("first_acquire_returns_true", func(t *testing.T) {
		key := "test-setnx-1"
		defer client.Del(ctx, key)

		acquired, err := cache.SetIfNotExist(ctx, key, "in_flight", 10*time.Second)
		if err != nil {
			t.Fatalf("SetIfNotExist: %v", err)
		}
		if !acquired {
			t.Error("expected true for first acquire")
		}
	})

	t.Run("second_acquire_returns_false", func(t *testing.T) {
		key := "test-setnx-2"
		defer client.Del(ctx, key)

		acquired1, _ := cache.SetIfNotExist(ctx, key, "in_flight", 10*time.Second)
		if !acquired1 {
			t.Fatal("expected first acquire to succeed")
		}

		acquired2, err := cache.SetIfNotExist(ctx, key, "in_flight", 10*time.Second)
		if err != nil {
			t.Fatalf("SetIfNotExist second: %v", err)
		}
		if acquired2 {
			t.Error("expected false for second acquire (key already exists)")
		}
	})

	t.Run("key_auto_expires_after_ttl", func(t *testing.T) {
		key := "test-setnx-ttl"
		defer client.Del(ctx, key)

		// Set with very short TTL.
		acquired, err := cache.SetIfNotExist(ctx, key, "in_flight", 100*time.Millisecond)
		if err != nil {
			t.Fatalf("SetIfNotExist: %v", err)
		}
		if !acquired {
			t.Fatal("expected first acquire to succeed")
		}

		// Wait for TTL to expire.
		time.Sleep(200 * time.Millisecond)

		// Should be able to acquire again after expiry.
		acquired2, err := cache.SetIfNotExist(ctx, key, "completed", 10*time.Second)
		if err != nil {
			t.Fatalf("SetIfNotExist after expiry: %v", err)
		}
		if !acquired2 {
			t.Error("expected true after TTL expiry (key should have been auto-deleted)")
		}
	})
}

func TestRedis_Get(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	cache := NewRedisIdempotencyCache(client)

	t.Run("get_returns_set_value", func(t *testing.T) {
		key := "test-get-1"
		defer client.Del(ctx, key)

		err := cache.Set(ctx, key, "completed", 10*time.Second)
		if err != nil {
			t.Fatalf("Set: %v", err)
		}

		val, err := cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if val != "completed" {
			t.Errorf("got %q, want %q", val, "completed")
		}
	})

	t.Run("get_returns_empty_for_nonexistent_key", func(t *testing.T) {
		val, err := cache.Get(ctx, "nonexistent-key")
		if err != nil {
			t.Fatalf("Get nonexistent: %v", err)
		}
		if val != "" {
			t.Errorf("got %q, want empty string", val)
		}
	})

	t.Run("get_returns_empty_after_expiry", func(t *testing.T) {
		key := "test-get-expiry"
		defer client.Del(ctx, key)

		err := cache.Set(ctx, key, "completed", 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Set: %v", err)
		}

		time.Sleep(200 * time.Millisecond)

		val, err := cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after expiry: %v", err)
		}
		if val != "" {
			t.Errorf("got %q, want empty string after TTL expiry", val)
		}
	})
}

func TestRedis_Set(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	cache := NewRedisIdempotencyCache(client)

	t.Run("set_overwrites_existing_value", func(t *testing.T) {
		key := "test-set-overwrite"
		defer client.Del(ctx, key)

		// Set initial value.
		err := cache.Set(ctx, key, "in_flight", 10*time.Second)
		if err != nil {
			t.Fatalf("Set initial: %v", err)
		}

		// Overwrite with new value.
		err = cache.Set(ctx, key, "completed", 24*time.Hour)
		if err != nil {
			t.Fatalf("Set overwrite: %v", err)
		}

		val, err := cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if val != "completed" {
			t.Errorf("got %q, want %q", val, "completed")
		}
	})

	t.Run("set_respects_ttl_expiry", func(t *testing.T) {
		key := "test-set-ttl"
		defer client.Del(ctx, key)

		err := cache.Set(ctx, key, "completed", 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Set: %v", err)
		}

		// Key should exist before TTL expires.
		val, err := cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get before expiry: %v", err)
		}
		if val != "completed" {
			t.Errorf("got %q, want %q", val, "completed")
		}

		// Wait for TTL to expire.
		time.Sleep(200 * time.Millisecond)

		// Key should be gone after expiry.
		val, err = cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after expiry: %v", err)
		}
		if val != "" {
			t.Errorf("got %q, want empty string after TTL expiry", val)
		}
	})
}

func TestRedis_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	cache := NewRedisIdempotencyCache(client)

	t.Run("delete_removes_key", func(t *testing.T) {
		key := "test-del-1"
		defer client.Del(ctx, key)

		err := cache.Set(ctx, key, "in_flight", 10*time.Second)
		if err != nil {
			t.Fatalf("Set: %v", err)
		}

		err = cache.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}

		val, err := cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after delete: %v", err)
		}
		if val != "" {
			t.Errorf("got %q, want empty string after delete", val)
		}
	})

	t.Run("delete_nonexistent_key_does_not_error", func(t *testing.T) {
		err := cache.Delete(ctx, "nonexistent-key-for-delete")
		if err != nil {
			t.Errorf("Delete nonexistent returned error: %v", err)
		}
	})
}

func TestRedis_FullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := newTestRedisClient(t)
	defer cleanup()

	cache := NewRedisIdempotencyCache(client)
	key := "test-lifecycle"
	defer client.Del(ctx, key)

	// Step 1: SETNX (in-flight marker) — should succeed.
	acquired, err := cache.SetIfNotExist(ctx, key, "in_flight", InFlightTTL)
	if err != nil {
		t.Fatalf("SetIfNotExist (step 1): %v", err)
	}
	if !acquired {
		t.Fatal("expected to acquire in-flight marker")
	}

	// Step 2: GET — should show in_flight.
	val, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get (step 2): %v", err)
	}
	if val != "in_flight" {
		t.Errorf("got %q, want %q", val, "in_flight")
	}

	// Step 3: Delete in-flight marker and SET completed.
	err = cache.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete (step 3): %v", err)
	}

	acquired, err = cache.SetIfNotExist(ctx, key, "completed", DefaultIdempotencyTTL)
	if err != nil {
		t.Fatalf("SetIfNotExist completed (step 3): %v", err)
	}
	if !acquired {
		t.Fatal("expected to acquire after deleting in-flight marker")
	}

	// Step 4: GET — should show completed.
	val, err = cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get (step 4): %v", err)
	}
	if val != "completed" {
		t.Errorf("got %q, want %q", val, "completed")
	}

	// Step 5: SetIfNotExist again — should fail (already exists).
	acquired, err = cache.SetIfNotExist(ctx, key, "in_flight", InFlightTTL)
	if err != nil {
		t.Fatalf("SetIfNotExist duplicate (step 5): %v", err)
	}
	if acquired {
		t.Error("expected false for duplicate acquire (key already exists as completed)")
	}
}
