package transactions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"senpay/internal/auth"
	"senpay/internal/types"

	"github.com/google/uuid"
)

// mockTxLogStore implements ledger.LedgerStore for testing.
type mockTxLogStore struct {
	txs      []types.Transaction
	findByID func(id uuid.UUID) (types.Transaction, error)
}

func (m *mockTxLogStore) Append(ctx context.Context, tx types.Transaction) error {
	return nil
}

func (m *mockTxLogStore) QueryByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]types.Transaction, string, error) {
	return m.txs, "", nil
}

func (m *mockTxLogStore) FindByID(ctx context.Context, id uuid.UUID) (types.Transaction, error) {
	if m.findByID != nil {
		return m.findByID(id)
	}
	return types.Transaction{}, types.ErrUserNotFound
}

// mockUserStore implements UserStore for testing.
type mockUserStore struct {
	users map[uuid.UUID]types.User
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{users: make(map[uuid.UUID]types.User)}
}

func (m *mockUserStore) addUser(id uuid.UUID, phone string) {
	m.users[id] = types.User{ID: id, Phone: phone}
}

func (m *mockUserStore) FindByID(ctx context.Context, id uuid.UUID) (types.User, error) {
	user, ok := m.users[id]
	if !ok {
		return types.User{}, types.ErrUserNotFound
	}
	return user, nil
}

func TestHandler_List_MissingAuth(t *testing.T) {
	t.Parallel()

	h := NewHandler(&mockTxLogStore{}, &mockUserStore{})
	req := httptest.NewRequest(http.MethodGet, "/v1/transactions", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

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

func TestHandler_List_Empty(t *testing.T) {
	t.Parallel()

	mockStore := &mockTxLogStore{txs: []types.Transaction{}}
	mockUsers := newMockUserStore()
	h := NewHandler(mockStore, mockUsers)

	userID := uuid.Must(uuid.NewV7())
	req := httptest.NewRequest(http.MethodGet, "/v1/transactions", nil)
	ctx := context.WithValue(req.Context(), auth.CtxKeyUserID, userID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
}

func TestHandler_List_WithTransactions(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())
	counterpartyID := uuid.Must(uuid.NewV7())

	now := "2025-01-01T00:00:00Z"
	_ = now

	tx1 := types.Transaction{
		ID:        uuid.Must(uuid.NewV7()),
		TxType:    types.TxTypeTransfer,
		SenderID:  &userID,
		ReceiverID: &counterpartyID,
		AmountSen: 50000,
		Currency:  types.CurrencyIDR,
		Status:    types.TxStatusCommitted,
	}

	mockStore := &mockTxLogStore{txs: []types.Transaction{tx1}}
	mockUsers := newMockUserStore()
	mockUsers.addUser(counterpartyID, "081234567890")
	h := NewHandler(mockStore, mockUsers)

	req := httptest.NewRequest(http.MethodGet, "/v1/transactions", nil)
	ctx := context.WithValue(req.Context(), auth.CtxKeyUserID, userID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(data))
	}

	tx := data[0].(map[string]interface{})
	if tx["tx_type"] != types.TxTypeTransfer {
		t.Errorf("expected tx_type %q, got %q", types.TxTypeTransfer, tx["tx_type"])
	}
	if tx["amount_sen"] != float64(50000) {
		t.Errorf("expected amount_sen 50000, got %v", tx["amount_sen"])
	}

	// Verify counterparty info.
	if tx["counterparty_id"] != counterpartyID.String() {
		t.Errorf("expected counterparty_id %q, got %q", counterpartyID.String(), tx["counterparty_id"])
	}
	if tx["counterparty_phone"] != "081234567890" {
		t.Errorf("expected counterparty_phone '081234567890', got %q", tx["counterparty_phone"])
	}

	// Verify pagination fields.
	nextCursor, ok := resp["next_cursor"]
	if !ok {
		t.Fatal("expected next_cursor field in response")
	}
	if nextCursor != "" {
		t.Errorf("expected empty next_cursor, got %q", nextCursor)
	}
	hasMore, ok := resp["has_more"]
	if !ok {
		t.Fatal("expected has_more field in response")
	}
	if hasMore != false {
		t.Errorf("expected has_more false, got %v", hasMore)
	}
}

func TestHandler_Detail_MissingAuth(t *testing.T) {
	t.Parallel()

	h := NewHandler(&mockTxLogStore{}, &mockUserStore{})
	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/some-id", nil)
	rec := httptest.NewRecorder()

	h.Detail(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// detailRequest creates a GET /v1/transactions/{id} request with auth context and path value set.
func detailRequest(t *testing.T, idStr string, userID uuid.UUID) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/transactions/"+idStr, nil)
	// Set path value for Go 1.22+ ServeMux pattern matching.
	req.SetPathValue("id", idStr)
	ctx := context.WithValue(req.Context(), auth.CtxKeyUserID, userID)
	return req.WithContext(ctx)
}

func TestHandler_Detail_NotFound(t *testing.T) {
	t.Parallel()

	mockStore := &mockTxLogStore{
		findByID: func(id uuid.UUID) (types.Transaction, error) {
			return types.Transaction{}, types.ErrUserNotFound
		},
	}
	h := NewHandler(mockStore, newMockUserStore())

	userID := uuid.Must(uuid.NewV7())
	txID := uuid.Must(uuid.NewV7())
	req := detailRequest(t, txID.String(), userID)
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error object in response")
	}
	if errObj["code"] != types.ErrCodeUserNotFound {
		t.Errorf("expected code %q, got %q", types.ErrCodeUserNotFound, errObj["code"])
	}
}

func TestHandler_Detail_InvalidUUID(t *testing.T) {
	t.Parallel()

	h := NewHandler(&mockTxLogStore{}, newMockUserStore())

	userID := uuid.Must(uuid.NewV7())
	req := detailRequest(t, "not-a-uuid", userID)
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_Detail_NotParticipant(t *testing.T) {
	t.Parallel()

	ownerID := uuid.Must(uuid.NewV7())
	otherUserID := uuid.Must(uuid.NewV7())

	txID := uuid.Must(uuid.NewV7())
	mockStore := &mockTxLogStore{
		findByID: func(id uuid.UUID) (types.Transaction, error) {
			if id == txID {
				return types.Transaction{
					ID:        txID,
					TxType:    types.TxTypeTransfer,
					SenderID:  &ownerID,
					ReceiverID: &otherUserID,
					AmountSen: 50000,
				}, nil
			}
			return types.Transaction{}, types.ErrUserNotFound
		},
	}
	h := NewHandler(mockStore, newMockUserStore())

	// Third user (not participant) tries to access.
	thirdUserID := uuid.Must(uuid.NewV7())
	req := detailRequest(t, txID.String(), thirdUserID)
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-participant, got %d", rec.Code)
	}
}

func TestHandler_Detail_Success(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())
	counterpartyID := uuid.Must(uuid.NewV7())

	txID := uuid.Must(uuid.NewV7())
	tx := types.Transaction{
		ID:         txID,
		TxType:     types.TxTypeTransfer,
		SenderID:   &userID,
		ReceiverID: &counterpartyID,
		AmountSen:  75000,
		Currency:   types.CurrencyIDR,
		Status:     types.TxStatusCommitted,
	}

	mockStore := &mockTxLogStore{
		findByID: func(id uuid.UUID) (types.Transaction, error) {
			if id == txID {
				return tx, nil
			}
			return types.Transaction{}, types.ErrUserNotFound
		},
	}
	mockUsers := newMockUserStore()
	mockUsers.addUser(counterpartyID, "08987654321")
	h := NewHandler(mockStore, mockUsers)

	req := detailRequest(t, txID.String(), userID)
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data object in response")
	}

	if data["id"] != txID.String() {
		t.Errorf("expected id %q, got %q", txID.String(), data["id"])
	}
	if data["tx_type"] != types.TxTypeTransfer {
		t.Errorf("expected tx_type %q, got %q", types.TxTypeTransfer, data["tx_type"])
	}
	if data["amount_sen"] != float64(75000) {
		t.Errorf("expected amount_sen 75000, got %v", data["amount_sen"])
	}
	if data["status"] != types.TxStatusCommitted {
		t.Errorf("expected status %q, got %q", types.TxStatusCommitted, data["status"])
	}

	// Verify counterparty.
	if data["counterparty_id"] != counterpartyID.String() {
		t.Errorf("expected counterparty_id %q, got %q", counterpartyID.String(), data["counterparty_id"])
	}
	if data["counterparty_phone"] != "08987654321" {
		t.Errorf("expected counterparty_phone '08987654321', got %q", data["counterparty_phone"])
	}
}
