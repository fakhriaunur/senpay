package auth

import (
	"strings"

	"senpay/internal/types"

	"golang.org/x/crypto/bcrypt"
)

// pepper is a server-side secret added to PINs before hashing.
// In production, this should come from environment configuration.
const pepper = "senpay-p3pper-2026"

// HashPIN hashes a PIN with bcrypt at cost 12.
// Returns the bcrypt hash as a string.
// The PIN is combined with a server-side pepper before hashing
// to provide an additional layer of secrecy.
func HashPIN(pin string) string {
	combined := pepper + pin
	hash, err := bcrypt.GenerateFromPassword([]byte(combined), types.BcryptCost)
	if err != nil {
		panic("auth: bcrypt hash failed: " + err.Error())
	}
	return string(hash)
}

// VerifyPIN verifies a PIN against a bcrypt hash.
// Returns true if the PIN matches the hash, false otherwise.
// Uses bcrypt.CompareHashAndPassword which provides timing-safe comparison.
func VerifyPIN(pin string, hash string) bool {
	combined := pepper + pin
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(combined))
	return err == nil
}

// ValidatePhone validates an Indonesian phone number.
// Rules:
//   - Must start with "08" or "62" (after stripping optional leading "+")
//   - Must be 10-13 digits long
//   - Only numeric digits allowed (plus optional leading "+")
//
// Returns a DomainError if invalid, nil if valid.
func ValidatePhone(phone string) *types.DomainError {
	if phone == "" {
		err := types.NewInvalidFormatError("phone", "nomor telepon tidak boleh kosong")
		return &err
	}

	// Strip leading "+" if present.
	normalized := phone
	if normalized[0] == '+' {
		normalized = normalized[1:]
	}

	if normalized == "" {
		err := types.NewInvalidFormatError("phone", "nomor telepon tidak boleh kosong")
		return &err
	}

	// Verify all remaining characters are digits.
	for _, c := range normalized {
		if c < '0' || c > '9' {
			err := types.NewInvalidFormatError("phone", "hanya angka yang diperbolehkan")
			return &err
		}
	}

	// Check length.
	digitLen := len(normalized)
	if digitLen < types.PhoneMinLength || digitLen > types.PhoneMaxLength {
		err := DomainErrorPhoneLength
		return &err
	}

	// Check prefix.
	if !strings.HasPrefix(normalized, types.PhonePrefix08) && !strings.HasPrefix(normalized, types.PhonePrefix62) {
		err := types.NewInvalidFormatError("phone", "nomor harus dimulai dengan 08 atau 62")
		return &err
	}

	return nil
}

// DomainErrorPhoneLength is the error for phone numbers outside 10-13 digit range.
var DomainErrorPhoneLength = types.DomainError{
	Code:       types.ErrCodeInvalidFormat,
	Message:    "Format nomor telepon tidak valid: nomor harus 10-13 digit",
	HTTPStatus: 400,
}
