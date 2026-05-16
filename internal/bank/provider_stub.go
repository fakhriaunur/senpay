package bank

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"senpay/internal/types"
)

// StubTimeoutDelay is the simulated timeout delay for stub adapter (31 seconds).
const StubTimeoutDelay = 31 * time.Second

// StubSlowDelay is the simulated slow response delay for stub adapter (25 seconds).
const StubSlowDelay = 25 * time.Second

// StubBehavior defines the simulated behavior of the stub adapter.
type StubBehavior string

const (
	// StubBehaviorSuccess returns a successful response immediately.
	StubBehaviorSuccess StubBehavior = "success"
	// StubBehaviorRejection returns ErrBankRejection with HTTP 422 semantics.
	StubBehaviorRejection StubBehavior = "rejection"
	// StubBehaviorTimeout simulates a timeout by sleeping beyond context deadline.
	StubBehaviorTimeout StubBehavior = "timeout"
	// StubBehaviorSlow responds after 25 seconds (used to test timeout handling).
	StubBehaviorSlow StubBehavior = "slow"
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
	// behavior controls the simulated behavior for withdraw operations.
	behavior StubBehavior
	// behavior for credit operations.
	creditBehavior StubBehavior
	// behavior for reversal operations.
	reversalBehavior StubBehavior
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
		behavior:         StubBehaviorSuccess,
		creditBehavior:   StubBehaviorSuccess,
		reversalBehavior: StubBehaviorSuccess,
	}
}

// Name returns the adapter name.
func (s *StubAdapter) Name() types.BankProvider { return types.BankProviderStub }

// SetBehavior configures the simulated behavior for withdraw operations.
func (s *StubAdapter) SetBehavior(b StubBehavior) {
	s.behavior = b
}

// SetCreditBehavior configures the simulated behavior for credit operations.
func (s *StubAdapter) SetCreditBehavior(b StubBehavior) {
	s.creditBehavior = b
}

// SetReversalBehavior configures the simulated behavior for reversal operations.
func (s *StubAdapter) SetReversalBehavior(b StubBehavior) {
	s.reversalBehavior = b
}

// simulateBehavior applies the configured behavior: delay or error.
func (s *StubAdapter) simulateBehavior(ctx context.Context, behavior StubBehavior) *types.DomainError {
	switch behavior {
	case StubBehaviorSuccess:
		return nil
	case StubBehaviorRejection:
		return &ErrBankRejection
	case StubBehaviorTimeout:
		// Sleep until context cancels (timeout).
		select {
		case <-ctx.Done():
			return &ErrTimeout
		case <-time.After(StubTimeoutDelay):
			// Shouldn't reach here if context has timeout.
			return &ErrTimeout
		}
	case StubBehaviorSlow:
		// Sleep for 25 seconds, then return success if context hasn't expired.
		select {
		case <-ctx.Done():
			return &ErrTimeout
		case <-time.After(StubSlowDelay):
			return nil
		}
	default:
		return nil
	}
}

// Credit returns a canned credit response without any network calls.
func (s *StubAdapter) Credit(ctx context.Context, req CreditRequest) (*CreditResult, *types.DomainError) {
	if s.alwaysFail {
		return nil, &ErrBankRejection
	}

	if domainErr := s.simulateBehavior(ctx, s.creditBehavior); domainErr != nil {
		return nil, domainErr
	}

	// Clone the canned response with a reference ID that includes the external ID.
	return &CreditResult{
		Success:      s.creditResponse.Success,
		ReferenceID:  fmt.Sprintf("STUB-CREDIT-%s", req.ExternalID),
		BankResponse: s.creditResponse.BankResponse,
	}, nil
}

// Withdraw returns a canned withdraw response without any network calls.
func (s *StubAdapter) Withdraw(ctx context.Context, req WithdrawRequest) (*CreditResult, *types.DomainError) {
	if s.alwaysFail {
		return nil, &ErrBankRejection
	}

	if domainErr := s.simulateBehavior(ctx, s.behavior); domainErr != nil {
		return nil, domainErr
	}

	return &CreditResult{
		Success:      s.withdrawResponse.Success,
		ReferenceID:  fmt.Sprintf("STUB-WITHDRAW-%s", req.ExternalID),
		BankResponse: s.withdrawResponse.BankResponse,
	}, nil
}

// Reversal returns a canned reversal response without any network calls.
func (s *StubAdapter) Reversal(ctx context.Context, req ReversalRequest) (*ReversalResult, *types.DomainError) {
	if s.alwaysFail {
		return nil, &ErrBankRejection
	}

	if domainErr := s.simulateBehavior(ctx, s.reversalBehavior); domainErr != nil {
		return nil, domainErr
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
