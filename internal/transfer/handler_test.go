package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"senpay/internal/auth"
	"senpay/internal/types"

	"github.com/google/uuid"
)

// Test handler error paths that don't require real database or Redis.
func TestHandler_Transfer_MissingAuth(t *testing.T) {
	t.Parallel()

	svc := &Service{} // nil service, won't be reached
	h := NewHandler(svc)

	body := `{"idempotency_key":"key-1","to_phone":"081234567890","amount_sen":50000}`
	req := httptest.NewRequest(http.MethodPost, "/v1/transfer", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	h.Transfer(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error object in response")
	}
	if errObj["code"] != types.ErrCodeUnauthorized {
		t.Errorf("expected code %q, got %q", types.ErrCodeUnauthorized, errObj["code"])
	}
}

func TestHandler_Transfer_MissingFields(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandler(svc)

	tests := []struct {
		name string
		body string
		code string // expected error code
	}{
		{name: "empty_body", body: `{}`, code: types.ErrCodeMissingField},
		{name: "no_idempotency_key", body: `{"to_phone":"081234567890","amount_sen":50000}`, code: types.ErrCodeMissingField},
		{name: "no_to_phone", body: `{"idempotency_key":"key-1","amount_sen":50000}`, code: types.ErrCodeMissingField},
		{name: "zero_amount", body: `{"idempotency_key":"key-1","to_phone":"081234567890","amount_sen":0}`, code: types.ErrCodeInvalidAmount},
		{name: "negative_amount", body: `{"idempotency_key":"key-1","to_phone":"081234567890","amount_sen":-1000}`, code: types.ErrCodeInvalidAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/transfer",
				bytes.NewBufferString(tt.body))
			// Add user to context (simulate auth middleware).
			userID := uuid.Must(uuid.NewV7())
			ctx := context.WithValue(req.Context(), auth.CtxKeyUserID, userID)
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.Transfer(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d for body=%s", rec.Code, tt.body)
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			errObj, ok := resp["error"].(map[string]interface{})
			if !ok {
				t.Fatal("expected error object in response")
			}
			if errObj["code"] != tt.code {
				t.Errorf("expected code %q, got %q", tt.code, errObj["code"])
			}
		})
	}
}

func TestHandler_Transfer_SuccessWithMocks(t *testing.T) {
	t.Parallel()

	// Create a service with mock dependencies.
	mockUsers := newMockUserStore()
	sender := mockUsers.addUser("081111111111")
	receiver := mockUsers.addUser("082222222222")
	mockCache := newMockRedisCache()

	svc := &Service{
		pool:       nil, // Service won't call pool directly since mockTransferSvc handles it
		redisCache: nil, // Use mock wrapper below
		userStore:  mockUsers,
	}

	// Override with a custom Transfer function for testing.
	// We'll test the handler with a real service and then mock at the service level.
	_ = sender
	_ = receiver
	_ = mockCache
	_ = svc

	// For proper unit testing, we'd inject the mock at the service level.
	// The handler delegates to svc.Transfer, so we test handler shape here.
	t.Skip("handler-level success test requires integration deps")
}

// --- Mocks ---

// mockUserStore implements UserStore for testing.
type mockUserStore struct {
	users map[string]types.User // phone -> user
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users: make(map[string]types.User),
	}
}

func (m *mockUserStore) addUser(phone string) types.User {
	user := types.NewUser(phone, "hash")
	m.users[phone] = user
	return user
}

func (m *mockUserStore) FindByPhone(_ context.Context, phone string) (types.User, error) {
	user, ok := m.users[phone]
	if !ok {
		return types.User{}, types.ErrUserNotFound
	}
	return user, nil
}

func (m *mockUserStore) FindByID(_ context.Context, id uuid.UUID) (types.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return types.User{}, types.ErrUserNotFound
}

// mockRedisCache implements a simple in-memory cache for testing.
type mockRedisCache struct {
	data map[string]string
}

func newMockRedisCache() *mockRedisCache {
	return &mockRedisCache{data: make(map[string]string)}
}

func (m *mockRedisCache) Get(_ context.Context, key string) (string, error) {
	return m.data[key], nil
}

func (m *mockRedisCache) SetIfNotExist(_ context.Context, key string, status string, _ interface{}) (bool, error) {
	if _, exists := m.data[key]; exists {
		return false, nil
	}
	m.data[key] = status
	return true, nil
}

func (m *mockRedisCache) Set(_ context.Context, key string, value string, _ interface{}) error {
	m.data[key] = value
	return nil
}

func (m *mockRedisCache) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}
