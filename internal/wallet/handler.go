// Package wallet provides HTTP handlers for wallet-related endpoints.
//
// FCIS structure:
//   - handler.go: Shell — parses HTTP request, calls service/store, formats JSON response.
//
// The balance is projected from tx_log committed entries, not from a mutable balance column.
// This ensures the balance is always an accurate reflection of the transaction history.
package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"senpay/internal/auth"
	"senpay/internal/i18n"
	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler implements HTTP handlers for wallet-related endpoints.
type Handler struct {
	pool *pgxpool.Pool
}

// NewHandler creates a new wallet Handler.
func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

// BalanceResponse represents the JSON response for the balance endpoint.
type BalanceResponse struct {
	BalanceSen int64 `json:"balance_sen"`
	Version    int   `json:"version"`
}

// Balance handles GET /v1/wallet/balance.
//
// Computes the user's balance by projecting from tx_log committed entries:
// balance = SUM(committed credits where user is receiver) -
//
//	SUM(committed debits where user is sender)
//
// Only COMMITTED entries are included; PENDING, FAILED, and COMPENSATED entries
// are excluded from the projection.
//
// Requires authentication via auth middleware (JWT in Authorization header).
// Returns 200 with balance_sen and version on success.
// Returns 401 if not authenticated.
func (h *Handler) Balance(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	balance, version, err := h.projectBalance(r.Context(), userID)
	if err != nil {
		slog.Error("failed to project balance", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data": BalanceResponse{
			BalanceSen: balance,
			Version:    version,
		},
	})
}

// projectBalance computes the projected balance from tx_log committed entries.
// Returns the balance in sen, the version (committed tx count), and any error.
func (h *Handler) projectBalance(ctx context.Context, userID uuid.UUID) (int64, int, error) {
	// Query: sum credits (user is receiver) minus debits (user is sender)
	// for all COMMITTED entries.
	const query = `
		SELECT
			COALESCE(SUM(CASE WHEN receiver_id = $1 THEN amount_sen ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN sender_id = $1 THEN amount_sen ELSE 0 END), 0) AS balance,
			COUNT(*) AS version
		FROM tx_log
		WHERE (sender_id = $1 OR receiver_id = $1) AND status = 'committed'
	`

	var balance int64
	var version int
	err := h.pool.QueryRow(ctx, query, userID).Scan(&balance, &version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	return balance, version, nil
}

// writeJSONError writes a DomainError as a JSON response,
// with the message dynamically resolved based on the Accept-Language
// in the request context.
// If r is nil, uses the default Indonesian message.
func writeJSONError(w http.ResponseWriter, r *http.Request, err types.DomainError) {
	lang := i18n.DefaultLang
	if r != nil {
		if l := types.GetAcceptLanguage(r.Context()); l != "" {
			lang = l
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.HTTPStatus)
	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    err.Code,
			"message": i18n.ResolveErrorMessage(err, lang),
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
