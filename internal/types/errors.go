package types

import (
	"fmt"
)

// DomainError represents a typed application error with Indonesian error taxonomy.
// Implements the error interface for use in Go error handling.
type DomainError struct {
	Code       string        `json:"code"`
	Message    string        `json:"message"`
	HTTPStatus int           `json:"-"`
	Args       []interface{} `json:"-"` // Format arguments for i18n message resolution
}

// Error implements the error interface.
func (e DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Domain error codes for Indonesian error taxonomy.
const (
	ErrCodeInvalidAmount          = "INVALID_AMOUNT"
	ErrCodeInsufficientBalance    = "INSUFFICIENT_BALANCE"
	ErrCodeSelfTransfer           = "SELF_TRANSFER"
	ErrCodeUserNotFound           = "USER_NOT_FOUND"
	ErrCodePhoneAlreadyRegistered = "PHONE_ALREADY_REGISTERED"
	ErrCodeInvalidPIN             = "INVALID_PIN"
	ErrCodeUnauthorized           = "UNAUTHORIZED"
	ErrCodeExceedsTransactionLimit = "EXCEEDS_TRANSACTION_LIMIT"
	ErrCodeAmountBelowMinimum     = "AMOUNT_BELOW_MINIMUM"
	ErrCodeRequestInFlight        = "REQUEST_IN_FLIGHT"
	ErrCodeSerializationConflict  = "SERIALIZATION_CONFLICT"
	ErrCodeMissingField           = "MISSING_FIELD"
	ErrCodeInvalidFormat          = "INVALID_FORMAT"
	ErrCodeDuplicateTransaction   = "DUPLICATE_TRANSACTION"
	ErrCodeFeatureNotAvailable    = "FEATURE_NOT_AVAILABLE"
	ErrCodeInternal               = "INTERNAL_ERROR"
)

// MinTransferSen is the minimum transfer amount in sen (Rp 10).
const MinTransferSen int64 = 1000

// Promo-specific error codes.
const (
	ErrCodePromoInvalid = "INVALID_PROMO_CODE"
	ErrCodePromoExpired = "PROMO_CODE_EXPIRED"
)

// Pre-built domain errors with Indonesian messages.
var (
	ErrInvalidAmount = DomainError{
		Code:       ErrCodeInvalidAmount,
		Message:    "Jumlah tidak valid",
		HTTPStatus: 400,
	}
	ErrAmountBelowMinimum = DomainError{
		Code:       ErrCodeAmountBelowMinimum,
		Message:    "Jumlah minimal transfer Rp 10",
		HTTPStatus: 400,
	}
	ErrInsufficientBalance = DomainError{
		Code:       ErrCodeInsufficientBalance,
		Message:    "Saldo tidak cukup",
		HTTPStatus: 400,
	}
	ErrSelfTransfer = DomainError{
		Code:       ErrCodeSelfTransfer,
		Message:    "Tidak bisa transfer ke diri sendiri",
		HTTPStatus: 400,
	}
	ErrUserNotFound = DomainError{
		Code:       ErrCodeUserNotFound,
		Message:    "Pengguna tidak ditemukan",
		HTTPStatus: 404,
	}
	ErrPhoneAlreadyRegistered = DomainError{
		Code:       ErrCodePhoneAlreadyRegistered,
		Message:    "Nomor telepon sudah terdaftar",
		HTTPStatus: 409,
	}
	ErrInvalidPIN = DomainError{
		Code:       ErrCodeInvalidPIN,
		Message:    "PIN salah",
		HTTPStatus: 401,
	}
	ErrUnauthorized = DomainError{
		Code:       ErrCodeUnauthorized,
		Message:    "Sesi habis, silakan login ulang",
		HTTPStatus: 401,
	}
	ErrExceedsTransactionLimit = DomainError{
		Code:       ErrCodeExceedsTransactionLimit,
		Message:    "Melebihi batas transaksi",
		HTTPStatus: 400,
	}
	ErrRequestInFlight = DomainError{
		Code:       ErrCodeRequestInFlight,
		Message:    "Permintaan sedang diproses",
		HTTPStatus: 202,
	}
	ErrSerializationConflict = DomainError{
		Code:       ErrCodeSerializationConflict,
		Message:    "Silakan coba lagi",
		HTTPStatus: 409,
	}
	ErrFeatureNotAvailable = DomainError{
		Code:       ErrCodeFeatureNotAvailable,
		Message:    "Fitur belum tersedia",
		HTTPStatus: 501,
	}
	ErrInternal = DomainError{
		Code:       ErrCodeInternal,
		Message:    "Terjadi kesalahan internal",
		HTTPStatus: 500,
	}
)

// Promo-related pre-built domain errors.
var (
	// ErrPromoCodeInvalid is returned when a promo code has invalid format or is not recognized.
	ErrPromoCodeInvalid = DomainError{
		Code:       ErrCodePromoInvalid,
		Message:    "Kode promo tidak valid",
		HTTPStatus: 400,
	}

	// ErrPromoCodeExpired is returned when a promo code has expired (outside free transfer window).
	ErrPromoCodeExpired = DomainError{
		Code:       ErrCodePromoExpired,
		Message:    "Kode promo sudah kadaluarsa",
		HTTPStatus: 400,
	}
)

// NewMissingFieldError creates a DomainError for a missing required field.
func NewMissingFieldError(field string) DomainError {
	return DomainError{
		Code:       ErrCodeMissingField,
		Message:    fmt.Sprintf("Field %s wajib diisi", field),
		HTTPStatus: 400,
		Args:       []interface{}{field},
	}
}

// NewInvalidFormatError creates a DomainError for an invalid field format.
func NewInvalidFormatError(field, detail string) DomainError {
	return DomainError{
		Code:       ErrCodeInvalidFormat,
		Message:    fmt.Sprintf("Format %s tidak valid: %s", field, detail),
		HTTPStatus: 400,
		Args:       []interface{}{field, detail},
	}
}
