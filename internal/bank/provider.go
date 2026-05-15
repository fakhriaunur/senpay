package bank

import (
	"context"
	"time"

	"senpay/internal/types"

	"github.com/google/uuid"
)

// ────────────────────────────────────────────────────────────────
// PaymentRail Interface (Port)
// ────────────────────────────────────────────────────────────────

// CreditRequest represents a request to credit (top-up) a user's VA.
type CreditRequest struct {
	VANumber      string    `json:"va_number"`
	AmountSen     int64     `json:"amount_sen"`
	PartnerID     string    `json:"partner_id"`
	ExternalID    string    `json:"external_id"`
	TransactionID uuid.UUID `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
}

// CreditResult represents the result of a credit request to the bank.
type CreditResult struct {
	Success      bool   `json:"success"`
	ReferenceID  string `json:"reference_id,omitempty"`
	BankResponse string `json:"bank_response,omitempty"`
}

// ReversalRequest represents a request to reverse a previous credit/withdraw.
type ReversalRequest struct {
	ExternalID    string    `json:"external_id"`
	AmountSen     int64     `json:"amount_sen"`
	PartnerID     string    `json:"partner_id"`
	TransactionID uuid.UUID `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
}

// WithdrawRequest represents a withdraw request to the bank.
type WithdrawRequest struct {
	BankAccount   string    `json:"bank_account"`
	AmountSen     int64     `json:"amount_sen"`
	PartnerID     string    `json:"partner_id"`
	ExternalID    string    `json:"external_id"`
	TransactionID uuid.UUID `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
}

// BankCallback represents a webhook callback from the bank.
type BankCallback struct {
	VANumber    string               `json:"va_number"`
	AmountSen   int64                `json:"amount_sen"`
	ExternalID  string               `json:"external_id"`
	Status      types.CallbackStatus `json:"status"` // "success" or "failed"
	ReferenceID string               `json:"reference_id,omitempty"`
}

// PaymentRail defines the interface for bank adapter implementations.
//
// Implementations:
//   - provider_snap.go: Real SNAP adapter — signs requests with HMAC_SHA512,
//     sends HTTP requests to mock bank, validates responses.
//   - provider_stub.go: Stub adapter — returns canned responses without
//     network calls. Useful for testing and development.
type PaymentRail interface {
	// Credit sends a credit (top-up) request to the bank.
	// Returns the credit result or a DomainError.
	// DomainErrors: ErrTimeout, ErrBankRejection, ErrDuplicateRequest.
	Credit(ctx context.Context, req CreditRequest) (*CreditResult, *types.DomainError)

	// Withdraw sends a withdraw request to the bank.
	// Returns the withdraw result or a DomainError.
	// DomainErrors: ErrTimeout, ErrBankRejection, ErrInvalidVA.
	Withdraw(ctx context.Context, req WithdrawRequest) (*CreditResult, *types.DomainError)

	// Reversal sends a reversal request to undo a previous operation.
	// Returns the reversal result or a DomainError.
	// DomainErrors: ErrTimeout, ErrBankRejection.
	Reversal(ctx context.Context, req ReversalRequest) (*ReversalResult, *types.DomainError)

	// ParseWebhook parses a bank webhook callback from raw bytes.
	// Returns the parsed callback or a DomainError.
	ParseWebhook(body []byte) (*BankCallback, *types.DomainError)

	// Name returns the adapter name ("snap" or "stub").
	Name() types.BankProvider
}

// ReversalResult represents the result of a reversal request.
type ReversalResult struct {
	Success   bool   `json:"success"`
	ReferenceID string `json:"reference_id,omitempty"`
}
