package fee

import (
	"math"

	"senpay/internal/types"
)

// TransferInput represents a single transfer for fee calculation.
type TransferInput struct {
	Amount   types.Money
	KYCLevel types.KYCLevel
}

const (
	basicFeeFlat       = types.Money(2500) // Rp 2,500 flat fee for basic KYC
	verifiedFeePercent = 7                 // 0.7% = 7/1000
	verifiedFeeFloor   = types.Money(1000) // Rp 1,000 minimum fee for verified KYC
)

// CalcFee calculates the transfer fee based on amount and user KYC level.
//
// Basic KYC: flat Rp 2,500 fee regardless of amount.
// Verified KYC: 0.7% of transfer amount with Rp 1,000 minimum floor.
// Unknown KYC levels are treated as basic.
//
// Errors:
//   - ErrInvalidAmount when amount <= 0
func CalcFee(amount types.Money, userKYC types.KYCLevel) (types.Money, *types.DomainError) {
	if !amount.IsPositive() {
		return 0, &types.ErrInvalidAmount
	}

	switch userKYC {
	case types.KYCLevelBasic:
		return basicFeeFlat, nil
	case types.KYCLevelVerified:
		// 0.7% = amount * 7 / 1000.
		// Use overflow-safe computation: for amounts above MaxInt64/7,
		// divide first to avoid overflow.
		const overflowThreshold = math.MaxInt64 / verifiedFeePercent
		var fee types.Money
		if amount > types.Money(overflowThreshold) {
			fee = amount / 1000 * verifiedFeePercent
		} else {
			fee = amount * verifiedFeePercent / 1000
		}
		if fee < verifiedFeeFloor {
			fee = verifiedFeeFloor
		}
		return fee, nil
	default:
		// Unknown KYC levels treated as basic
		return basicFeeFlat, nil
	}
}

// CalcFees calculates the total fee across a batch of transfers.
// Returns the sum with overflow protection.
//
// Errors:
//   - ErrInvalidAmount if any individual fee calculation fails
//   - ErrInternal if total overflows int64
func CalcFees(inputs []TransferInput) (types.Money, *types.DomainError) {
	var total types.Money
	for _, input := range inputs {
		fee, err := CalcFee(input.Amount, input.KYCLevel)
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
