// Package senpai provides spending analytics, budget management, and financial insights.
//
// FCIS structure:
//   - budgets.go: Shell — budget CRUD operations against PostgreSQL
//   - handler.go: Shell — HTTP handlers for senpai endpoints
//   - nudge.go: Core — pure nudge engine functions (no I/O, deterministic)
//
// All nudge functions are pure: they accept data as parameters and return Nudge structs.
// No I/O, no side effects, no external dependencies beyond stdlib + internal/types.
package senpai

import (
	"math"
)

// ────────────────────────────────────────────────────────────────
// Nudge Types
// ────────────────────────────────────────────────────────────────

// NudgeType categorises the kind of financial nudge.
type NudgeType string

const (
	// NudgeTypeVelocity flags above-average spending velocity.
	NudgeTypeVelocity NudgeType = "velocity"
	// NudgeTypeTrend flags upward spending trend detected.
	NudgeTypeTrend NudgeType = "trend"
	// NudgeTypeAnomaly flags anomalous transaction amounts.
	NudgeTypeAnomaly NudgeType = "anomaly"
	// NudgeTypeExhaustion flags projected budget exhaustion.
	NudgeTypeExhaustion NudgeType = "exhaustion"
)

// NudgeSeverity indicates the urgency level of a nudge.
type NudgeSeverity string

const (
	// NudgeSeverityInfo is for informational nudges.
	NudgeSeverityInfo NudgeSeverity = "info"
	// NudgeSeverityWarning is for cautionary nudges (e.g., velocity, trend, anomaly).
	NudgeSeverityWarning NudgeSeverity = "warning"
	// NudgeSeverityCritical is for urgent nudges (e.g., budget exhaustion).
	NudgeSeverityCritical NudgeSeverity = "critical"
)

// Nudge represents a single financial insight or alert.
// All fields are exported for JSON serialisation.
type Nudge struct {
	Type        NudgeType     `json:"type"`
	Severity    NudgeSeverity `json:"severity"`
	Message     string        `json:"message"`
	Action      string        `json:"action,omitempty"`
	Dismissible bool          `json:"dismissible"`
}

// ────────────────────────────────────────────────────────────────
// Nudge Sub-types for Exhaustion Projection
// ────────────────────────────────────────────────────────────────

// dailySpendingStats holds summary statistics for a set of daily spending values.
type dailySpendingStats struct {
	mean     float64
	stddev   float64
}

// computeStats computes mean and sample standard deviation for a slice of int64 values.
// Returns zero stats if fewer than 2 values.
func computeStats(vals []int64) dailySpendingStats {
	if len(vals) < 2 {
		return dailySpendingStats{}
	}
	var sum int64
	for _, v := range vals {
		sum += v
	}
	mean := float64(sum) / float64(len(vals))

	var sumSq float64
	for _, v := range vals {
		d := float64(v) - mean
		sumSq += d * d
	}
	stddev := math.Sqrt(sumSq / float64(len(vals)-1)) // sample stddev

	return dailySpendingStats{mean: mean, stddev: stddev}
}

// computeAverage computes the arithmetic mean of an int64 slice.
func computeAverage(vals []int64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum int64
	for _, v := range vals {
		sum += v
	}
	return float64(sum) / float64(len(vals))
}

// computeSum computes the total of an int64 slice.
func computeSum(vals []int64) int64 {
	var sum int64
	for _, v := range vals {
		sum += v
	}
	return sum
}

// ────────────────────────────────────────────────────────────────
// Velocity Rate
// ────────────────────────────────────────────────────────────────

// VelocityRate computes the current spending velocity ratio (recent spending / 7-day average).
// Returns a velocity nudge (severity: warning) if the ratio exceeds 1.5x.
//
// Parameters:
//   - recentSpending: transaction amounts in the current evaluation window (e.g., last 24h).
//     Summed to get current spending rate.
//   - dailyHistory: daily spending totals for the last N days used as baseline.
//     Averaged to compute the 7-day moving average baseline.
//
// Pure function: no I/O, deterministic for same inputs.
func VelocityRate(recentSpending, dailyHistory []int64) *Nudge {
	if len(recentSpending) == 0 || len(dailyHistory) == 0 {
		return nil
	}

	currentRate := float64(computeSum(recentSpending))
	baselineAvg := computeAverage(dailyHistory)

	if baselineAvg <= 0 {
		return nil
	}

	ratio := currentRate / baselineAvg

	// Threshold: > 1.5x of the 7-day moving average.
	if ratio > 1.5 {
		return &Nudge{
			Type:        NudgeTypeVelocity,
			Severity:    NudgeSeverityWarning,
			Message:     "Kecepatan pengeluaran Anda tinggi (" + formatRatio(ratio) + "x dari rata-rata mingguan). Pertimbangkan untuk menahan pengeluaran.",
			Action:      "Lihat Ringkasan",
			Dismissible: true,
		}
	}

	return nil
}

// ────────────────────────────────────────────────────────────────
// Trend Detection
// ────────────────────────────────────────────────────────────────

// TrendDetection computes a 3-period moving average of daily spending averages
// and flags an upward trend if the moving average shows consecutive increases.
//
// Parameters:
//   - dailyAvg: daily spending averages (at least 4 values needed to detect a trend).
//
// Pure function: no I/O, deterministic for same inputs.
func TrendDetection(dailyAvg []float64) *Nudge {
	if len(dailyAvg) < 4 {
		return nil
	}

	// Compute 3-period moving averages.
	var ma []float64
	for i := 0; i <= len(dailyAvg)-3; i++ {
		avg := (dailyAvg[i] + dailyAvg[i+1] + dailyAvg[i+2]) / 3.0
		ma = append(ma, avg)
	}

	// Check for consecutive increases in the moving average.
	// Need at least 2 MA values to compare.
	if len(ma) < 2 {
		return nil
	}

	// Count consecutive increases.
	consecutive := 0
	for i := 1; i < len(ma); i++ {
		if ma[i] > ma[i-1] {
			consecutive++
		} else {
			consecutive = 0
		}
	}

	// Flag if 3 or more consecutive increases (or just at least 2 consecutive increases).
	// Since each MA is already smoothed over 3 periods, even 2 consecutive increases
	// indicate a meaningful upward trend.
	if consecutive >= 2 {
		return &Nudge{
			Type:        NudgeTypeTrend,
			Severity:    NudgeSeverityWarning,
			Message:     "Tren pengeluaran Anda meningkat dalam beberapa hari terakhir. Pantau anggaran Anda.",
			Action:      "Lihat Tren",
			Dismissible: true,
		}
	}

	return nil
}

// ────────────────────────────────────────────────────────────────
// Anomaly Flagging
// ────────────────────────────────────────────────────────────────

// AnomalyFlagging flags transactions whose amount exceeds mean + 2*stddev
// of the category spending distribution.
//
// Parameters:
//   - transactions: slice of transaction amounts (in sen) for a given category.
//     Requires at least 2 values to compute meaningful statistics.
//
// Pure function: no I/O, deterministic for same inputs.
func AnomalyFlagging(transactions []int64) *Nudge {
	if len(transactions) < 2 {
		return nil
	}

	stats := computeStats(transactions)
	if stats.stddev <= 0 {
		return nil
	}

	threshold := stats.mean + 2*stats.stddev

	// Check if any transaction exceeds the threshold.
	for _, t := range transactions {
		if float64(t) > threshold {
			return &Nudge{
				Type:        NudgeTypeAnomaly,
				Severity:    NudgeSeverityWarning,
				Message:     "Terdeteksi transaksi dengan jumlah tidak biasa (" + formatMoney(int64(float64(t))) + "). Periksa riwayat transaksi Anda.",
				Action:      "Lihat Riwayat",
				Dismissible: true,
			}
		}
	}

	return nil
}

// ────────────────────────────────────────────────────────────────
// Exhaustion Projection
// ────────────────────────────────────────────────────────────────

// ExhaustionProjection uses linear extrapolation to estimate when the budget cap
// will be reached based on current daily burn rate. Flags if projected exhaustion
// is within 7 days, or if ≥90% of budget has been used with ≥2 days remaining.
//
// Parameters:
//   - budgetCap: the monthly budget limit in sen.
//   - spentSoFar: amount spent so far in sen.
//   - dailySpending: daily spending amounts in sen for computing burn rate.
//   - daysRemainingInPeriod: number of days remaining in the budget period.
//
// Pure function: no I/O, deterministic for same inputs.
func ExhaustionProjection(budgetCap, spentSoFar int64, dailySpending []int64, daysRemainingInPeriod int) *Nudge {
	if budgetCap <= 0 || len(dailySpending) == 0 {
		return nil
	}

	// Check 1: Already exceeded budget.
	if spentSoFar >= budgetCap {
		return &Nudge{
			Type:        NudgeTypeExhaustion,
			Severity:    NudgeSeverityCritical,
			Message:     "Anggaran Anda sudah habis. Batasi pengeluaran untuk sisa periode ini.",
			Action:      "Lihat Anggaran",
			Dismissible: true,
		}
	}

	// Check 2: High usage threshold (≥90%) with sufficient remaining days.
	usagePct := float64(spentSoFar) / float64(budgetCap) * 100.0
	if usagePct >= 90.0 && daysRemainingInPeriod >= 2 {
		// Compute projected exhaustion date based on burn rate.
		burnRate := computeAverage(dailySpending)
		remainingBudget := float64(budgetCap - spentSoFar)

		var projectedDays int
		if burnRate > 0 {
			projectedDays = int(math.Ceil(remainingBudget / burnRate))
		}

		msg := "Anggaran " + formatPct(usagePct) + " sudah terpakai."
		if projectedDays > 0 && projectedDays <= daysRemainingInPeriod {
			msg += " Dengan kecepatan saat ini, anggaran akan habis dalam " + formatInt(projectedDays) + " hari."
		}

		return &Nudge{
			Type:        NudgeTypeExhaustion,
			Severity:    NudgeSeverityCritical,
			Message:     msg,
			Action:      "Lihat Anggaran",
			Dismissible: true,
		}
	}

	// Check 3: Linear extrapolation — projected exhaustion within 7 days.
	burnRate := computeAverage(dailySpending)
	if burnRate <= 0 {
		return nil
	}

	remainingBudget := float64(budgetCap - spentSoFar)
	daysToExhaustion := remainingBudget / burnRate

	if daysToExhaustion > 0 && daysToExhaustion < 7.0 {
		return &Nudge{
			Type:        NudgeTypeExhaustion,
			Severity:    NudgeSeverityCritical,
			Message:     "Anggaran diproyeksikan habis dalam " + formatInt(int(math.Ceil(daysToExhaustion))) + " hari dengan pengeluaran saat ini. Kurangi pengeluaran untuk menghindari kelebihan anggaran.",
			Action:      "Lihat Anggaran",
			Dismissible: true,
		}
	}

	return nil
}

// ────────────────────────────────────────────────────────────────
// Combined Evaluation
// ────────────────────────────────────────────────────────────────

// EvaluateAll runs all four nudge checks and returns the non-nil results.
// This is the main entry point for the nudge engine.
//
// Parameters:
//   - recentSpending: transaction amounts in the current evaluation window.
//   - dailyHistory: daily spending totals for the baseline period (used for both velocity rate and exhaustion projection).
//   - dailyAvg: daily spending averages for trend detection.
//   - categoryTransactions: transaction amounts for anomaly detection.
//   - budgetCap: monthly budget limit in sen.
//   - spentSoFar: amount spent so far in sen.
//   - daysRemainingInPeriod: days remaining in the budget period.
//
// Pure function: no I/O, deterministic for same inputs.
func EvaluateAll(
	recentSpending, dailyHistory []int64,
	dailyAvg []float64,
	categoryTransactions []int64,
	budgetCap, spentSoFar int64,
	daysRemainingInPeriod int,
) []Nudge {
	var nudges []Nudge

	if n := VelocityRate(recentSpending, dailyHistory); n != nil {
		nudges = append(nudges, *n)
	}
	if n := TrendDetection(dailyAvg); n != nil {
		nudges = append(nudges, *n)
	}
	if n := AnomalyFlagging(categoryTransactions); n != nil {
		nudges = append(nudges, *n)
	}
	if n := ExhaustionProjection(budgetCap, spentSoFar, dailyHistory, daysRemainingInPeriod); n != nil {
		nudges = append(nudges, *n)
	}

	if nudges == nil {
		nudges = []Nudge{}
	}

	return nudges
}

// ────────────────────────────────────────────────────────────────
// Formatting Helpers
// ────────────────────────────────────────────────────────────────

// formatRatio formats a velocity ratio to one decimal place.
func formatRatio(r float64) string {
	whole := int(r)
	frac := int((r - float64(whole)) * 10)
	if frac < 0 {
		frac = 0
	}
	return itoa(whole) + "," + itoa(frac)
}

// formatMoney formats an integer sen amount as IDR without decimal (display value).
func formatMoney(m int64) string {
	// Convert sen to IDR (1 IDR = 100 sen).
	idr := m / 100
	rem := m % 100
	if rem >= 50 {
		idr++ // round up
	}
	return "Rp" + formatThousands(idr)
}

// formatPct formats a percentage to one decimal place.
func formatPct(pct float64) string {
	whole := int(pct)
	frac := int((pct - float64(whole)) * 10)
	if frac < 0 {
		frac = 0
	}
	return itoa(whole) + "," + itoa(frac) + "%"
}

// formatInt formats an integer.
func formatInt(n int) string {
	return itoa(n)
}

// formatThousands formats an integer with thousands separator dots.
func formatThousands(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		if len(digits) > 0 && len(digits)%4 == 3 {
			digits = append(digits, '.')
		}
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	// Reverse.
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// itoa converts a non-negative integer to a string without allocations.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
