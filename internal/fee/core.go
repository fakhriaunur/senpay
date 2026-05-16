package fee

import (
	"senpay/internal/types"
)

// TransferInput represents a single transfer for fee calculation.
type TransferInput struct {
	Amount   types.Money
	KYCLevel types.KYCLevel
}

// CalcFee calculates the transfer fee based on amount, user KYC level, and config.
//
// Basic KYC: flat fee from FeeConfig.FlatFeeBasicSen.
// Verified KYC: percentage-based fee (RateVerifiedPct% of amount) with
//   MinFeeSen minimum floor.
// Unknown KYC levels are treated as basic.
//
// Pure function: no I/O, no side effects, deterministic for given inputs.
//
// Errors:
//   - ErrInvalidAmount when amount <= 0
func CalcFee(amount types.Money, userKYC types.KYCLevel, cfg FeeConfig) (types.Money, *types.DomainError) {
	if !amount.IsPositive() {
		return 0, &types.ErrInvalidAmount
	}

	switch userKYC {
	case types.KYCLevelBasic:
		return calcBasicFee(cfg), nil
	case types.KYCLevelVerified:
		return calcVerifiedFee(amount, cfg), nil
	default:
		// Unknown KYC levels treated as basic
		return calcBasicFee(cfg), nil
	}
}

// CalcFees calculates the total fee across a batch of transfers.
// Returns the sum with overflow protection.
//
// Errors:
//   - ErrInvalidAmount if any individual fee calculation fails
//   - ErrInternal if total overflows int64
func CalcFees(inputs []TransferInput, cfg FeeConfig) (types.Money, *types.DomainError) {
	var total types.Money
	for _, input := range inputs {
		fee, err := CalcFee(input.Amount, input.KYCLevel, cfg)
		if err != nil {
			return 0, err
		}
		var overflow bool
		total, overflow = types.SafeAdd(total, fee)
		if overflow {
			return 0, &types.ErrInternal
		}
	}
	return total, nil
}
