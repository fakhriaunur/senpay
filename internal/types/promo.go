package types

import (
	"regexp"
)

// PromoCode is a validated promo code string.
// Only alphanumeric characters (A-Z, a-z, 0-9) and hyphens (-) are allowed.
// Matches the regex [a-zA-Z0-9-]+.
type PromoCode string

// promoCodePattern is the allowed character pattern for promo codes.
var promoCodePattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// ParsePromoCode validates a string as a PromoCode.
// Returns the parsed PromoCode on success, or a DomainError on invalid input.
//
// Allowed characters: A-Z, a-z, 0-9, hyphen (-).
// Empty strings are rejected.
// Non-ASCII characters, spaces, underscores, and special characters are rejected.
func ParsePromoCode(s string) (PromoCode, error) {
	if s == "" {
		return "", NewInvalidFormatError("kode promo", "kode promo tidak boleh kosong")
	}

	if !promoCodePattern.MatchString(s) {
		return "", DomainError{
			Code:       ErrCodeInvalidFormat,
			Message:    "Kode promo tidak valid: hanya boleh berisi huruf, angka, dan tanda hubung",
			HTTPStatus: 400,
		}
	}

	return PromoCode(s), nil
}

// String returns the string representation of PromoCode.
func (p PromoCode) String() string {
	return string(p)
}
