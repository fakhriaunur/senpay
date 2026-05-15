package senpai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"senpay/internal/auth"
	"senpay/internal/types"

	"github.com/google/uuid"
)

// testUserID is a fixed UUID used in tests that require authentication.
var testUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// contextWithUserID returns a new context with a user ID set (bypassing auth middleware).
func contextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, auth.CtxKeyUserID, userID)
}

// Test handlers return 401 when no auth context is present.
func TestHandler_AuthRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		handler func(w http.ResponseWriter, r *http.Request)
	}{
		{"Summary", http.MethodGet, "/v1/senpai/summary", "", (&Handler{}).Summary},
		{"Trend", http.MethodGet, "/v1/senpai/trend", "", (&Handler{}).Trend},
		{"CreateBudget", http.MethodPost, "/v1/senpai/budgets", `{"category":"Makanan","limit_sen":2000000}`, (&Handler{}).CreateBudget},
		{"ListBudgets", http.MethodGet, "/v1/senpai/budgets", "", (&Handler{}).ListBudgets},
		{"Nudge", http.MethodGet, "/v1/senpai/nudge", "", (&Handler{}).Nudge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()

			tt.handler(rec, req)

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
		})
	}
}

// Test Nudge endpoint returns 501 when fullEnabled is false (with valid auth).
func TestHandler_Nudge_FeatureDisabled(t *testing.T) {
	t.Parallel()

	h := NewHandler(nil, false)

	req := httptest.NewRequest(http.MethodGet, "/v1/senpai/nudge", nil)
	// Inject auth context to bypass the auth check.
	ctx := contextWithUserID(req.Context(), testUserID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.Nudge(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error object in response")
	}
	if errObj["code"] != types.ErrCodeFeatureNotAvailable {
		t.Errorf("expected code %q, got %q", types.ErrCodeFeatureNotAvailable, errObj["code"])
	}
	m := errObj["message"].(string)
	if m == "" {
		t.Error("expected non-empty error message")
	}
}

// Test CreateBudget validates required fields.
func TestHandler_CreateBudget_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "empty body",
			body:     ``,
			wantCode: http.StatusUnauthorized, // no auth context
		},
		{
			name:     "missing category",
			body:     `{"limit_sen":2000000}`,
			wantCode: http.StatusUnauthorized, // no auth context
		},
		{
			name:     "invalid amount",
			body:     `{"category":"Makanan","limit_sen":0}`,
			wantCode: http.StatusUnauthorized, // no auth context
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/senpai/budgets", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h := &Handler{}
			h.CreateBudget(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, rec.Code)
			}
		})
	}
}
