package transfer

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"senpay/internal/auth"
	"senpay/internal/i18n"
	"senpay/internal/types"
)

// Handler implements the HTTP handler for POST /v1/transfer.
type Handler struct {
	svc *Service
}

// NewHandler creates a new transfer Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Transfer handles POST /v1/transfer.
//
// The sender is identified from the JWT auth context (auth middleware).
// Request body:
//
//	{"idempotency_key":"...","to_phone":"...","amount_sen":500000}
//
// Responses: 201 Created on success, 200 for duplicate key, 202 for in-flight,
// 400 for validation/insufficient balance, 404 for receiver not found,
// 409 for serialization conflict, 500 for internal error.
func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	senderID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	var req TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, types.NewMissingFieldError("body"))
		return
	}

	if req.IdempotencyKey == "" {
		writeJSONError(w, r, types.NewMissingFieldError("idempotency_key"))
		return
	}

	if req.ToPhone == "" {
		writeJSONError(w, r, types.NewMissingFieldError("to_phone"))
		return
	}

	if req.AmountSen <= 0 {
		writeJSONError(w, r, types.ErrInvalidAmount)
		return
	}

	result, domainErr := h.svc.Transfer(r.Context(), senderID, req)
	if domainErr != nil {
		writeJSONError(w, r, *domainErr)
		return
	}

	statusCode := http.StatusCreated
	if result.Cached {
		statusCode = http.StatusOK
	}

	writeJSONResponse(w, statusCode, map[string]interface{}{
		"data": result,
	})
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
