package auth

import (
	"sync"
	"time"
)

// TokenStore tracks used/revoked tokens for single-use rotation.
// Current implementation uses an in-memory map with TTL-based cleanup.
// In production, this should be backed by Redis.
type TokenStore struct {
	mu       sync.RWMutex
	used     map[string]time.Time
	cleanupInterval time.Duration
	stopCh   chan struct{}
}

// NewTokenStore creates a new TokenStore and starts a background cleanup goroutine.
// usedTokensTTL is how long to remember used tokens (should match refresh token lifetime).
func NewTokenStore(cleanupInterval time.Duration) *TokenStore {
	ts := &TokenStore{
		used:            make(map[string]time.Time),
		cleanupInterval: cleanupInterval,
		stopCh:          make(chan struct{}),
	}
	go ts.cleanupLoop()
	return ts
}

// MarkUsed marks a token as used/invalidated.
// Returns true if the token was already used (double-spend detected).
func (ts *TokenStore) MarkUsed(tokenJTI string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if _, exists := ts.used[tokenJTI]; exists {
		return true // already used
	}
	ts.used[tokenJTI] = time.Now()
	return false
}

// IsUsed checks if a token has been marked as used.
func (ts *TokenStore) IsUsed(tokenJTI string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	_, exists := ts.used[tokenJTI]
	return exists
}

// Stop stops the background cleanup goroutine.
func (ts *TokenStore) Stop() {
	close(ts.stopCh)
}

// TokenCleanupThresholdDays is the age threshold in days for cleaning up used tokens.
// Set to 8 days (RefreshTokenDuration + buffer).
const TokenCleanupThresholdDays = 8

// cleanupLoop periodically removes expired entries.
// Since we use the map as an ever-growing set, entries are only removed
// when the store is stopped. For a production system with long TTLs,
// entries should be stored with expiry and cleaned up periodically.
func (ts *TokenStore) cleanupLoop() {
	ticker := time.NewTicker(ts.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ts.mu.Lock()
			// Remove entries older than 8 days (RefreshTokenDuration + buffer).
			threshold := time.Now().Add(-TokenCleanupThresholdDays * 24 * time.Hour)
			for key, added := range ts.used {
				if added.Before(threshold) {
					delete(ts.used, key)
				}
			}
			ts.mu.Unlock()
		case <-ts.stopCh:
			return
		}
	}
}
