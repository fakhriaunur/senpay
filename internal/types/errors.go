package types

import (
	"fmt"
)

// DomainError represents a typed application error with Indonesian error taxonomy.
// Implements the error interface for use in Go error handling.
type DomainError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
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
	ErrCodeRequestInFlight        = "REQUEST_IN_FLIGHT"
	ErrCodeSerializationConflict  = "SERIALIZATION_CONFLICT"
	ErrCodeMissingField           = "MISSING_FIELD"
	ErrCodeInvalidFormat          = "INVALID_FORMAT"
	ErrCodeDuplicateTransaction   = "DUPLICATE_TRANSACTION"
	ErrCodeFeatureNotAvailable    = "FEATURE_NOT_AVAILABLE"
	ErrCodeInternal               = "INTERNAL_ERROR"
)

// Pre-built domain errors with Indonesian messages.
var (
	ErrInvalidAmount = DomainError{
		Code:       ErrCodeInvalidAmount,
		Message:    "Jumlah tidak valid",
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

// NewMissingFieldError creates a DomainError for a missing required field.
func NewMissingFieldError(field string) DomainError {
	return DomainError{
		Code:       ErrCodeMissingField,
		Message:    fmt.Sprintf("Field %s wajib diisi", field),
		HTTPStatus: 400,
	}
}

// NewInvalidFormatError creates a DomainError for an invalid field format.
func NewInvalidFormatError(field, detail string) DomainError {
	return DomainError{
		Code:       ErrCodeInvalidFormat,
		Message:    fmt.Sprintf("Format %s tidak valid: %s", field, detail),
		HTTPStatus: 400,
	}
}
