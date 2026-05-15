package wallet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"senpay/internal/types"
)

func TestHandler_Balance_MissingAuth(t *testing.T) {
	t.Parallel()

	h := NewHandler(nil) // nil pool, won't be reached
	req := httptest.NewRequest(http.MethodGet, "/v1/wallet/balance", nil)
	rec := httptest.NewRecorder()

	h.Balance(rec, req)

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
