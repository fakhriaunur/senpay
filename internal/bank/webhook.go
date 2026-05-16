package bank

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"senpay/internal/i18n"
	"senpay/internal/types"
)

// ────────────────────────────────────────────────────────────────
// Webhook Handler
// ────────────────────────────────────────────────────────────────

// WebhookHandler processes incoming bank webhook callbacks.
//
// The mock bank sends a POST request to this handler when it simulates
// a successful VA payment. The handler parses the BankCallback payload,
// validates it, and processes the VA payment (commits the pending tx_log
// and credits the user's balance).
type WebhookHandler struct {
	svc *Service
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(svc *Service) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

// HandleWebhook processes a bank webhook callback (POST /bank/webhook).
//
// Expected JSON body:
//
//	{
//	  "va_number": "8999123456",
//	  "amount_sen": 10000000,
//	  "external_id": "ext-001",
//	  "status": "success",
//	  "reference_id": "BANK-CALLBACK-ext-001"
//	}
//
// Returns 200 OK on success.
// Returns 400 for validation errors.
// Returns 404 if VA not found.
// Returns 409 if VA already processed.
// Returns 500 for internal errors.
func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, r, types.ErrInternal)
		return
	}

	var callback BankCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		writeJSONError(w, r, types.ErrInternal)
		return
	}

	if callback.VANumber == "" {
		writeJSONError(w, r, types.NewMissingFieldError("va_number"))
		return
	}

	if domainErr := h.svc.ProcessWebhook(r.Context(), &callback); domainErr != nil {
		writeJSONError(w, r, *domainErr)
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]string{"status": "processed"})
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
