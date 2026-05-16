// Package transactions provides HTTP handlers for transaction history endpoints.
//
// FCIS structure:
//   - handler.go: Shell — parses HTTP request, calls stores, formats JSON response.
//
// Endpoints:
//   - GET /v1/transactions — paginated list with cursor-based pagination
//   - GET /v1/transactions/{id} — full transaction detail including counterparty info
package transactions

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"senpay/internal/auth"
	"senpay/internal/ledger"
	"senpay/internal/types"

	"github.com/google/uuid"
)

// Handler implements HTTP handlers for transaction endpoints.
type Handler struct {
	txLogStore ledger.LedgerStore
	userStore  UserStore
}

// UserStore defines the interface for looking up users (counterparty info).
type UserStore interface {
	FindByID(ctx context.Context, id uuid.UUID) (types.User, error)
}

// NewHandler creates a new transactions Handler.
func NewHandler(txLogStore ledger.LedgerStore, userStore UserStore) *Handler {
	return &Handler{
		txLogStore: txLogStore,
		userStore:  userStore,
	}
}

// TransactionResponse is the full transaction detail returned in API responses.
type TransactionResponse struct {
	ID                uuid.UUID  `json:"id"`
	IdempotencyKey    string     `json:"idempotency_key"`
	TxType            string     `json:"tx_type"`
	SenderID          *uuid.UUID `json:"sender_id,omitempty"`
	ReceiverID        *uuid.UUID `json:"receiver_id,omitempty"`
	CounterpartyID    *uuid.UUID `json:"counterparty_id,omitempty"`
	CounterpartyPhone string     `json:"counterparty_phone,omitempty"`
	AmountSen         int64      `json:"amount_sen"`
	Currency          string     `json:"currency"`
	Status            string     `json:"status"`
	FailureReason     *string    `json:"failure_reason,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	CommittedAt       *time.Time `json:"committed_at,omitempty"`
}

// ListResponse is the paginated list response.
type ListResponse struct {
	Data       []TransactionResponse `json:"data"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
}

// List handles GET /v1/transactions.
//
// Returns a paginated list of transactions for the authenticated user.
// Supports cursor-based pagination with ?cursor=<cursor>&limit=<n>.
// Results are ordered by created_at DESC (newest first).
// The sender is the authenticated user, identified from JWT auth context.
//
// Query parameters:
//   - cursor: opaque cursor string from previous response (empty for first page)
//   - limit: maximum items per page (default 20, max 100)
//
// Requires authentication via auth middleware.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit := types.PageDefaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Enforce max limit.
	if limit > types.PageMaxLimit {
		limit = types.PageMaxLimit
	}

	txs, nextCursor, err := h.txLogStore.QueryByUserID(r.Context(), userID, cursor, limit)
	if err != nil {
		slog.Error("failed to query transactions", "user_id", userID, "error", err)
		writeJSONError(w, types.ErrInternal)
		return
	}

	// Build response with counterparty info.
	resp := make([]TransactionResponse, 0, len(txs))
	for _, tx := range txs {
		txResp := toTransactionResponse(tx, userID)
		// Enrich with counterparty phone.
		txResp = h.enrichCounterparty(r.Context(), txResp, tx, userID)
		resp = append(resp, txResp)
	}

	listResp := ListResponse{
		Data:       resp,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data":       listResp.Data,
		"next_cursor": listResp.NextCursor,
		"has_more":   listResp.HasMore,
	})
}

// Detail handles GET /v1/transactions/{id}.
//
// Returns the full transaction detail for the given transaction ID.
// The authenticated user must be either the sender or receiver of the transaction.
// Includes counterparty info (phone number).
//
// Requires authentication via auth middleware.
// Returns 200 with full transaction detail on success.
// Returns 404 if the transaction doesn't exist or doesn't belong to the user.
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	// Extract transaction ID from the URL path.
	idStr := r.PathValue("id")
	txID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, types.DomainError{
			Code:       types.ErrCodeInvalidFormat,
			Message:    "ID transaksi tidak valid",
			HTTPStatus: 400,
		})
		return
	}

	tx, err := h.txLogStore.FindByID(r.Context(), txID)
	if err != nil {
		// Check if it's a USER_NOT_FOUND DomainError (tx not found).
		if isDomainError(err) {
			writeJSONError(w, types.ErrUserNotFound)
			return
		}
		slog.Error("failed to find transaction", "tx_id", txID, "error", err)
		writeJSONError(w, types.ErrInternal)
		return
	}

	// Verify the transaction belongs to the authenticated user.
	if !isUserParticipant(tx, userID) {
		writeJSONError(w, types.ErrUserNotFound)
		return
	}

	txResp := toTransactionResponse(tx, userID)
	txResp = h.enrichCounterparty(r.Context(), txResp, tx, userID)

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data": txResp,
	})
}

// isUserParticipant checks if the user is either the sender or receiver of the transaction.
func isUserParticipant(tx types.Transaction, userID uuid.UUID) bool {
	if tx.SenderID != nil && *tx.SenderID == userID {
		return true
	}
	if tx.ReceiverID != nil && *tx.ReceiverID == userID {
		return true
	}
	return false
}

// getCounterpartyID returns the counterparty ID for a transaction relative to the given user.
func getCounterpartyID(tx types.Transaction, userID uuid.UUID) *uuid.UUID {
	if tx.SenderID != nil && *tx.SenderID == userID && tx.ReceiverID != nil {
		return tx.ReceiverID
	}
	if tx.ReceiverID != nil && *tx.ReceiverID == userID && tx.SenderID != nil {
		return tx.SenderID
	}
	return nil
}

// toTransactionResponse converts a types.Transaction to a TransactionResponse.
func toTransactionResponse(tx types.Transaction, userID uuid.UUID) TransactionResponse {
	return TransactionResponse{
		ID:             tx.ID,
		IdempotencyKey: tx.IdempotencyKey,
		TxType:         tx.TxType.String(),
		SenderID:       tx.SenderID,
		ReceiverID:     tx.ReceiverID,
		CounterpartyID: getCounterpartyID(tx, userID),
		AmountSen:      tx.AmountSen,
		Currency:       tx.Currency,
		Status:         tx.Status.String(),
		FailureReason:  tx.FailureReason,
		CreatedAt:      tx.CreatedAt,
		CommittedAt:    tx.CommittedAt,
	}
}

// enrichCounterparty adds the counterparty's phone number to the response.
func (h *Handler) enrichCounterparty(ctx context.Context, resp TransactionResponse, tx types.Transaction, userID uuid.UUID) TransactionResponse {
	counterpartyID := getCounterpartyID(tx, userID)
	if counterpartyID == nil {
		return resp
	}

	user, err := h.userStore.FindByID(ctx, *counterpartyID)
	if err != nil {
		// If we can't find the counterparty, just omit the phone.
		return resp
	}
	resp.CounterpartyPhone = user.Phone
	return resp
}

// isDomainError checks if an error is a DomainError by attempting to cast it.
func isDomainError(err error) bool {
	if err == nil {
		return false
	}
	// Check if it's already a DomainError (value or pointer).
	if _, ok := err.(types.DomainError); ok {
		return true
	}
	_, ok := err.(*types.DomainError)
	return ok
}

// writeJSONError writes a DomainError as a JSON error response.
func writeJSONError(w http.ResponseWriter, err types.DomainError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.HTTPStatus)
	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    err.Code,
			"message": err.Message,
		},
	}); encodeErr != nil {
		slog.Error("failed to encode error response", "error", encodeErr)
	}
}

// writeJSONResponse writes a success JSON response.
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(data); encodeErr != nil {
		slog.Error("failed to encode response", "error", encodeErr)
	}
}
