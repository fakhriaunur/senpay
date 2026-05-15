package senpai

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"senpay/internal/auth"
	"senpay/internal/types"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler implements HTTP handlers for senpai analytics and budget endpoints.
type Handler struct {
	queryStore  *QueryStore
	budgetStore *BudgetStore
	fullEnabled bool
}

// NewHandler creates a new senpai Handler.
func NewHandler(pool *pgxpool.Pool, fullEnabled bool) *Handler {
	return &Handler{
		queryStore:  NewQueryStore(pool),
		budgetStore: NewBudgetStore(pool),
		fullEnabled: fullEnabled,
	}
}

// ────────────────────────────────────────────────────────────────
// Spending Summary
// ────────────────────────────────────────────────────────────────

// Summary handles GET /v1/senpai/summary.
//
// Returns monthly spending aggregated by category with totals and percentages.
// Requires authentication.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	summary, err := h.queryStore.GetMonthlySummary(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get spending summary", "user_id", userID, "error", err)
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data": summary,
	})
}

// ────────────────────────────────────────────────────────────────
// Spending Trend
// ────────────────────────────────────────────────────────────────

// Trend handles GET /v1/senpai/trend.
//
// Returns 6-month spending trend as monthly totals.
// Requires authentication.
func (h *Handler) Trend(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	trend, err := h.queryStore.GetTrend(r.Context(), userID, 6)
	if err != nil {
		slog.Error("failed to get spending trend", "user_id", userID, "error", err)
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data": trend,
	})
}

// ────────────────────────────────────────────────────────────────
// Budget CRUD
// ────────────────────────────────────────────────────────────────

// CreateBudget handles POST /v1/senpai/budgets.
//
// Creates a budget for a category with a monthly limit in sen.
// Request body: {"category": "Makanan", "limit_sen": 2000000}
// Returns 201 with budget ID on success.
// Requires authentication.
func (h *Handler) CreateBudget(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	var req CreateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, types.NewMissingFieldError("body"))
		return
	}

	if req.Category == "" {
		writeJSONError(w, types.NewMissingFieldError("category"))
		return
	}

	if req.LimitSen <= 0 {
		writeJSONError(w, types.ErrInvalidAmount)
		return
	}

	budget, err := h.budgetStore.Create(r.Context(), userID, req)
	if err != nil {
		slog.Error("failed to create budget", "user_id", userID, "error", err)
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusCreated, map[string]interface{}{
		"data": budget,
	})
}

// ListBudgets handles GET /v1/senpai/budgets.
//
// Lists all active budgets with current spending vs limit and percentage used.
// Budget alerts are computed: when spending reaches 80% of limit, alert flag is true;
// at 100%, exceeded flag is set with a warning message.
// Requires authentication.
func (h *Handler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	budgets, err := h.budgetStore.ListCurrentWithSpending(r.Context(), userID)
	if err != nil {
		slog.Error("failed to list budgets", "user_id", userID, "error", err)
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data": budgets,
	})
}

// ────────────────────────────────────────────────────────────────
// Nudge Engine (Feature Flagged)
// ────────────────────────────────────────────────────────────────

// Nudge handles GET /v1/senpai/nudge.
//
// Requires authentication.
// When SENPAI_FULL_ENABLED is false (default), returns 501 Not Implemented.
// When enabled, this would return personalized financial nudges.
func (h *Handler) Nudge(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	if !h.fullEnabled {
		writeJSONError(w, types.ErrFeatureNotAvailable)
		return
	}

	// When full nudge engine is enabled, this would return nudges.
	// For now, return 501 regardless even if enabled (no implementation yet).
	writeJSONError(w, types.ErrFeatureNotAvailable)
}

// ────────────────────────────────────────────────────────────────
// Response Helpers
// ────────────────────────────────────────────────────────────────

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
