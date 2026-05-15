package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"senpay/internal/auth"
	"senpay/internal/types"

	"github.com/google/uuid"
)

// --- Mock UserStore for BI limit tests ---

type mockUserStore struct {
	users map[uuid.UUID]types.User
}

func (m *mockUserStore) FindByID(_ context.Context, id uuid.UUID) (types.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return types.User{}, types.ErrUserNotFound
}

func newMockStore() *mockUserStore {
	return &mockUserStore{users: make(map[uuid.UUID]types.User)}
}

func (m *mockUserStore) addUser(id uuid.UUID, kycLevel string) {
	m.users[id] = types.User{
		ID:       id,
		Phone:    "081234567890",
		KYCLevel: kycLevel,
	}
}

// --- Helper functions ---

// okHandler returns an http.Handler that responds with 200 OK.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}

// panicHandler returns an http.Handler that panics.
func panicHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})
}

// readResponse decodes a JSON response body.
func readResponse(t *testing.T, w *httptest.ResponseRecorder) (int, map[string]interface{}) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return w.Code, resp
}

// --- Tests ---

func TestRecoveryMiddleware(t *testing.T) {
	t.Run("recovers_from_panic", func(t *testing.T) {
		wrapped := Recovery(panicHandler())

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}

		code, resp := readResponse(t, w)
		if code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", code)
		}
		errData, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error in response")
		}
		if errData["code"] != "INTERNAL_ERROR" {
			t.Errorf("expected INTERNAL_ERROR, got %v", errData["code"])
		}
		if errData["message"] != "Terjadi kesalahan internal" {
			t.Errorf("expected Indonesian message, got %v", errData["message"])
		}
	})

	t.Run("passes_through_normal_requests", func(t *testing.T) {
		wrapped := Recovery(okHandler())

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("recovers_with_request_id_in_context", func(t *testing.T) {
		wrapped := RequestID(Recovery(panicHandler()))

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
		// Should not crash — recovery works even with request ID in context.
	})
}

func TestRequestIDMiddleware(t *testing.T) {
	t.Run("generates_uuid_when_missing", func(t *testing.T) {
		wrapped := RequestID(okHandler())

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Fatal("expected X-Request-ID header")
		}
		_, err := uuid.Parse(requestID)
		if err != nil {
			t.Fatalf("expected valid UUID, got %q: %v", requestID, err)
		}
	})

	t.Run("preserves_existing_request_id", func(t *testing.T) {
		wrapped := RequestID(okHandler())

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		r.Header.Set("X-Request-ID", "test-request-123")
		wrapped.ServeHTTP(w, r)

		requestID := w.Header().Get("X-Request-ID")
		if requestID != "test-request-123" {
			t.Fatalf("expected 'test-request-123', got %q", requestID)
		}
	})

	t.Run("injects_into_context", func(t *testing.T) {
		checkHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := GetRequestID(r.Context())
			if rid == "" {
				t.Error("expected request ID in context")
			}
			w.WriteHeader(http.StatusOK)
		})

		wrapped := RequestID(checkHandler)

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		r.Header.Set("X-Request-ID", "ctx-test-id")
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

func TestLoggingMiddleware(t *testing.T) {
	t.Run("captures_status_code", func(t *testing.T) {
		// Handler that returns a specific status.
		statusHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		})

		wrapped := RequestID(Logging(statusHandler))

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("passes_through_success", func(t *testing.T) {
		wrapped := RequestID(Logging(okHandler()))

		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/data", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("writes_body_correctly", func(t *testing.T) {
		wrapped := RequestID(Logging(okHandler()))

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		code, resp := readResponse(t, w)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if resp["status"] != "ok" {
			t.Errorf("expected status=ok, got %v", resp["status"])
		}
	})
}

func TestRateLimitMiddleware(t *testing.T) {
	t.Run("allows_requests_within_limit", func(t *testing.T) {
		// Create a rate limiter with high rate.
		rl := NewRateLimiter(1000, 1000)
		wrapped := RateLimit(rl)(okHandler())

		// Send 10 requests — all should pass.
		for i := 0; i < 10; i++ {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/v1/test", nil)
			wrapped.ServeHTTP(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("request %d: expected 200, got %d", i, w.Code)
			}
		}
	})

	t.Run("rate_limits_after_exceeding_burst", func(t *testing.T) {
		// Create a rate limiter with burst of 1, slow refill.
		rl := NewRateLimiter(0.1, 1) // 0.1 tokens/sec, burst 1
		wrapped := RateLimit(rl)(okHandler())

		// First request should pass.
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("first request: expected 200, got %d", w.Code)
		}

		// Second request should be rate limited (burst exhausted, rate too slow).
		w = httptest.NewRecorder()
		r = httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("second request: expected 429, got %d", w.Code)
		}

		// Check Retry-After header.
		retryAfter := w.Header().Get("Retry-After")
		if retryAfter == "" {
			t.Error("expected Retry-After header on 429 response")
		}
	})

	t.Run("rate_limit_returns_429_body", func(t *testing.T) {
		rl := NewRateLimiter(0.1, 1)
		wrapped := RateLimit(rl)(okHandler())

		// Exhaust the burst.
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r) // passes

		// Second request: rate limited.
		w = httptest.NewRecorder()
		r = httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", w.Code)
		}

		code, resp := readResponse(t, w)
		if code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", code)
		}
		errData, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error in response")
		}
		if errData["code"] != "RATE_LIMITED" {
			t.Errorf("expected RATE_LIMITED, got %v", errData["code"])
		}
	})

	t.Run("separate_keys_have_separate_buckets", func(t *testing.T) {
		rl := NewRateLimiter(0.1, 1) // 0.1 tokens/sec, burst 1
		wrapped := RateLimit(rl)(okHandler())

		// Exhaust bucket for /v1/test.
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		wrapped.ServeHTTP(w, r)

		// Different path should have its own bucket (first request passes).
		w = httptest.NewRecorder()
		r = httptest.NewRequest("GET", "/v1/other", nil)
		wrapped.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("different path: expected 200, got %d", w.Code)
		}
	})
}

func TestBILimitMiddleware(t *testing.T) {
	basicUserID := uuid.Must(uuid.NewV7())
	verifiedUserID := uuid.Must(uuid.NewV7())

	store := newMockStore()
	store.addUser(basicUserID, types.KYCLevelBasic)
	store.addUser(verifiedUserID, types.KYCLevelVerified)

	t.Run("basic_user_blocked_above_limit", func(t *testing.T) {
		// Create handler with auth context and BI limit.
		biMiddleware := BILimit(store)
		wrapped := biMiddleware(okHandler())

		body := `{"amount_sen":250000}` // Rp 2.500.000 > Rp 2.000.000 limit
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/transfer", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = setUserContext(r, basicUserID)

		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", w.Code)
		}

		code, resp := readResponse(t, w)
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", code)
		}
		errData, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error in response")
		}
		if errData["code"] != ErrCodeBILimitExceeded {
			t.Errorf("expected %s, got %v", ErrCodeBILimitExceeded, errData["code"])
		}
		msg, _ := errData["message"].(string)
		if !strings.Contains(msg, "transaksi") {
			t.Errorf("expected Indonesian message about limit, got %q", msg)
		}
	})

	t.Run("basic_user_allowed_within_limit", func(t *testing.T) {
		biMiddleware := BILimit(store)
		wrapped := biMiddleware(okHandler())

		body := `{"amount_sen":150000}` // Rp 1.500.000 < Rp 2.000.000 limit
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/transfer", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = setUserContext(r, basicUserID)

		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for within-limit amount, got %d", w.Code)
		}
	})

	t.Run("verified_user_allowed_above_basic_limit", func(t *testing.T) {
		biMiddleware := BILimit(store)
		wrapped := biMiddleware(okHandler())

		body := `{"amount_sen":500000}` // Rp 5.000.000 > Rp 2M but < Rp 10M
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/transfer", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = setUserContext(r, verifiedUserID)

		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for verified user within limit, got %d", w.Code)
		}
	})

	t.Run("verified_user_blocked_above_limit", func(t *testing.T) {
		biMiddleware := BILimit(store)
		wrapped := biMiddleware(okHandler())

		body := `{"amount_sen":1100000}` // Rp 11.000.000 > Rp 10.000.000 limit
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/transfer", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = setUserContext(r, verifiedUserID)

		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for verified above limit, got %d", w.Code)
		}
	})

	t.Run("get_request_passes_through", func(t *testing.T) {
		biMiddleware := BILimit(store)
		wrapped := biMiddleware(okHandler())

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/balance", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for GET, got %d", w.Code)
		}
	})

	t.Run("no_amount_field_passes_through", func(t *testing.T) {
		biMiddleware := BILimit(store)
		wrapped := biMiddleware(okHandler())

		body := `{"some_other_field":"value"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/transfer", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = setUserContext(r, basicUserID)

		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for request without amount, got %d", w.Code)
		}
	})

	t.Run("boundary_value_at_limit_is_allowed", func(t *testing.T) {
		biMiddleware := BILimit(store)
		wrapped := biMiddleware(okHandler())

		// Exactly at the basic limit (200,000 sen).
		body := `{"amount_sen":200000}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/transfer", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = setUserContext(r, basicUserID)

		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for amount at exact limit, got %d", w.Code)
		}
	})
}

// --- Test helpers ---

// setUserContext sets the user ID in the request context using the same
// context key type as the auth middleware.
func setUserContext(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := context.WithValue(r.Context(), auth.CtxKeyUserID, userID)
	return r.WithContext(ctx)
}

func TestTokenBucket(t *testing.T) {
	t.Run("allows_initial_burst", func(t *testing.T) {
		tb := NewTokenBucket(10, 5)

		// Should allow up to burst (5) requests immediately.
		for i := 0; i < 5; i++ {
			ok, _ := tb.Allow()
			if !ok {
				t.Fatalf("request %d: expected allowed, got denied", i)
			}
		}

		// 6th request should be denied (burst exhausted).
		ok, _ := tb.Allow()
		if ok {
			t.Error("expected 6th request to be denied")
		}
	})

	t.Run("refills_over_time", func(t *testing.T) {
		tb := NewTokenBucket(10, 5)

		// Exhaust all tokens.
		for i := 0; i < 5; i++ {
			tb.Allow()
		}

		// Wait for refill.
		time.Sleep(200 * time.Millisecond)

		// Should have at least 1 token (10 * 0.2 = 2).
		ok, _ := tb.Allow()
		if !ok {
			t.Error("expected token to be available after refill")
		}
	})

	t.Run("returns_retry_after_when_limited", func(t *testing.T) {
		tb := NewTokenBucket(1, 0) // 1 token/sec, burst 0 — always empty

		ok, retryAfter := tb.Allow()
		if ok {
			t.Fatal("expected rate limited with burst 0")
		}
		if retryAfter <= 0 {
			t.Errorf("expected positive retry-after, got %v", retryAfter)
		}
	})
}

func TestRateLimiter(t *testing.T) {
	t.Run("separate_keys_independent", func(t *testing.T) {
		rl := NewRateLimiter(100, 50)

		// Both keys should be allowed independently.
		for i := 0; i < 10; i++ {
			ok1, _ := rl.Allow("key1")
			ok2, _ := rl.Allow("key2")
			if !ok1 {
				t.Fatalf("key1 iteration %d: expected allowed", i)
			}
			if !ok2 {
				t.Fatalf("key2 iteration %d: expected allowed", i)
			}
		}
	})
}

func TestMiddlewareChain(t *testing.T) {
	t.Run("request_id_recovery_logging_chain", func(t *testing.T) {
		// Chain: Recovery -> RequestID -> Logging -> handler
		chain := Recovery(RequestID(Logging(okHandler())))

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		chain.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		// X-Request-ID should be set.
		if w.Header().Get("X-Request-ID") == "" {
			t.Error("expected X-Request-ID header")
		}
	})

	t.Run("recovery_catches_panic_in_chain", func(t *testing.T) {
		chain := Recovery(RequestID(Logging(panicHandler())))

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/test", nil)
		r.Header.Set("X-Request-ID", "panic-test")
		chain.ServeHTTP(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	t.Run("full_chain_with_bi_limit", func(t *testing.T) {
		userID := uuid.Must(uuid.NewV7())
		store := newMockStore()
		store.addUser(userID, types.KYCLevelBasic)

		rl := NewRateLimiter(1000, 1000)
		chain := Recovery(RequestID(Logging(RateLimit(rl)(BILimit(store)(okHandler())))))

		w := httptest.NewRecorder()
		body := `{"amount_sen":250000}`
		r := httptest.NewRequest("POST", "/v1/transfer", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = setUserContext(r, userID)

		chain.ServeHTTP(w, r)

		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for basic user above limit, got %d", w.Code)
		}
	})
}
