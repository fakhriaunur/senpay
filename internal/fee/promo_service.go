package fee

import (
	"time"

	"senpay/internal/types"
)

// PromoService handles promo code validation and discount application.
// This is a shell-layer service that applies discounts based on the promo config.
// It does NOT modify CalcFee — that remains a pure function in core.go.
type PromoService struct {
	config PromoConfig
}

// NewPromoService creates a PromoService with the given promo configuration.
func NewPromoService(config PromoConfig) *PromoService {
	return &PromoService{config: config}
}

// ApplyPromoCodeResult holds the result of applying a promo code to a fee.
type ApplyPromoCodeResult struct {
	// DiscountedFee is the fee after applying the promo discount.
	DiscountedFee types.Money
	// PromoWarning is set when the promo code is invalid or expired.
	// The transfer should still proceed with the original fee.
	PromoWarning *types.DomainError
	// Applied indicates whether the discount was successfully applied.
	Applied bool
}

// ApplyPromoCode validates a promo code and applies the discount.
//
// Rules:
//   - If promoCode is empty: returns original fee with no warning.
//   - If promoCode fails format validation (ParsePromoCode): returns original fee
//     with a PromoWarning (invalid format). Transfer still proceeds.
//   - If promoCode is not in the campaign codes list: returns original fee
//     with a PromoWarning (invalid code). Transfer still proceeds.
//   - If promoCode is valid but outside the free_transfer_window: returns original
//     fee with a PromoWarning (expired). Transfer still proceeds.
//   - If promoCode is valid and within the window: returns discounted fee
//     (fee * (100 - discount_pct) / 100) with no warning.
//
// The caller (transfer service) uses DiscountedFee for the actual transfer
// and surfaces PromoWarning in the API response.
func (s *PromoService) ApplyPromoCode(promoCode string, fee types.Money, now time.Time) ApplyPromoCodeResult {
	if promoCode == "" {
		return ApplyPromoCodeResult{
			DiscountedFee: fee,
			Applied:       false,
		}
	}

	// 1. Format validation.
	_, err := types.ParsePromoCode(promoCode)
	if err != nil {
		return ApplyPromoCodeResult{
			DiscountedFee: fee,
			PromoWarning:  &types.ErrPromoCodeInvalid,
			Applied:       false,
		}
	}

	// 2. Check campaign code list.
	if !s.isCampaignCode(promoCode) {
		return ApplyPromoCodeResult{
			DiscountedFee: fee,
			PromoWarning:  &types.ErrPromoCodeInvalid,
			Applied:       false,
		}
	}

	// 3. Check free transfer window.
	if !s.config.FreeTransferWindow.Contains(now) {
		return ApplyPromoCodeResult{
			DiscountedFee: fee,
			PromoWarning:  &types.ErrPromoCodeExpired,
			Applied:       false,
		}
	}

	// 4. Apply discount.
	discountedFee := s.applyDiscount(fee)
	return ApplyPromoCodeResult{
		DiscountedFee: discountedFee,
		Applied:       true,
	}
}

// isCampaignCode checks if the given code is in the configured campaign codes list.
func (s *PromoService) isCampaignCode(code string) bool {
	for _, c := range s.config.CampaignCodes {
		if c == code {
			return true
		}
	}
	return false
}

// applyDiscount applies the configured discount percentage to the given fee.
// discount_pct 100.0 → fee becomes 0 (free).
// discount_pct 50.0 → fee becomes half.
func (s *PromoService) applyDiscount(f types.Money) types.Money {
	if s.config.DiscountPct >= 100.0 {
		return 0
	}
	if s.config.DiscountPct <= 0 {
		return f
	}
	// fee - fee * discountPct / 100
	discount := int64(float64(f) * s.config.DiscountPct / 100.0)
	if types.Money(discount) >= f {
		return 0
	}
	return f - types.Money(discount)
}
