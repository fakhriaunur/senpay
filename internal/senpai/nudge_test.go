package senpai

import (
	"testing"
)

// ────────────────────────────────────────────────────────────────
// Velocity Rate Tests
// ────────────────────────────────────────────────────────────────

func TestVelocityRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		recentSpending []int64
		dailyHistory   []int64
		wantNudge      bool
		wantType       NudgeType
		wantSeverity   NudgeSeverity
	}{
		{
			name:           "high_velocity_above_1.5x",
			recentSpending: []int64{500000, 300000, 400000}, // 1.2M total
			dailyHistory:   []int64{200000, 150000, 180000, 220000, 170000, 190000, 210000}, // avg ~188571
			wantNudge:      true,
			wantType:       NudgeTypeVelocity,
			wantSeverity:   NudgeSeverityWarning,
		},
		{
			name:           "normal_velocity_below_threshold",
			recentSpending: []int64{100000, 120000, 130000}, // 350K total
			dailyHistory:   []int64{200000, 250000, 300000, 220000, 280000, 260000, 240000}, // avg ~250K
			wantNudge:      false,
		},
		{
			name:           "zero_daily_history_returns_nil",
			recentSpending: []int64{100000},
			dailyHistory:   []int64{},
			wantNudge:      false,
		},
		{
			name:           "zero_recent_spending",
			recentSpending: []int64{},
			dailyHistory:   []int64{200000, 150000, 180000},
			wantNudge:      false,
		},
		{
			name:           "exactly_at_1.5x_threshold",
			recentSpending: []int64{300000}, // 300K
			dailyHistory:   []int64{200000, 200000, 200000}, // avg = 200K, ratio = 1.5, not strictly above
			wantNudge:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nudge := VelocityRate(tt.recentSpending, tt.dailyHistory)

			if tt.wantNudge && nudge == nil {
				t.Fatal("expected nudge, got nil")
			}
			if !tt.wantNudge && nudge != nil {
				t.Fatalf("expected no nudge, got %+v", *nudge)
			}
			if tt.wantNudge {
				if nudge.Type != tt.wantType {
					t.Errorf("Type = %q, want %q", nudge.Type, tt.wantType)
				}
				if nudge.Severity != tt.wantSeverity {
					t.Errorf("Severity = %q, want %q", nudge.Severity, tt.wantSeverity)
				}
				if nudge.Message == "" {
					t.Error("expected non-empty message")
				}
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────
// Trend Detection Tests
// ────────────────────────────────────────────────────────────────

func TestTrendDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		dailyAvg     []float64
		wantNudge    bool
		wantType     NudgeType
		wantSeverity NudgeSeverity
	}{
		{
			name:         "upward_trend_three_consecutive_increases",
			dailyAvg:     []float64{100000, 120000, 150000, 190000, 240000}, // 3-period MA: 123333, 153333, 193333 → 2+ consecutive
			wantNudge:    true,
			wantType:     NudgeTypeTrend,
			wantSeverity: NudgeSeverityWarning,
		},
		{
			name:         "flat_trend_no_nudge",
			dailyAvg:     []float64{200000, 200000, 200000, 200000}, // 3-period MA: flat
			wantNudge:    false,
		},
		{
			name:         "downward_trend_no_nudge",
			dailyAvg:     []float64{300000, 250000, 200000, 150000},
			wantNudge:    false,
		},
		{
			name:         "not_enough_days_less_than_4",
			dailyAvg:     []float64{100000, 150000},
			wantNudge:    false,
		},
		{
			name:         "oscillating_no_consecutive_increase",
			dailyAvg:     []float64{100000, 300000, 100000, 300000},
			wantNudge:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nudge := TrendDetection(tt.dailyAvg)

			if tt.wantNudge && nudge == nil {
				t.Fatal("expected nudge, got nil")
			}
			if !tt.wantNudge && nudge != nil {
				t.Fatalf("expected no nudge, got %+v", *nudge)
			}
			if tt.wantNudge {
				if nudge.Type != tt.wantType {
					t.Errorf("Type = %q, want %q", nudge.Type, tt.wantType)
				}
				if nudge.Severity != tt.wantSeverity {
					t.Errorf("Severity = %q, want %q", nudge.Severity, tt.wantSeverity)
				}
				if nudge.Message == "" {
					t.Error("expected non-empty message")
				}
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────
// Anomaly Flagging Tests
// ────────────────────────────────────────────────────────────────

func TestAnomalyFlagging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		transactions []int64
		wantNudge    bool
		wantType     NudgeType
		wantSeverity NudgeSeverity
	}{
		{
			name:         "anomalous_transaction_above_2_stddev",
			transactions: []int64{1000, 1200, 1100, 900, 950, 5000, 1050, 980, 1100, 1150},
			// mean ≈ 1428 (with 5000), stddev ≈ 1170, mean+2*stddev ≈ 3768, 5000 > 3768
			wantNudge:    true,
			wantType:     NudgeTypeAnomaly,
			wantSeverity: NudgeSeverityWarning,
		},
		{
			name:         "all_normal_no_anomaly",
			transactions: []int64{1000, 1010, 1020, 990, 1010, 980, 1000, 990, 1010, 1000},
			wantNudge:    false,
		},
		{
			name:         "not_enough_transactions_less_than_2",
			transactions: []int64{1000},
			wantNudge:    false,
		},
		{
			name:         "all_equal_no_stddev",
			transactions: []int64{1000, 1000, 1000, 1000, 1000},
			wantNudge:    false, // stddev=0, so all transactions equal mean, none > mean + 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nudge := AnomalyFlagging(tt.transactions)

			if tt.wantNudge && nudge == nil {
				t.Fatal("expected nudge, got nil")
			}
			if !tt.wantNudge && nudge != nil {
				t.Fatalf("expected no nudge, got %+v", *nudge)
			}
			if tt.wantNudge {
				if nudge.Type != tt.wantType {
					t.Errorf("Type = %q, want %q", nudge.Type, tt.wantType)
				}
				if nudge.Severity != tt.wantSeverity {
					t.Errorf("Severity = %q, want %q", nudge.Severity, tt.wantSeverity)
				}
				if nudge.Message == "" {
					t.Error("expected non-empty message")
				}
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────
// Exhaustion Projection Tests
// ────────────────────────────────────────────────────────────────

func TestExhaustionProjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		budgetCap            int64
		spentSoFar           int64
		dailySpending        []int64
		daysRemainingInPeriod int
		wantNudge            bool
		wantType             NudgeType
		wantSeverity         NudgeSeverity
	}{
		{
			name:                 "less_than_7_days_to_exhaustion",
			budgetCap:            1000000,
			spentSoFar:           500000,
			dailySpending:        []int64{100000, 120000, 90000, 110000, 80000, 95000, 105000}, // avg ~100K
			daysRemainingInPeriod: 15,
			// remaining = 500000, burn rate = 100000, days = 5 < 7 → nudge
			wantNudge:    true,
			wantType:     NudgeTypeExhaustion,
			wantSeverity: NudgeSeverityCritical,
		},
		{
			name:                 "more_than_7_days_to_exhaustion",
			budgetCap:            2000000,
			spentSoFar:           500000,
			dailySpending:        []int64{50000, 60000, 45000, 55000, 40000, 48000, 52000}, // avg ~50K
			daysRemainingInPeriod: 30,
			// remaining = 1500000, burn rate = 50000, days = 30 > 7 → no nudge
			wantNudge:    false,
		},
		{
			name:                 "already_exceeded_budget",
			budgetCap:            1000000,
			spentSoFar:           1200000,
			dailySpending:        []int64{50000, 60000, 45000},
			daysRemainingInPeriod: 10,
			wantNudge:    true,
			wantType:     NudgeTypeExhaustion,
			wantSeverity: NudgeSeverityCritical,
		},
		{
			name:                 "zero_burn_rate_no_spending",
			budgetCap:            1000000,
			spentSoFar:           500000,
			dailySpending:        []int64{},
			daysRemainingInPeriod: 15,
			wantNudge:    false, // can't project
		},
		{
			name:                 "burn_rate_zero_avg_but_not_spent",
			budgetCap:            1000000,
			spentSoFar:           500000,
			dailySpending:        []int64{0, 0, 0, 0, 0, 0, 0},
			daysRemainingInPeriod: 15,
			wantNudge:    false, // burn rate = 0, never exhausts
		},
		{
			name:                 "above_90_percent_usage_with_days_remaining",
			budgetCap:            1000000,
			spentSoFar:           950000,
			dailySpending:        []int64{10000, 12000, 9000, 11000, 8000},
			daysRemainingInPeriod: 5,
			// 95% used, 5 days remaining, >=2 days → nudge
			wantNudge:    true,
			wantType:     NudgeTypeExhaustion,
			wantSeverity: NudgeSeverityCritical,
		},
		{
			name:                 "above_90_percent_but_less_than_2_days",
			budgetCap:            1000000,
			spentSoFar:           950000,
			dailySpending:        []int64{1000, 1000, 1000}, // burn rate ~1000/day, 50 days to exhaust → > 7
			daysRemainingInPeriod: 1,
			// 95% used, 1 day remaining, <2 days → no nudge (per VAL-NUDGE-007)
			// Also linear extrapolation: 50 days > 7 → no nudge
			wantNudge:    false,
		},
		{
			name:                 "below_90_percent_any_days",
			budgetCap:            1000000,
			spentSoFar:           500000,
			dailySpending:        []int64{40000, 45000, 35000, 42000, 38000}, // avg ~40K, 12.5 days to exhaust > 7
			daysRemainingInPeriod: 5,
			wantNudge:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nudge := ExhaustionProjection(tt.budgetCap, tt.spentSoFar, tt.dailySpending, tt.daysRemainingInPeriod)

			if tt.wantNudge && nudge == nil {
				t.Fatal("expected nudge, got nil")
			}
			if !tt.wantNudge && nudge != nil {
				t.Fatalf("expected no nudge, got %+v", *nudge)
			}
			if tt.wantNudge {
				if nudge.Type != tt.wantType {
					t.Errorf("Type = %q, want %q", nudge.Type, tt.wantType)
				}
				if nudge.Severity != tt.wantSeverity {
					t.Errorf("Severity = %q, want %q", nudge.Severity, tt.wantSeverity)
				}
				if nudge.Message == "" {
					t.Error("expected non-empty message")
				}
			}
		})
	}
}

// ────────────────────────────────────────────────────────────────
// Edge Case Tests
// ────────────────────────────────────────────────────────────────

func TestVelocityRate_EdgeCases(t *testing.T) {
	t.Parallel()

	// Test with nil slices and zero-length.
	if n := VelocityRate(nil, []int64{100000, 200000}); n != nil {
		t.Error("expected nil for nil recent spending")
	}
	if n := VelocityRate([]int64{100000}, nil); n != nil {
		t.Error("expected nil for nil daily history")
	}
}

func TestTrendDetection_EdgeCases(t *testing.T) {
	t.Parallel()

	if n := TrendDetection(nil); n != nil {
		t.Error("expected nil for nil input")
	}
	// Single value, need at least 4 for 3-period MA with one comparison.
	if n := TrendDetection([]float64{100000}); n != nil {
		t.Error("expected nil for single value")
	}
}

func TestAnomalyFlagging_EdgeCases(t *testing.T) {
	t.Parallel()

	if n := AnomalyFlagging(nil); n != nil {
		t.Error("expected nil for nil transactions")
	}
	if n := AnomalyFlagging([]int64{}); n != nil {
		t.Error("expected nil for empty transactions")
	}
}

func TestExhaustionProjection_EdgeCases(t *testing.T) {
	t.Parallel()

	if n := ExhaustionProjection(1000000, 500000, nil, 10); n != nil {
		t.Error("expected nil for nil daily spending")
	}
	if n := ExhaustionProjection(0, 0, []int64{1000}, 10); n != nil {
		t.Error("expected nil for zero budget cap")
	}
}

// ────────────────────────────────────────────────────────────────
// EvaluateAll Integration Test
// ────────────────────────────────────────────────────────────────

func TestEvaluateAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		recentSpending        []int64
		dailyHistory          []int64
		dailyAvg              []float64
		categoryTransactions  []int64
		budgetCap             int64
		spentSoFar            int64
		daysRemainingInPeriod int
		wantCount             int
		wantTypes             []NudgeType
	}{
		{
			name:                  "multiple_nudges_triggered",
			recentSpending:        []int64{500000, 400000, 300000}, // 1.2M > 1.5x avg
			dailyHistory:          []int64{200000, 150000, 180000, 220000, 170000, 190000, 210000},
			dailyAvg:              []float64{100000, 130000, 170000, 200000, 240000}, // upward trend
			categoryTransactions:  []int64{1000, 1200, 1100, 900, 950, 5000, 1050},
			budgetCap:             1000000,
			spentSoFar:            950000,
			daysRemainingInPeriod: 5,
			// velocity + trend + anomaly + exhaustion = 4
			wantCount: 4,
			wantTypes: []NudgeType{NudgeTypeVelocity, NudgeTypeTrend, NudgeTypeAnomaly, NudgeTypeExhaustion},
		},
		{
			name:                  "no_nudges_for_normal_spending",
			recentSpending:        []int64{100000, 110000},
			dailyHistory:          []int64{200000, 150000, 180000, 220000, 170000, 190000, 210000},
			dailyAvg:              []float64{200000, 190000, 195000, 180000}, // flat/down
			categoryTransactions:  []int64{1000, 1100, 1050, 950, 1020},
			budgetCap:             2000000,
			spentSoFar:            500000,
			daysRemainingInPeriod: 20,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nudges := EvaluateAll(tt.recentSpending, tt.dailyHistory, tt.dailyAvg, tt.categoryTransactions, tt.budgetCap, tt.spentSoFar, tt.daysRemainingInPeriod)

			if len(nudges) != tt.wantCount {
				t.Errorf("got %d nudges, want %d", len(nudges), tt.wantCount)
			}

			// Verify all expected types are present
			typeSet := make(map[NudgeType]bool)
			for _, n := range nudges {
				typeSet[n.Type] = true
				if n.Message == "" {
					t.Errorf("nudge %q has empty message", n.Type)
				}
				if n.Severity == "" {
					t.Errorf("nudge %q has empty severity", n.Type)
				}
			}
			for _, wt := range tt.wantTypes {
				if !typeSet[wt] {
					t.Errorf("expected nudge type %q not found", wt)
				}
			}
		})
	}
}


