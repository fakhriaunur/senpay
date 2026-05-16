package senpai

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"senpay/internal/auth"
	"senpay/internal/i18n"
	"senpay/internal/senpai/llm"
	"senpay/internal/types"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler implements HTTP handlers for senpai analytics and budget endpoints.
type Handler struct {
	queryStore  *QueryStore
	budgetStore *BudgetStore
	fullEnabled bool
	nudgeLLM    llm.NudgeLLM // optional LLM adapter for nudge tips
}

// NewHandler creates a new senpai Handler.
// If nudgeLLM is nil, LLM-powered nudge tips are disabled.
func NewHandler(pool *pgxpool.Pool, fullEnabled bool, nudgeLLM llm.NudgeLLM) *Handler {
	return &Handler{
		queryStore:  NewQueryStore(pool),
		budgetStore: NewBudgetStore(pool),
		fullEnabled: fullEnabled,
		nudgeLLM:    nudgeLLM,
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
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	summary, err := h.queryStore.GetMonthlySummary(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get spending summary", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
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
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	trend, err := h.queryStore.GetTrend(r.Context(), userID, 6)
	if err != nil {
		slog.Error("failed to get spending trend", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
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
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	var req CreateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, types.NewMissingFieldError("body"))
		return
	}

	if req.Category == "" {
		writeJSONError(w, r, types.NewMissingFieldError("category"))
		return
	}

	if req.LimitSen <= 0 {
		writeJSONError(w, r, types.ErrInvalidAmount)
		return
	}

	budget, err := h.budgetStore.Create(r.Context(), userID, req)
	if err != nil {
		slog.Error("failed to create budget", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
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
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	budgets, err := h.budgetStore.ListCurrentWithSpending(r.Context(), userID)
	if err != nil {
		slog.Error("failed to list budgets", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
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
// When enabled, queries transaction data and returns personalised financial nudges
// from the nudge rules engine (EvaluateAll).
//
// Response format:
//
//	{"data": [{"type":"velocity","severity":"warning","message":"...","action":"...","dismissible":true}, ...]}
func (h *Handler) Nudge(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, r, types.ErrUnauthorized)
		return
	}

	if !h.fullEnabled {
		writeJSONError(w, r, types.ErrFeatureNotAvailable)
		return
	}

	// Gather data for the nudge engine.
	recentSpending, err := h.queryStore.GetRecentSpending(r.Context(), userID, 24)
	if err != nil {
		slog.Error("failed to get recent spending", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
		return
	}

	dailyHistory, err := h.queryStore.GetDailyTotals(r.Context(), userID, 7)
	if err != nil {
		slog.Error("failed to get daily history", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
		return
	}

	// Daily averages for trend detection: use daily totals as averages.
	// Convert int64 daily totals to float64 for the trend detection function.
	var dailyAvg []float64
	for _, d := range dailyHistory {
		dailyAvg = append(dailyAvg, float64(d))
	}

	categoryTx, err := h.queryStore.GetCategoryTransactions(r.Context(), userID)
	if err != nil {
		slog.Error("failed to get category transactions", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
		return
	}

	// Get budgets for exhaustion projection.
	budgets, err := h.budgetStore.ListCurrentWithSpending(r.Context(), userID)
	if err != nil {
		slog.Error("failed to list budgets for nudge", "user_id", userID, "error", err)
		writeJSONError(w, r, types.ErrInternal)
		return
	}

	// Aggregate budget totals for exhaustion projection.
	var totalBudgetCap, totalSpent int64
	for _, b := range budgets {
		totalBudgetCap += b.LimitSen
		totalSpent += b.SpentSen
	}

	// Compute days remaining in the current month.
	now := time.Now().UTC()
	firstOfNextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	daysRemaining := int(firstOfNextMonth.Sub(now).Hours() / 24)

	nudges := EvaluateAll(
		recentSpending,
		dailyHistory,
		dailyAvg,
		categoryTx,
		totalBudgetCap,
		totalSpent,
		daysRemaining,
	)

	// Build response with optional LLM tip.
	resp := map[string]interface{}{
		"data": nudges,
	}

	if h.nudgeLLM != nil && len(nudges) > 0 {
		prompt := buildLLMPrompt(nudges)
		tip, tipErr := h.nudgeLLM.Generate(r.Context(), prompt)
		if tipErr != nil {
			// LLM failure is non-fatal — omit llm_tip, return rule-based nudges normally.
			slog.Warn("llm nudge tip generation failed", "error", tipErr)
		} else if tip != "" {
			resp["llm_tip"] = tip
		}
	}

	writeJSONResponse(w, http.StatusOK, resp)
}

// buildLLMPrompt constructs a prompt for the LLM based on the user's nudges.
// The prompt asks for concise Indonesian financial advice.
func buildLLMPrompt(nudges []Nudge) string {
	var sb []byte
	sb = append(sb, "Berikut adalah kondisi keuangan pengguna saat ini:\n"...)
	for _, n := range nudges {
		sb = append(sb, "- "...)
		sb = append(sb, n.Message...)
		sb = append(sb, '\n')
	}
	sb = append(sb, "\nBerdasarkan kondisi di atas, berikan 1-2 kalimat saran keuangan singkat dalam Bahasa Indonesia. Jangan ulangi kondisi yang sudah disebutkan. Langsung berikan sarannya."...)
	return string(sb)
}

// ────────────────────────────────────────────────────────────────
// Response Helpers
// ────────────────────────────────────────────────────────────────

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
