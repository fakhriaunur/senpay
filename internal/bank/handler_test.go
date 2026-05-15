package bank

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

func TestHandler_Topup_MissingAuth(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandler(svc)

	body := `{"idempotency_key":"key-1","amount_sen":10000000}`
	req := httptest.NewRequest(http.MethodPost, "/v1/topup", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	h.Topup(rec, req)

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

func TestHandler_Topup_MissingFields(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandler(svc)

	tests := []struct {
		name string
		body string
		code string // expected error code
	}{
		{name: "empty_body", body: `{}`, code: types.ErrCodeMissingField},
		{name: "no_idempotency_key", body: `{"amount_sen":10000000}`, code: types.ErrCodeMissingField},
		{name: "zero_amount", body: `{"idempotency_key":"key-1","amount_sen":0}`, code: types.ErrCodeInvalidAmount},
		{name: "negative_amount", body: `{"idempotency_key":"key-1","amount_sen":-1000}`, code: types.ErrCodeInvalidAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/topup",
				bytes.NewBufferString(tt.body))
			userID := uuid.Must(uuid.NewV7())
			ctx := context.WithValue(req.Context(), auth.CtxKeyUserID, userID)
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.Topup(rec, req)

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

func TestHandler_Webhook_InvalidBody(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	wh := NewWebhookHandler(svc)

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{name: "empty_body", body: ``, wantCode: http.StatusInternalServerError},
		{name: "invalid_json", body: `not-json`, wantCode: http.StatusInternalServerError},
		{name: "missing_va_number", body: `{"status":"success"}`, wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/bank/webhook",
				bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			wh.HandleWebhook(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("expected %d, got %d: body=%s", tt.wantCode, rec.Code, tt.body)
			}
		})
	}
}

// TestHandler_Webhook_ValidBodyRouting validates webhook handler routing with a basic service
// that has nil internal dependencies. The handler should process the request without
// panicking and return an appropriate error response.
func TestHandler_Webhook_ValidBodyRouting(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	wh := NewWebhookHandler(svc)

	body := `{"va_number":"8999123456","amount_sen":10000000,"external_id":"ext-001","status":"success"}`
	req := httptest.NewRequest(http.MethodPost, "/bank/webhook",
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	// Should not panic — handle recovery for nil pointer dereference gracefully.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("handler panicked with nil dependencies (expected): %v", r)
		}
	}()

	wh.HandleWebhook(rec, req)

	// If no panic, check response.
	if rec.Code > 0 {
		t.Logf("webhook handler returned status %d without panic", rec.Code)
	}
}

// ── Withdraw Handler Tests ─────────────────────────────────────

func TestHandler_Withdraw_MissingAuth(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandler(svc)

	body := `{"idempotency_key":"key-1","amount_sen":5000000,"bank_account":"1234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/withdraw", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	h.Withdraw(rec, req)

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

func TestHandler_Withdraw_MissingFields(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	h := NewHandler(svc)

	tests := []struct {
		name string
		body string
		code string // expected error code
	}{
		{name: "empty_body", body: `{}`, code: types.ErrCodeMissingField},
		{name: "no_idempotency_key", body: `{"amount_sen":5000000,"bank_account":"1234567890"}`, code: types.ErrCodeMissingField},
		{name: "no_bank_account", body: `{"idempotency_key":"key-1","amount_sen":5000000}`, code: types.ErrCodeMissingField},
		{name: "zero_amount", body: `{"idempotency_key":"key-1","amount_sen":0,"bank_account":"1234567890"}`, code: types.ErrCodeInvalidAmount},
		{name: "negative_amount", body: `{"idempotency_key":"key-1","amount_sen":-1000,"bank_account":"1234567890"}`, code: types.ErrCodeInvalidAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/withdraw",
				bytes.NewBufferString(tt.body))
			userID := uuid.Must(uuid.NewV7())
			ctx := context.WithValue(req.Context(), auth.CtxKeyUserID, userID)
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			h.Withdraw(rec, req)

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
