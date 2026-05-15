package fee

import (
	"math"
	"testing"

	"senpay/internal/types"

	"pgregory.net/rapid"
)

func TestCalcFee(t *testing.T) {
	t.Parallel()

	maxI64 := types.Money(math.MaxInt64)

	tests := []struct {
		name     string
		amount   types.Money
		kycLevel string
		want     types.Money
		wantErr  bool
		wantCode string
	}{
		// ── Basic KYC: flat Rp 2,500 ───────────────────────────
		{name: "basic_minimum", amount: 1, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_small", amount: 100, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_medium", amount: 50000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_large", amount: 10000000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_very_large", amount: 1000000000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_max", amount: maxI64, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_one_sen", amount: 1, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_1000_sen", amount: 1000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_2500_sen", amount: 2500, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_100000_sen", amount: 100000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_1million_sen", amount: 1000000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_10million_sen", amount: 10000000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_100million_sen", amount: 100000000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_1billion_sen", amount: 1000000000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_1trillion_sen", amount: 1000000000000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_exact_bi_limit", amount: 200000, kycLevel: types.KYCLevelBasic, want: 2500},

		// ── Verified KYC: 0.7% with Rp 1,000 floor ─────────────
		// 0.7% = amount * 7 / 1000
		// Floor: 1000 sen (Rp 1,000)
		// At amount = 142858: 142858*7/1000 = 1000 (at floor exactly)
		// Below that: fee = floor = 1000
		// Above that: fee = amount * 7 / 1000

		{name: "verified_one_sen", amount: 1, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_100_sen", amount: 100, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_1000_sen", amount: 1000, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_below_floor", amount: 100000, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_at_floor_boundary", amount: 142857, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_just_above_floor", amount: 142858, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_small_above_floor", amount: 150000, kycLevel: types.KYCLevelVerified, want: 1050},
		{name: "verified_200000_sen", amount: 200000, kycLevel: types.KYCLevelVerified, want: 1400},
		{name: "verified_500000_sen", amount: 500000, kycLevel: types.KYCLevelVerified, want: 3500},
		{name: "verified_1million_sen", amount: 1000000, kycLevel: types.KYCLevelVerified, want: 7000},
		{name: "verified_2million_sen", amount: 2000000, kycLevel: types.KYCLevelVerified, want: 14000},
		{name: "verified_5million_sen", amount: 5000000, kycLevel: types.KYCLevelVerified, want: 35000},
		{name: "verified_10million_sen", amount: 10000000, kycLevel: types.KYCLevelVerified, want: 70000},
		{name: "verified_50million_sen", amount: 50000000, kycLevel: types.KYCLevelVerified, want: 350000},
		{name: "verified_100million_sen", amount: 100000000, kycLevel: types.KYCLevelVerified, want: 700000},
		{name: "verified_1billion_sen", amount: 1000000000, kycLevel: types.KYCLevelVerified, want: 7000000},
		{name: "verified_exact_bi_limit", amount: 1000000, kycLevel: types.KYCLevelVerified, want: 7000},

		// ── Verified KYC: edge calculations ─────────────────────
		// At amount = 142858: 142858*7/1000 = 1000006/1000 = 1000 (integer)
		{name: "verified_floor_exact_math", amount: 142858, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_floor_minus_one", amount: 142857, kycLevel: types.KYCLevelVerified, want: 1000},
		{name: "verified_floor_plus_100k", amount: 242858, kycLevel: types.KYCLevelVerified, want: 1700},
		{name: "verified_round_amount", amount: 1000000, kycLevel: types.KYCLevelVerified, want: 7000},
		{name: "verified_even_division", amount: 7000000, kycLevel: types.KYCLevelVerified, want: 49000},
		{name: "verified_large_round", amount: 100000000, kycLevel: types.KYCLevelVerified, want: 700000},
		{name: "verified_max_safe", amount: maxI64, kycLevel: types.KYCLevelVerified, want: maxI64 / 1000 * 7},

		// Upper bound: fee never exceeds amount.
		// For verified: fee = amount * 7/1000 = 0.7% of amount, so fee < amount always.
		// For basic: fee = 2500, so for amount < 2500, fee > amount.
		{name: "verified_fee_less_than_amount", amount: 1000000, kycLevel: types.KYCLevelVerified, want: 7000},
		{name: "verified_fee_less_than_amount_large", amount: 1000000000, kycLevel: types.KYCLevelVerified, want: 7000000},
		{name: "verified_floor_less_than_amount", amount: 10000000, kycLevel: types.KYCLevelVerified, want: 70000},

		// ── Error cases ─────────────────────────────────────────
		{name: "zero_amount_basic", amount: 0, kycLevel: types.KYCLevelBasic,
			wantErr: true, wantCode: types.ErrCodeInvalidAmount},
		{name: "zero_amount_verified", amount: 0, kycLevel: types.KYCLevelVerified,
			wantErr: true, wantCode: types.ErrCodeInvalidAmount},
		{name: "negative_amount_basic", amount: -1, kycLevel: types.KYCLevelBasic,
			wantErr: true, wantCode: types.ErrCodeInvalidAmount},
		{name: "negative_amount_verified", amount: -1000, kycLevel: types.KYCLevelVerified,
			wantErr: true, wantCode: types.ErrCodeInvalidAmount},
		{name: "negative_large_basic", amount: -9999999, kycLevel: types.KYCLevelBasic,
			wantErr: true, wantCode: types.ErrCodeInvalidAmount},
		{name: "negative_large_verified", amount: -9999999, kycLevel: types.KYCLevelVerified,
			wantErr: true, wantCode: types.ErrCodeInvalidAmount},
		{name: "negative_max_basic", amount: types.Money(math.MinInt64), kycLevel: types.KYCLevelBasic,
			wantErr: true, wantCode: types.ErrCodeInvalidAmount},

		// ── Unknown KYC level (treated as basic) ────────────────
		{name: "unknown_kyc_small", amount: 100, kycLevel: "unknown", want: 2500},
		{name: "unknown_kyc_empty", amount: 50000, kycLevel: "", want: 2500},
		{name: "unknown_kyc_platinum", amount: 1000000, kycLevel: "platinum", want: 2500},
		{name: "unknown_kyc_premium", amount: maxI64, kycLevel: "premium", want: 2500},

		// ── Cross-tier boundary checks ──────────────────────────
		{name: "basic_vs_verified_same_amount", amount: 100000, kycLevel: types.KYCLevelBasic, want: 2500},
		{name: "basic_100000_vs_verified_100000", amount: 100000, kycLevel: types.KYCLevelBasic, want: 2500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalcFee(tt.amount, tt.kycLevel)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Code != tt.wantCode {
					t.Errorf("error code: got %q, want %q", err.Code, tt.wantCode)
				}
				if got != 0 {
					t.Errorf("fee should be 0 on error, got %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("CalcFee(%d, %q) = %d, want %d", tt.amount, tt.kycLevel, got, tt.want)
			}
		})
	}
}

func TestCalcFee_ParameterizedEdgeValues(t *testing.T) {
	t.Parallel()

	// Test all combinations of edge values for basic KYC.
	edgeAmounts := []types.Money{
		1, 100, 500, 999, 1000, 2499, 2500, 2501,
		5000, 10000, 50000, 99999, 100000, 500000,
		1000000, 5000000, 10000000, 50000000, 100000000,
		1000000000, types.Money(math.MaxInt64),
	}

	t.Run("basic_all_edge_values", func(t *testing.T) {
		for _, amt := range edgeAmounts {
			if amt <= 0 {
				continue
			}
			fee, err := CalcFee(amt, types.KYCLevelBasic)
			if err != nil {
				t.Errorf("unexpected error for amount=%d: %v", amt, err)
			}
			if fee != 2500 {
				t.Errorf("basic fee for amount=%d: got %d, want 2500", amt, fee)
			}
		}
	})

	t.Run("verified_all_edge_values", func(t *testing.T) {
		for _, amt := range edgeAmounts {
			if amt <= 0 {
				continue
			}
			fee, err := CalcFee(amt, types.KYCLevelVerified)
			if err != nil {
				t.Errorf("unexpected error for amount=%d: %v", amt, err)
			}
			// Match overflow-safe formula from CalcFee: for amounts above MaxInt64/7,
			// use amt/1000*7 to avoid overflow; otherwise use amt*7/1000.
			const overflowThreshold = math.MaxInt64 / 7
			var expected types.Money
			if amt > types.Money(overflowThreshold) {
				expected = amt / 1000 * 7
			} else {
				expected = amt * 7 / 1000
			}
			if expected < 1000 {
				expected = 1000
			}
			if fee != expected {
				t.Errorf("verified fee for amount=%d: got %d, want %d", amt, fee, expected)
			}
		}
	})
}

func TestCalcFees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []TransferInput
		want    types.Money
		wantErr bool
		wantErrCode string
	}{
		{
			name:    "empty_slice",
			inputs:  []TransferInput{},
			want:    0,
		},
		{
			name: "single_basic",
			inputs: []TransferInput{
				{Amount: 100000, KYCLevel: types.KYCLevelBasic},
			},
			want: 2500,
		},
		{
			name: "single_verified",
			inputs: []TransferInput{
				{Amount: 1000000, KYCLevel: types.KYCLevelVerified},
			},
			want: 7000,
		},
		{
			name: "multiple_basic",
			inputs: []TransferInput{
				{Amount: 50000, KYCLevel: types.KYCLevelBasic},
				{Amount: 100000, KYCLevel: types.KYCLevelBasic},
				{Amount: 200000, KYCLevel: types.KYCLevelBasic},
			},
			want: 7500, // 3 * 2500
		},
		{
			name: "multiple_verified",
			inputs: []TransferInput{
				{Amount: 1000000, KYCLevel: types.KYCLevelVerified},
				{Amount: 2000000, KYCLevel: types.KYCLevelVerified},
				{Amount: 5000000, KYCLevel: types.KYCLevelVerified},
			},
			want: 56000, // 7000 + 14000 + 35000
		},
		{
			name: "mixed_kyc_levels",
			inputs: []TransferInput{
				{Amount: 100000, KYCLevel: types.KYCLevelBasic},     // 2500
				{Amount: 1000000, KYCLevel: types.KYCLevelVerified},  // 7000
				{Amount: 50000, KYCLevel: types.KYCLevelBasic},       // 2500
			},
			want: 12000, // 2500 + 7000 + 2500
		},
		{
			name: "verified_below_and_above_floor",
			inputs: []TransferInput{
				{Amount: 100000, KYCLevel: types.KYCLevelVerified},   // 1000 (floor)
				{Amount: 1000000, KYCLevel: types.KYCLevelVerified},  // 7000
			},
			want: 8000,
		},
		{
			name: "many_small_transfers",
			inputs: []TransferInput{
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
				{Amount: 100, KYCLevel: types.KYCLevelBasic},
			},
			want: 25000, // 10 * 2500
		},
		{
			name: "unknown_kyc_treated_as_basic",
			inputs: []TransferInput{
				{Amount: 50000, KYCLevel: "unknown"},
				{Amount: 75000, KYCLevel: ""},
			},
			want: 5000, // 2 * 2500
		},
		{
			name: "single_verified_below_floor",
			inputs: []TransferInput{
				{Amount: 100000, KYCLevel: types.KYCLevelVerified},
			},
			want: 1000,
		},
		{
			name: "error_zero_amount",
			inputs: []TransferInput{
				{Amount: 0, KYCLevel: types.KYCLevelBasic},
			},
			wantErr: true,
			wantErrCode: types.ErrCodeInvalidAmount,
		},
		{
			name: "error_negative_amount",
			inputs: []TransferInput{
				{Amount: 100000, KYCLevel: types.KYCLevelBasic},
				{Amount: -5000, KYCLevel: types.KYCLevelVerified},
			},
			wantErr: true,
			wantErrCode: types.ErrCodeInvalidAmount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalcFees(tt.inputs)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Code != tt.wantErrCode {
					t.Errorf("error code: got %q, want %q", err.Code, tt.wantErrCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CalcFees = %d, want %d", got, tt.want)
			}
		})
	}
}

// ── Rapid property-based tests ──────────────────────────────────

func TestProperty_CalcFee_NonNegative(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Int64Range(1, math.MaxInt64).Draw(t, "amount")
		kycLevel := rapid.SampledFrom([]string{
			types.KYCLevelBasic,
			types.KYCLevelVerified,
			"unknown",
			"",
		}).Draw(t, "kycLevel")

		fee, err := CalcFee(types.Money(amount), kycLevel)
		if err != nil {
			t.Fatalf("unexpected error for amount=%d kyc=%q: %v", amount, kycLevel, err)
		}

		// Fee must always be non-negative.
		if fee < 0 {
			t.Fatalf("negative fee: %d", fee)
		}
	})
}

func TestProperty_CalcFee_FormulaVerified(t *testing.T) {
	t.Parallel()

	// For verified KYC with large enough amounts (above floor threshold),
	// fee = amount * 7 / 1000 (with overflow protection), and this must be <= amount.
	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Int64Range(143000, math.MaxInt64).Draw(t, "amount")

		fee, err := CalcFee(types.Money(amount), types.KYCLevelVerified)
		if err != nil {
			t.Fatalf("unexpected error for amount=%d: %v", amount, err)
		}

		// Match the overflow-safe formula from CalcFee.
		const overflowThreshold = math.MaxInt64 / 7
		var expected types.Money
		if amount > overflowThreshold {
			expected = types.Money(amount) / 1000 * 7
		} else {
			expected = types.Money(amount) * 7 / 1000
		}
		if fee != expected {
			t.Fatalf("verified fee for amount=%d: got %d, want %d", amount, fee, expected)
		}

		// For verified KYC above floor, fee must be <= amount.
		if int64(fee) > amount {
			t.Fatalf("fee=%d > amount=%d for verified KYC above floor", fee, amount)
		}
	})
}

func TestProperty_CalcFee_VerifiedFloor(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Int64Range(1, 142857).Draw(t, "amount") // below floor threshold

		fee, err := CalcFee(types.Money(amount), types.KYCLevelVerified)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// For verified KYC with amount <= 142857, fee should be floor (1000).
		if fee != 1000 {
			t.Fatalf("expected floor 1000 for amount=%d, got %d", amount, fee)
		}
	})
}

func TestProperty_CalcFee_BasicAlways2500(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Int64Range(1, math.MaxInt64).Draw(t, "amount")

		fee, err := CalcFee(types.Money(amount), types.KYCLevelBasic)
		if err != nil {
			t.Fatalf("unexpected error for amount=%d: %v", amount, err)
		}

		if fee != 2500 {
			t.Fatalf("basic fee for amount=%d: got %d, want 2500", amount, fee)
		}
	})
}

func TestProperty_CalcFees_SumMatches(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 20).Draw(t, "numInputs")
		inputs := make([]TransferInput, n)
		var expectedTotal types.Money

		for i := 0; i < n; i++ {
			amount := rapid.Int64Range(1, 10000000).Draw(t, "amount")
			kyc := rapid.SampledFrom([]string{
				types.KYCLevelBasic,
				types.KYCLevelVerified,
			}).Draw(t, "kycLevel")

			inputs[i] = TransferInput{
				Amount:   types.Money(amount),
				KYCLevel: kyc,
			}

			fee, err := CalcFee(types.Money(amount), kyc)
			if err != nil {
				t.Fatalf("CalcFee error: %v", err)
			}
			sum, overflow := types.SafeAdd(expectedTotal, fee)
			if overflow {
				return // skip overflow cases
			}
			expectedTotal = sum
		}

		total, err := CalcFees(inputs)
		if err != nil {
			t.Fatalf("CalcFees error: %v", err)
		}
		if total != expectedTotal {
			t.Fatalf("CalcFees=%d, sum of individual CalcFee=%d", total, expectedTotal)
		}
	})
}
