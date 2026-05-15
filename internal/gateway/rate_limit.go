package gateway

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter.
// Thread-safe. Per-bucket state is maintained via sync.Mutex.
type TokenBucket struct {
	mu sync.Mutex

	rate    float64   // tokens per second
	burst   int       // maximum token count
	tokens  float64   // current token count
	lastRefill time.Time // last refill timestamp
}

// NewTokenBucket creates a new token bucket with the given rate (tokens/sec)
// and burst size (max accumulated tokens).
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	return &TokenBucket{
		rate:    rate,
		burst:   burst,
		tokens:  float64(burst),
		lastRefill: time.Now(),
	}
}

// Allow attempts to consume one token from the bucket.
// Returns true if the request is allowed, false if rate limited.
// If rate limited, returns the wait duration via Retry-After.
func (tb *TokenBucket) Allow() (ok bool, retryAfter time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time.
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.burst) {
		tb.tokens = float64(tb.burst)
	}
	tb.lastRefill = now

	// Try to consume a token.
	if tb.tokens >= 1 {
		tb.tokens--
		return true, 0
	}

	// Rate limited. Calculate retry-after.
	// Time until next token: (1 - tokens) / rate seconds.
	wait := time.Duration((1 - tb.tokens) / tb.rate * float64(time.Second))
	if wait < time.Second {
		wait = time.Second
	}
	return false, wait
}

// RateLimiter manages multiple token buckets keyed by a string (e.g., "ip:method:path").
type RateLimiter struct {
	mu sync.Mutex
	buckets map[string]*TokenBucket
	rate    float64
	burst   int
}

// NewRateLimiter creates a new rate limiter with the given per-key rate and burst.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*TokenBucket),
		rate:    rate,
		burst:   burst,
	}
}

// Allow checks if a request identified by key is allowed.
// Returns true if allowed, false if rate limited.
// If rate limited, returns the suggested Retry-After duration.
func (rl *RateLimiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	rl.mu.Lock()
	bucket, exists := rl.buckets[key]
	if !exists {
		bucket = NewTokenBucket(rl.rate, rl.burst)
		rl.buckets[key] = bucket
	}
	rl.mu.Unlock()

	return bucket.Allow()
}

// DefaultRateLimiter returns a rate limiter with sensible defaults:
// 100 requests per second, burst of 200.
func DefaultRateLimiter() *RateLimiter {
	return NewRateLimiter(100, 200)
}
