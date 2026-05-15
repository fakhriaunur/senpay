package types

import "fmt"

// ────────────────────────────────────────────────────────────────
// VAStatus — Virtual Account lifecycle status
// ────────────────────────────────────────────────────────────────

// VAStatus represents the lifecycle status of a Virtual Account.
// Valid values: active, paid, expired.
type VAStatus string

const (
	VAStatusActive  VAStatus = "active"
	VAStatusPaid    VAStatus = "paid"
	VAStatusExpired VAStatus = "expired"
)

// ParseVAStatus parses a string into a VAStatus.
// Returns DomainError with code ErrCodeInvalidFormat for unknown values.
func ParseVAStatus(s string) (VAStatus, error) {
	switch s {
	case string(VAStatusActive):
		return VAStatusActive, nil
	case string(VAStatusPaid):
		return VAStatusPaid, nil
	case string(VAStatusExpired):
		return VAStatusExpired, nil
	default:
		return "", NewInvalidFormatError("VA status", fmt.Sprintf("unknown status: %q", s))
	}
}

// String returns the string representation of VAStatus.
func (s VAStatus) String() string {
	return string(s)
}

// ────────────────────────────────────────────────────────────────
// CallbackStatus — Bank webhook callback status
// ────────────────────────────────────────────────────────────────

// CallbackStatus represents the status of a bank webhook callback.
// Valid values: success, failed.
type CallbackStatus string

const (
	CallbackSuccess CallbackStatus = "success"
	CallbackFailed  CallbackStatus = "failed"
)

// ParseCallbackStatus parses a string into a CallbackStatus.
// Returns DomainError with code ErrCodeInvalidFormat for unknown values.
func ParseCallbackStatus(s string) (CallbackStatus, error) {
	switch s {
	case string(CallbackSuccess):
		return CallbackSuccess, nil
	case string(CallbackFailed):
		return CallbackFailed, nil
	default:
		return "", NewInvalidFormatError("callback status", fmt.Sprintf("unknown status: %q", s))
	}
}

// String returns the string representation of CallbackStatus.
func (s CallbackStatus) String() string {
	return string(s)
}

// ────────────────────────────────────────────────────────────────
// BankProvider — Bank adapter provider
// ────────────────────────────────────────────────────────────────

// BankProvider represents the bank adapter implementation.
// Valid values: stub, snap.
type BankProvider string

const (
	BankProviderStub BankProvider = "stub"
	BankProviderSnap BankProvider = "snap"
)

// ParseBankProvider parses a string into a BankProvider.
// Returns DomainError with code ErrCodeInvalidFormat for unknown values.
func ParseBankProvider(s string) (BankProvider, error) {
	switch s {
	case string(BankProviderStub):
		return BankProviderStub, nil
	case string(BankProviderSnap):
		return BankProviderSnap, nil
	default:
		return "", NewInvalidFormatError("bank provider", fmt.Sprintf("unknown provider: %q", s))
	}
}

// String returns the string representation of BankProvider.
func (p BankProvider) String() string {
	return string(p)
}

// ────────────────────────────────────────────────────────────────
// EntryType — Ledger entry type (debit/credit)
// ────────────────────────────────────────────────────────────────

// EntryType represents the type of a ledger entry.
// Valid values: debit, credit.
type EntryType string

const (
	EntryTypeDebit  EntryType = "debit"
	EntryTypeCredit EntryType = "credit"
)

// ParseEntryType parses a string into an EntryType.
// Returns DomainError with code ErrCodeInvalidFormat for unknown values.
func ParseEntryType(s string) (EntryType, error) {
	switch s {
	case string(EntryTypeDebit):
		return EntryTypeDebit, nil
	case string(EntryTypeCredit):
		return EntryTypeCredit, nil
	default:
		return "", NewInvalidFormatError("entry type", fmt.Sprintf("unknown type: %q", s))
	}
}

// String returns the string representation of EntryType.
func (t EntryType) String() string {
	return string(t)
}
