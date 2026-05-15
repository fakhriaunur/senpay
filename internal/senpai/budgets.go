// Package senpai provides spending analytics, budget management, and financial insights.
//
// FCIS structure:
//   - budgets.go: Shell — budget CRUD operations against PostgreSQL
//   - handler.go: Shell — HTTP handlers for senpai endpoints
//
// This package implements the "Senpai Minimal" feature set: spending summary,
// category breakdown, spending trends, budget creation with threshold alerts,
// and a feature-flagged nudge engine placeholder.
package senpai

import (
	"context"
	"fmt"
	"time"

	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ────────────────────────────────────────────────────────────────
// Budget Types
// ────────────────────────────────────────────────────────────────

// Budget represents a monthly spending budget for a category.
type Budget struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Category  string    `json:"category"`
	LimitSen  int64     `json:"limit_sen"`
	SpentSen  int64     `json:"spent_sen"`
	Month     int       `json:"month"`
	Year      int       `json:"year"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BudgetWithAlert extends Budget with computed alert information.
type BudgetWithAlert struct {
	Budget
	UsedPercent   float64 `json:"used_percent"`
	Alert         bool    `json:"alert"`
	Exceeded      bool    `json:"exceeded"`
	WarningMsg    string  `json:"warning_message,omitempty"`
}

// CreateBudgetRequest represents a request to create a new budget.
type CreateBudgetRequest struct {
	Category string `json:"category"`
	LimitSen int64  `json:"limit_sen"`
}

// ────────────────────────────────────────────────────────────────
// Budget Store
// ────────────────────────────────────────────────────────────────

// BudgetStore handles budget CRUD operations against PostgreSQL.
type BudgetStore struct {
	pool *pgxpool.Pool
}

// NewBudgetStore creates a new BudgetStore.
func NewBudgetStore(pool *pgxpool.Pool) *BudgetStore {
	return &BudgetStore{pool: pool}
}

// Create inserts a new budget for the current month/year.
// If a budget for the same user+category+month already exists, it returns the existing one.
func (s *BudgetStore) Create(ctx context.Context, userID uuid.UUID, req CreateBudgetRequest) (*Budget, error) {
	now := time.Now().UTC()
	month := int(now.Month())
	year := now.Year()

	budget := &Budget{
		ID:        uuid.Must(uuid.NewV7()),
		UserID:    userID,
		Category:  req.Category,
		LimitSen:  req.LimitSen,
		SpentSen:  0,
		Month:     month,
		Year:      year,
		CreatedAt: now,
		UpdatedAt: now,
	}

	const query = `
		INSERT INTO budgets (id, user_id, category, limit_sen, spent_sen, month, year, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, category, month, year)
		DO UPDATE SET limit_sen = EXCLUDED.limit_sen, updated_at = NOW()
		RETURNING id, user_id, category, limit_sen, spent_sen, month, year, created_at, updated_at
	`

	err := s.pool.QueryRow(ctx, query,
		budget.ID, budget.UserID, budget.Category, budget.LimitSen,
		budget.SpentSen, budget.Month, budget.Year, budget.CreatedAt, budget.UpdatedAt,
	).Scan(
		&budget.ID, &budget.UserID, &budget.Category, &budget.LimitSen,
		&budget.SpentSen, &budget.Month, &budget.Year, &budget.CreatedAt, &budget.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert budget: %w", err)
	}

	return budget, nil
}

// List returns all budgets for a user in the current month, with current spending.
func (s *BudgetStore) List(ctx context.Context, userID uuid.UUID) ([]BudgetWithAlert, error) {
	now := time.Now().UTC()
	currentMonth := int(now.Month())
	currentYear := now.Year()

	// Query budgets for current month with current spending from tx_log.
	const query = `
		SELECT
			b.id, b.user_id, b.category, b.limit_sen,
			COALESCE(SUM(t.amount_sen), 0) AS spent_sen,
			b.month, b.year, b.created_at, b.updated_at
		FROM budgets b
		LEFT JOIN tx_log t ON t.sender_id = b.user_id
			AND t.category = b.category
			AND t.status = 'committed'
			AND EXTRACT(MONTH FROM t.created_at) = $2
			AND EXTRACT(YEAR FROM t.created_at) = $3
		WHERE b.user_id = $1 AND b.month = $2 AND b.year = $3
		GROUP BY b.id, b.user_id, b.category, b.limit_sen, b.month, b.year, b.created_at, b.updated_at
	`

	rows, err := s.pool.Query(ctx, query, userID, currentMonth, currentYear)
	if err != nil {
		return nil, fmt.Errorf("query budgets: %w", err)
	}
	defer rows.Close()

	var budgets []BudgetWithAlert
	for rows.Next() {
		var b BudgetWithAlert
		err := rows.Scan(
			&b.ID, &b.UserID, &b.Category, &b.LimitSen,
			&b.SpentSen, &b.Month, &b.Year, &b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan budget: %w", err)
		}
		b.computeAlert()
		budgets = append(budgets, b)
	}

	if budgets == nil {
		budgets = []BudgetWithAlert{}
	}

	return budgets, nil
}

// ListCurrentWithSpending returns budgets with computed alert info for the current month.
// Alias for List.
func (s *BudgetStore) ListCurrentWithSpending(ctx context.Context, userID uuid.UUID) ([]BudgetWithAlert, error) {
	return s.List(ctx, userID)
}

// computeAlert calculates alert flags based on spending vs limit.
func (b *BudgetWithAlert) computeAlert() {
	if b.LimitSen <= 0 {
		return
	}

	pct := float64(b.SpentSen) / float64(b.LimitSen) * 100.0
	b.UsedPercent = pct

	if b.SpentSen >= b.LimitSen {
		b.Exceeded = true
		b.WarningMsg = fmt.Sprintf("Anggaran %s sudah habis (%.0f%%)", b.Category, pct)
		b.Alert = true
	} else if pct >= 80.0 {
		b.Alert = true
		b.WarningMsg = fmt.Sprintf("Anggaran %s tersisa %.0f%% lagi", b.Category, 100-pct)
	}
}

// ────────────────────────────────────────────────────────────────
// Spending Summary Types
// ────────────────────────────────────────────────────────────────

// CategorySpending represents spending in a single category.
type CategorySpending struct {
	Category   string  `json:"category"`
	TotalSen   int64   `json:"total_sen"`
	Percentage float64 `json:"percentage"`
	TxCount    int     `json:"tx_count"`
}

// SpendingSummary is the response for the summary endpoint.
type SpendingSummary struct {
	Month          string             `json:"month"`
	Year           int                `json:"year"`
	TotalSpent     int64              `json:"total_spent"`
	Categories     []CategorySpending `json:"categories"`
	TxCount        int                `json:"tx_count"`
}

// TrendEntry represents one month in the spending trend.
type TrendEntry struct {
	Month     string `json:"month"`
	Year      int    `json:"year"`
	TotalSen  int64  `json:"total_sen"`
	TxCount   int    `json:"tx_count"`
}

// ────────────────────────────────────────────────────────────────
// Spending Queries
// ────────────────────────────────────────────────────────────────

// QueryStore handles analytics queries.
type QueryStore struct {
	pool *pgxpool.Pool
}

// NewQueryStore creates a new QueryStore.
func NewQueryStore(pool *pgxpool.Pool) *QueryStore {
	return &QueryStore{pool: pool}
}

// GetMonthlySummary returns spending summary for the current month grouped by category.
// Only COMMITTED outgoing transactions (sender_id = user) are counted as spending.
// Categories with no transactions are omitted.
func (qs *QueryStore) GetMonthlySummary(ctx context.Context, userID uuid.UUID) (*SpendingSummary, error) {
	now := time.Now().UTC()
	month := int(now.Month())
	year := now.Year()

	return qs.GetSummaryForMonth(ctx, userID, month, year)
}

// GetSummaryForMonth returns spending summary for a specific month.
func (qs *QueryStore) GetSummaryForMonth(ctx context.Context, userID uuid.UUID, month, year int) (*SpendingSummary, error) {
	// Query: group committed debits by category for this user and month.
	const query = `
		SELECT
			COALESCE(NULLIF(category, ''), '` + types.CategoryDefault + `') AS cat,
			SUM(amount_sen) AS total,
			COUNT(*) AS cnt
		FROM tx_log
		WHERE sender_id = $1
			AND status = 'committed'
			AND tx_type = 'transfer'
			AND EXTRACT(MONTH FROM created_at) = $2
			AND EXTRACT(YEAR FROM created_at) = $3
		GROUP BY cat
		ORDER BY total DESC
	`

	rows, err := qs.pool.Query(ctx, query, userID, month, year)
	if err != nil {
		return nil, fmt.Errorf("query summary: %w", err)
	}
	defer rows.Close()

	var categories []CategorySpending
	var totalSpent int64
	var totalTx int

	for rows.Next() {
		var cs CategorySpending
		if err := rows.Scan(&cs.Category, &cs.TotalSen, &cs.TxCount); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		totalSpent += cs.TotalSen
		totalTx += cs.TxCount
		categories = append(categories, cs)
	}

	if categories == nil {
		categories = []CategorySpending{}
	}

	// Calculate percentages.
	for i := range categories {
		if totalSpent > 0 {
			categories[i].Percentage = float64(categories[i].TotalSen) / float64(totalSpent) * 100.0
		}
	}

	monthName := time.Month(month).String()

	return &SpendingSummary{
		Month:      monthName,
		Year:       year,
		TotalSpent: totalSpent,
		Categories: categories,
		TxCount:    totalTx,
	}, nil
}

// GetTrend returns spending trend for the last N months.
func (qs *QueryStore) GetTrend(ctx context.Context, userID uuid.UUID, months int) ([]TrendEntry, error) {
	now := time.Now().UTC()

	const query = `
		SELECT
			EXTRACT(MONTH FROM created_at)::int AS m,
			EXTRACT(YEAR FROM created_at)::int AS y,
			SUM(amount_sen) AS total,
			COUNT(*) AS cnt
		FROM tx_log
		WHERE sender_id = $1
			AND status = 'committed'
			AND tx_type = 'transfer'
			AND created_at >= $2
		GROUP BY y, m
		ORDER BY y DESC, m DESC
	`

	startDate := now.AddDate(0, -(months - 1), 0)
	startOfStart := time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, time.UTC)

	rows, err := qs.pool.Query(ctx, query, userID, startOfStart)
	if err != nil {
		return nil, fmt.Errorf("query trend: %w", err)
	}
	defer rows.Close()

	trendMap := make(map[string]*TrendEntry)

	for rows.Next() {
		var m, y int
		var total int64
		var cnt int
		if err := rows.Scan(&m, &y, &total, &cnt); err != nil {
			return nil, fmt.Errorf("scan trend: %w", err)
		}
		key := fmt.Sprintf("%d-%02d", y, m)
		trendMap[key] = &TrendEntry{
			Month:    time.Month(m).String(),
			Year:     y,
			TotalSen: total,
			TxCount:  cnt,
		}
	}

	// Fill in missing months with zero entries.
	var trend []TrendEntry
	for i := months - 1; i >= 0; i-- {
		d := now.AddDate(0, -i, 0)
		m, y := int(d.Month()), d.Year()
		key := fmt.Sprintf("%d-%02d", y, m)
		if entry, ok := trendMap[key]; ok {
			trend = append(trend, *entry)
		} else {
			trend = append(trend, TrendEntry{
				Month:   time.Month(m).String(),
				Year:    y,
				TotalSen: 0,
				TxCount: 0,
			})
		}
	}

	return trend, nil
}
