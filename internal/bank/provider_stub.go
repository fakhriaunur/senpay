package bank

import (
	"context"
	"encoding/json"
	"fmt"

	"senpay/internal/types"
)

// ────────────────────────────────────────────────────────────────
// Stub Adapter (Canned Responses)
// ────────────────────────────────────────────────────────────────

// StubAdapter implements PaymentRail with deterministic canned responses.
// No network calls are made. Useful for testing and development.
type StubAdapter struct {
	// creditResponse is the canned response for credit requests.
	creditResponse *CreditResult
	// withdrawResponse is the canned response for withdraw requests.
	withdrawResponse *CreditResult
	// reversalResponse is the canned response for reversal requests.
	reversalResponse *ReversalResult
	// alwaysFail, if true, makes all operations return ErrBankRejection.
	alwaysFail bool
}

// NewStubAdapter creates a new StubAdapter with default successful responses.
func NewStubAdapter() *StubAdapter {
	return &StubAdapter{
		creditResponse: &CreditResult{
			Success:      true,
			ReferenceID:  "STUB-REF-001",
			BankResponse: "stub: credit approved",
		},
		withdrawResponse: &CreditResult{
			Success:      true,
			ReferenceID:  "STUB-REF-002",
			BankResponse: "stub: withdraw approved",
		},
		reversalResponse: &ReversalResult{
			Success: true,
		},
	}
}

// Name returns the adapter name.
func (s *StubAdapter) Name() string { return "stub" }

// Credit returns a canned credit response without any network calls.
func (s *StubAdapter) Credit(_ context.Context, req CreditRequest) (*CreditResult, *types.DomainError) {
	if s.alwaysFail {
		return nil, &ErrBankRejection
	}

	// Clone the canned response with a reference ID that includes the external ID.
	return &CreditResult{
		Success:      s.creditResponse.Success,
		ReferenceID:  fmt.Sprintf("STUB-CREDIT-%s", req.ExternalID),
		BankResponse: s.creditResponse.BankResponse,
	}, nil
}

// Withdraw returns a canned withdraw response without any network calls.
func (s *StubAdapter) Withdraw(_ context.Context, req WithdrawRequest) (*CreditResult, *types.DomainError) {
	if s.alwaysFail {
		return nil, &ErrBankRejection
	}

	return &CreditResult{
		Success:      s.withdrawResponse.Success,
		ReferenceID:  fmt.Sprintf("STUB-WITHDRAW-%s", req.ExternalID),
		BankResponse: s.withdrawResponse.BankResponse,
	}, nil
}

// Reversal returns a canned reversal response without any network calls.
func (s *StubAdapter) Reversal(_ context.Context, req ReversalRequest) (*ReversalResult, *types.DomainError) {
	if s.alwaysFail {
		return nil, &ErrBankRejection
	}

	return &ReversalResult{
		Success:     s.reversalResponse.Success,
		ReferenceID: fmt.Sprintf("STUB-REVERSAL-%s", req.ExternalID),
	}, nil
}

// ParseWebhook parses a bank webhook callback from raw bytes.
func (s *StubAdapter) ParseWebhook(body []byte) (*BankCallback, *types.DomainError) {
	var callback BankCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		return nil, &types.ErrInternal
	}
	if callback.VANumber == "" {
		err := types.NewMissingFieldError("va_number")
		return nil, &err
	}
	return &callback, nil
}

// SetAlwaysFail configures the stub to always return ErrBankRejection.
func (s *StubAdapter) SetAlwaysFail(fail bool) {
	s.alwaysFail = fail
}

// ensure interfaces are satisfied
var _ PaymentRail = (*StubAdapter)(nil)
