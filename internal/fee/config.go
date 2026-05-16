package fee

import (
	"fmt"
	"math"
	"os"

	"senpay/internal/types"

	"gopkg.in/yaml.v3"
)

// FeeConfig holds fee calculation parameters parsed from fees.yaml.
// Loaded once at startup — changes require server restart (no hot-reload).
type FeeConfig struct {
	// FlatFeeBasicSen is the flat fee in sen for basic KYC transfers.
	FlatFeeBasicSen int64 `yaml:"flat_fee_basic_sen"`
	// RateVerifiedPct is the percentage fee for verified KYC transfers.
	// Applied as: amount * RateVerifiedPct / 100.
	// Example: 0.7 means 0.7% = 0.007 of the transfer amount.
	RateVerifiedPct float64 `yaml:"rate_verified_pct"`
	// MinFeeSen is the minimum fee in sen for verified KYC transfers (floor).
	MinFeeSen int64 `yaml:"min_fee_sen"`
}

// DefaultFeeConfig returns the default fee configuration.
// These values match the pre-v0.2.0 hardcoded constants.
func DefaultFeeConfig() FeeConfig {
	return FeeConfig{
		FlatFeeBasicSen: 2500,
		RateVerifiedPct: 0.7,
		MinFeeSen:       1000,
	}
}

// LoadFeeConfig reads and parses a fees.yaml file.
// Returns (nil, error) for any issue: file not found, invalid YAML,
// or validation failure (negative/zero values).
//
// Parse-Don't-Validate: the struct is fully parsed first, then validated.
// Callers must crash-early on error (os.Exit(1)).
func LoadFeeConfig(path string) (*FeeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fee config: read file %s: %w", path, err)
	}

	var cfg FeeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("fee config: parse YAML from %s: %w", path, err)
	}

	// Validate parsed values (Parse-Don't-Validate: reject invalid data at boundary).
	if cfg.FlatFeeBasicSen <= 0 {
		return nil, fmt.Errorf(
			"fee config: flat_fee_basic_sen must be positive, got %d", cfg.FlatFeeBasicSen,
		)
	}
	if cfg.RateVerifiedPct <= 0 {
		return nil, fmt.Errorf(
			"fee config: rate_verified_pct must be positive, got %f", cfg.RateVerifiedPct,
		)
	}
	if cfg.MinFeeSen <= 0 {
		return nil, fmt.Errorf(
			"fee config: min_fee_sen must be positive, got %d", cfg.MinFeeSen,
		)
	}

	return &cfg, nil
}

// calcBasicFee returns the flat fee for basic KYC transfers.
func calcBasicFee(cfg FeeConfig) types.Money {
	return types.Money(cfg.FlatFeeBasicSen)
}

// calcVerifiedFee returns the percentage-based fee for verified KYC transfers,
// floored at the configured minimum.
//
// Uses integer arithmetic with overflow-safe approach.
// The percentage rate_verified_pct is converted to a reduced fraction
// (numerator/denominator) for precise integer computation:
//
//	rate_verified_pct% = rate_verified_pct / 100
//	fee = amount * num / den
//
// where num/den is the reduced fraction of (rate_verified_pct * 10000) / 1000000.
// For rate_verified_pct = 0.7: num=7, den=1000 → fee = amount * 7 / 1000 ✓
func calcVerifiedFee(amount types.Money, cfg FeeConfig) types.Money {
	// Compute the unreduced fraction: pctBasis / 1000000
	// where pctBasis = round(rate_verified_pct * 10000)
	// e.g., 0.7 → pctBasis=7000, fraction=7000/1000000
	pctBasis := int64(math.Round(cfg.RateVerifiedPct * 10000))
	if pctBasis <= 0 {
		pctBasis = 1
	}

	const baseDen int64 = 1000000 // 10000 × 100

	// Reduce the fraction for precise integer arithmetic.
	num, den := reduceFraction(pctBasis, baseDen)

	var fee types.Money
	// Overflow-safe computation: avoid amount * num > MaxInt64.
	maxSafeAmount := types.Money(math.MaxInt64 / num)
	if amount > maxSafeAmount {
		// Divide first to avoid overflow: (amount / den) * num
		fee = (amount / types.Money(den)) * types.Money(num)
	} else {
		fee = amount * types.Money(num) / types.Money(den)
	}

	// Apply minimum floor.
	if fee < types.Money(cfg.MinFeeSen) {
		fee = types.Money(cfg.MinFeeSen)
	}
	return fee
}

// reduceFraction reduces a fraction to lowest terms using GCD.
func reduceFraction(num, den int64) (int64, int64) {
	g := gcd64(num, den)
	return num / g, den / g
}

// gcd64 computes the greatest common divisor of a and b using Euclidean algorithm.
func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
