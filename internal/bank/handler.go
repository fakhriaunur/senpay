package bank

import (
	"encoding/json"
	"net/http"

	"senpay/internal/auth"
	"senpay/internal/types"
)

// ────────────────────────────────────────────────────────────────
// Top-up & Withdraw HTTP Handler
// ────────────────────────────────────────────────────────────────

// Handler implements HTTP handlers for top-up, withdraw, and bank-related endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a new bank Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Topup handles POST /v1/topup.
//
// The user is identified from the JWT auth context (auth middleware).
// Request body:
//
//	{"idempotency_key":"...","amount_sen":10000000}
//
// Responses:
//   - 200 OK on success (or duplicate key)
//   - 202 Accepted for in-flight request
//   - 400 Bad Request for validation errors
//   - 401 Unauthorized for invalid/missing JWT
//   - 504 Gateway Timeout for bank timeout
func (h *Handler) Topup(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	var req TopupHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, types.NewMissingFieldError("body"))
		return
	}

	if req.IdempotencyKey == "" {
		writeJSONError(w, r, types.NewMissingFieldError("idempotency_key"))
		return
	}

	if req.AmountSen <= 0 {
		writeJSONError(w, r, types.ErrInvalidAmount)
		return
	}

	result, domainErr := h.svc.Topup(r.Context(), userID, req)
	if domainErr != nil {
		// Special case: in-flight returns 202.
		if domainErr.Code == types.ErrCodeRequestInFlight {
			writeJSONError(w, r, *domainErr)
			return
		}
		writeJSONError(w, r, *domainErr)
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data": result,
	})
}

// Withdraw handles POST /v1/withdraw.
//
// The user is identified from the JWT auth context (auth middleware).
// Request body:
//
//	{"idempotency_key":"...","amount_sen":5000000,"bank_account":"1234567890"}
//
// Responses:
//   - 200 OK on success (or duplicate key)
//   - 202 Accepted for in-flight request
//   - 400 Bad Request for validation errors (invalid amount, missing fields,
//     insufficient balance, exceeds BI limit)
//   - 401 Unauthorized for invalid/missing JWT
//   - 502 Bad Gateway for bank rejection
//   - 504 Gateway Timeout for bank timeout
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	var req WithdrawHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, types.NewMissingFieldError("body"))
		return
	}

	if req.IdempotencyKey == "" {
		writeJSONError(w, r, types.NewMissingFieldError("idempotency_key"))
		return
	}

	if req.AmountSen <= 0 {
		writeJSONError(w, r, types.ErrInvalidAmount)
		return
	}

	if req.BankAccount == "" {
		writeJSONError(w, r, types.NewMissingFieldError("bank_account"))
		return
	}

	result, domainErr := h.svc.Withdraw(r.Context(), userID, req)
	if domainErr != nil {
		// Special case: in-flight returns 202.
		if domainErr.Code == types.ErrCodeRequestInFlight {
			writeJSONError(w, r, *domainErr)
			return
		}
		writeJSONError(w, r, *domainErr)
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data": result,
	})
}
