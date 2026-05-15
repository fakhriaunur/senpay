package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisIdempotencyCache provides idempotency key caching backed by Redis.
// Uses SETNX for atomic acquire and SET for status updates with TTL.
// Keys auto-expire after 24 hours as per the idempotency contract.
type RedisIdempotencyCache struct {
	client *redis.Client
}

// NewRedisIdempotencyCache creates a new RedisIdempotencyCache.
func NewRedisIdempotencyCache(client *redis.Client) *RedisIdempotencyCache {
	return &RedisIdempotencyCache{client: client}
}

// SetIfNotExist atomically sets a key with the given status and TTL if it does not exist.
// Returns true if the key was acquired (first caller), false if it already exists.
func (c *RedisIdempotencyCache) SetIfNotExist(ctx context.Context, key string, status string, ttl time.Duration) (bool, error) {
	ok, err := c.client.SetNX(ctx, key, status, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx: %w", err)
	}
	return ok, nil
}

// Get retrieves the status of an idempotency key.
// Returns empty string if the key does not exist or has expired.
func (c *RedisIdempotencyCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", fmt.Errorf("redis get: %w", err)
	}
	return val, nil
}

// Set updates the status of an existing idempotency key with the given TTL.
// Overwrites any existing value regardless of prior status.
func (c *RedisIdempotencyCache) Set(ctx context.Context, key string, status string, ttl time.Duration) error {
	err := c.client.Set(ctx, key, status, ttl).Err()
	if err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Delete removes an idempotency key from the cache.
// Used for clearing in-flight markers after completion.
func (c *RedisIdempotencyCache) Delete(ctx context.Context, key string) error {
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

// DefaultIdempotencyTTL is the standard TTL for idempotency keys: 24 hours.
const DefaultIdempotencyTTL = 24 * time.Hour

// InFlightTTL is the TTL for in-flight markers: 30 seconds.
// This is short because in-flight markers are only needed briefly
// while the request is being processed.
const InFlightTTL = 30 * time.Second
