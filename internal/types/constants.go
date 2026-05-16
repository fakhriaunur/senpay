// Package types provides domain types and shared constants for the Senpay application.
package types

// ────────────────────────────────────────────────────────────────
// Domain Constants
// ────────────────────────────────────────────────────────────────

const (
	// IDRDecimals is the number of decimal places for IDR currency.
	// 1 IDR = 100 sen (2 decimal places).
	IDRDecimals = 2

	// SenPerIDR is the number of sen in one IDR.
	SenPerIDR = 100

	// PINMinLength is the minimum PIN length for user registration.
	PINMinLength = 4

	// PhoneMinLength is the minimum length of an Indonesian phone number.
	PhoneMinLength = 10

	// PhoneMaxLength is the maximum length of an Indonesian phone number.
	PhoneMaxLength = 13

	// BcryptCost is the bcrypt hashing cost used for PIN hashing.
	BcryptCost = 12

	// PageDefaultLimit is the default number of items per page for paginated responses.
	PageDefaultLimit = 20

	// PageMaxLimit is the maximum number of items per page for paginated responses.
	PageMaxLimit = 100

	// MinWithdrawSen is the minimum withdraw amount in sen (Rp 100 = 10.000 sen).
	MinWithdrawSen int64 = 10_000

	// BudgetAlertPercent is the spending threshold percentage for budget alerts.
	BudgetAlertPercent = 80.0

	// PhonePrefix08 is the Indonesian mobile phone prefix.
	PhonePrefix08 = "08"

	// PhonePrefix62 is the Indonesian country code prefix.
	PhonePrefix62 = "62"

	// BILimitBasicSen is the per-transaction limit for basic KYC users in sen.
	// Rp 2.000.000 = 200,000,000 sen.
	BILimitBasicSen Money = 200_000_000

	// BILimitVerifiedSen is the per-transaction limit for verified KYC users in sen.
	// Rp 10.000.000 = 1,000,000,000 sen.
	BILimitVerifiedSen Money = 1_000_000_000
)

// ────────────────────────────────────────────────────────────────
// SQL Error Code Constants
// ────────────────────────────────────────────────────────────────

const (
	// SQLUniqueViolation is the PostgreSQL error code for unique constraint violations (23505).
	SQLUniqueViolation = "23505"

	// SQLSerializationError is the PostgreSQL error code for serialization failures (40001).
	SQLSerializationError = "40001"

	// SQLDeadlockError is the PostgreSQL error code for deadlock detection (40P01).
	SQLDeadlockError = "40P01"
)
