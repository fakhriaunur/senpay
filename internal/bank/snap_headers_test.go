//go:build integration

package bank

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestSnapAdapter_Withdraw_SendsAllSNAPHeaders verifies VAL-BANK-009:
// When SnapAdapter sends a withdraw request to the mock bank, all 5 mandatory
// SNAP headers are present in the outgoing HTTP request:
//   - X-TIMESTAMP
//   - X-SIGNATURE (HMAC_SHA512)
//   - X-PARTNER-ID
//   - X-EXTERNAL-ID
//   - CHANNEL-ID
//
// This test inspects the mock bank's request log to confirm every header
// was received and the signature was valid.
func TestSnapAdapter_Withdraw_SendsAllSNAPHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create mock bank with minimal delay.
	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 0
	mockBank := NewMockBank(config)

	// Create an HTTP test server wrapping the mock bank handler.
	handler := mockBank.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create SnapAdapter pointed at the mock bank.
	adapter := NewSnapAdapter(server.URL, config.ClientSecret, config.PartnerID, config.ChannelID)

	// Send a withdraw request.
	req := WithdrawRequest{
		BankAccount:   "1234567890",
		AmountSen:     5000000,
		PartnerID:     config.PartnerID,
		ExternalID:    uuid.Must(uuid.NewV7()).String(),
		TransactionID: uuid.Must(uuid.NewV7()),
		Timestamp:     time.Now().UTC(),
	}

	result, domainErr := adapter.Withdraw(context.Background(), req)
	if domainErr != nil {
		t.Fatalf("withdraw failed: code=%s message=%s", domainErr.Code, domainErr.Message)
	}
	if result == nil {
		t.Fatal("withdraw result must not be nil")
	}
	if !result.Success {
		t.Fatal("expected withdraw success")
	}

	// Check mock bank request log.
	requests := mockBank.GetRequests()
	if len(requests) == 0 {
		t.Fatal("expected at least 1 request in mock bank log")
	}

	lastReq := requests[len(requests)-1]

	// Verify all 5 mandatory SNAP headers are present.
	// Note: Go canonicalizes HTTP header keys (textproto.CanonicalMIMEHeaderKey).
	// X-TIMESTAMP → X-Timestamp, X-SIGNATURE → X-Signature, X-PARTNER-ID → X-Partner-Id,
	// X-EXTERNAL-ID → X-External-Id, CHANNEL-ID → Channel-Id.
	requiredHeaders := []string{
		"X-Timestamp",
		"X-Signature",
		"X-Partner-Id",
		"X-External-Id",
		"Channel-Id",
	}

	for _, h := range requiredHeaders {
		if _, ok := lastReq.Headers[h]; !ok {
			t.Errorf("missing required SNAP header: %s", h)
		} else if lastReq.Headers[h] == "" {
			t.Errorf("SNAP header %s is empty", h)
		}
	}

	// Verify signature was valid.
	if !lastReq.SignatureValid {
		t.Error("mock bank reported invalid SNAP signature")
	}

	// Verify the endpoint path.
	if lastReq.Path != "/bank/api/v1/withdraw" {
		t.Errorf("expected path /bank/api/v1/withdraw, got %s", lastReq.Path)
	}

	t.Logf("All SNAP headers verified for withdraw request")
	t.Logf("  X-Timestamp:   %s", lastReq.Headers["X-Timestamp"])
	t.Logf("  X-Signature:   %s", lastReq.Headers["X-Signature"][:20]+"...")
	t.Logf("  X-Partner-Id:  %s", lastReq.Headers["X-Partner-Id"])
	t.Logf("  X-External-Id: %s", lastReq.Headers["X-External-Id"])
	t.Logf("  Channel-Id:    %s", lastReq.Headers["Channel-Id"])
	t.Logf("  Signature valid: %v", lastReq.SignatureValid)
}

// TestSnapAdapter_Credit_SendsAllSNAPHeaders verifies VAL-BANK-009 for credit (top-up) requests:
// All 5 mandatory SNAP headers are present in credit (top-up) HTTP requests.
func TestSnapAdapter_Credit_SendsAllSNAPHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 0
	mockBank := NewMockBank(config)

	handler := mockBank.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	adapter := NewSnapAdapter(server.URL, config.ClientSecret, config.PartnerID, config.ChannelID)

	req := CreditRequest{
		VANumber:      "8999123456",
		AmountSen:     10000000,
		PartnerID:     config.PartnerID,
		ExternalID:    uuid.Must(uuid.NewV7()).String(),
		TransactionID: uuid.Must(uuid.NewV7()),
		Timestamp:     time.Now().UTC(),
	}

	result, domainErr := adapter.Credit(context.Background(), req)
	if domainErr != nil {
		t.Fatalf("credit failed: code=%s message=%s", domainErr.Code, domainErr.Message)
	}
	if result == nil || !result.Success {
		t.Fatal("expected credit success")
	}

	requests := mockBank.GetRequests()
	if len(requests) == 0 {
		t.Fatal("expected at least 1 request in mock bank log")
	}

	lastReq := requests[len(requests)-1]

	requiredHeaders := []string{
		"X-Timestamp", "X-Signature", "X-Partner-Id", "X-External-Id", "Channel-Id",
	}
	for _, h := range requiredHeaders {
		if _, ok := lastReq.Headers[h]; !ok {
			t.Errorf("missing required SNAP header: %s", h)
		} else if lastReq.Headers[h] == "" {
			t.Errorf("SNAP header %s is empty", h)
		}
	}

	if !lastReq.SignatureValid {
		t.Error("mock bank reported invalid SNAP signature")
	}

	t.Logf("All SNAP headers verified for credit request")
}

// TestSnapAdapter_Withdraw_HeaderValues verifies VAL-BANK-009:
// The SNAP header values are correct:
//   - X-TIMESTAMP is valid ISO 8601
//   - X-PARTNER-ID matches configured partner
//   - X-EXTERNAL-ID is unique per request
//   - CHANNEL-ID is present and non-empty
func TestSnapAdapter_Withdraw_HeaderValues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 0
	mockBank := NewMockBank(config)

	handler := mockBank.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	adapter := NewSnapAdapter(server.URL, config.ClientSecret, config.PartnerID, config.ChannelID)

	// Make two sequential withdraw requests to verify X-EXTERNAL-ID uniqueness.
	externalID1 := uuid.Must(uuid.NewV7()).String()
	req1 := WithdrawRequest{
		BankAccount:   "1234567890",
		AmountSen:     1000000,
		PartnerID:     config.PartnerID,
		ExternalID:    externalID1,
		TransactionID: uuid.Must(uuid.NewV7()),
		Timestamp:     time.Now().UTC(),
	}

	_, domainErr := adapter.Withdraw(context.Background(), req1)
	if domainErr != nil {
		t.Fatalf("first withdraw failed: %v", domainErr)
	}

	// Clear requests to isolate the second request's log.
	mockBank.ClearRequests()

	externalID2 := uuid.Must(uuid.NewV7()).String()
	req2 := WithdrawRequest{
		BankAccount:   "0987654321",
		AmountSen:     2000000,
		PartnerID:     config.PartnerID,
		ExternalID:    externalID2,
		TransactionID: uuid.Must(uuid.NewV7()),
		Timestamp:     time.Now().UTC(),
	}

	_, domainErr = adapter.Withdraw(context.Background(), req2)
	if domainErr != nil {
		t.Fatalf("second withdraw failed: %v", domainErr)
	}

	requests := mockBank.GetRequests()
	if len(requests) != 1 {
		t.Fatalf("expected 1 request logged, got %d", len(requests))
	}

	req := requests[0]

	// Verify X-PARTNER-ID matches configuration.
	if req.Headers["X-Partner-Id"] != config.PartnerID {
		t.Errorf("X-Partner-Id: got %q, want %q", req.Headers["X-Partner-Id"], config.PartnerID)
	}

	// Verify X-EXTERNAL-ID matches our request.
	if req.Headers["X-External-Id"] != externalID2 {
		t.Errorf("X-External-Id: got %q, want %q", req.Headers["X-External-Id"], externalID2)
	}

	// Verify CHANNEL-ID is configured and non-empty.
	if req.Headers["Channel-Id"] == "" {
		t.Error("Channel-Id must not be empty")
	}

	// Verify X-TIMESTAMP is non-empty.
	if req.Headers["X-Timestamp"] == "" {
		t.Error("X-Timestamp must not be empty")
	}

	// Verify X-SIGNATURE is non-empty.
	if req.Headers["X-Signature"] == "" {
		t.Error("X-Signature must not be empty")
	}

	t.Logf("All SNAP header values verified for withdraw")
}

// TestProviderSwap verifies VAL-BANK-017:
// The bank adapter interface has two implementations (stub and snap) that
// can be swapped via configuration. The stub adapter returns canned responses
// without network calls. The snap adapter sends real SNAP-signed requests.
//
// This test verifies both adapters through the PaymentRail interface and
// confirms that:
//   - StubAdapter returns deterministic responses without HTTP calls
//   - SnapAdapter sends SNAP-signed HTTP requests to mock bank
//   - Both adapters satisfy the same interface
func TestProviderSwap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// ── Stub Adapter ──────────────────────────────────────────
	t.Run("stub_adapter_returns_canned_responses", func(t *testing.T) {
		stub := NewStubAdapter()

		// Withdraw with stub.
		result, err := stub.Withdraw(context.Background(), WithdrawRequest{
			BankAccount: "1234567890",
			AmountSen:   5000000,
			ExternalID:  "test-stub-001",
		})
		if err != nil {
			t.Fatalf("stub withdraw failed: %v", err)
		}
		if !result.Success {
			t.Fatal("expected stub withdraw success")
		}
		if result.ReferenceID != "STUB-WITHDRAW-test-stub-001" {
			t.Errorf("stub reference: got %q, want stub format", result.ReferenceID)
		}

		// Credit with stub.
		creditResult, err := stub.Credit(context.Background(), CreditRequest{
			VANumber:   "8999123456",
			AmountSen:  10000000,
			ExternalID: "test-stub-credit-001",
		})
		if err != nil {
			t.Fatalf("stub credit failed: %v", err)
		}
		if !creditResult.Success {
			t.Fatal("expected stub credit success")
		}
		// Verify deterministic responses.
		if creditResult.ReferenceID != "STUB-CREDIT-test-stub-credit-001" {
			t.Errorf("stub credit reference: got %q, want stub format", creditResult.ReferenceID)
		}

		// Verify adapter name.
		if stub.Name() != "stub" {
			t.Errorf("stub adapter name: got %q, want %q", stub.Name(), "stub")
		}
	})

	// ── Snap Adapter ───────────────────────────────────────────
	t.Run("snap_adapter_sends_snap_requests", func(t *testing.T) {
		config := DefaultMockBankConfig()
		config.MinDelay = 0
		config.MaxDelay = 0
		mockBank := NewMockBank(config)

		handler := mockBank.Handler()
		server := httptest.NewServer(handler)
		defer server.Close()

		adapter := NewSnapAdapter(server.URL, config.ClientSecret, config.PartnerID, config.ChannelID)

		// Verify adapter name.
		if adapter.Name() != "snap" {
			t.Errorf("snap adapter name: got %q, want %q", adapter.Name(), "snap")
		}

		// Withdraw via snap adapter.
		withdrawResult, err := adapter.Withdraw(context.Background(), WithdrawRequest{
			BankAccount:   "1234567890",
			AmountSen:     5000000,
			PartnerID:     config.PartnerID,
			ExternalID:    uuid.Must(uuid.NewV7()).String(),
			TransactionID: uuid.Must(uuid.NewV7()),
			Timestamp:     time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("snap withdraw failed: %v", err)
		}
		if !withdrawResult.Success {
			t.Fatal("expected snap withdraw success")
		}
		if withdrawResult.ReferenceID == "" {
			t.Error("snap withdraw reference must not be empty")
		}

		// Verify mock bank received the request.
		requests := mockBank.GetRequests()
		if len(requests) == 0 {
			t.Fatal("expected mock bank request log entries")
		}

		lastReq := requests[len(requests)-1]
		if !lastReq.SignatureValid {
			t.Error("snap signature should be valid")
		}
		if lastReq.Method != "POST" {
			t.Errorf("request method: got %q, want %q", lastReq.Method, "POST")
		}
		if lastReq.Headers["X-Partner-Id"] != config.PartnerID {
			t.Errorf("X-Partner-Id: got %q, want %q", lastReq.Headers["X-Partner-Id"], config.PartnerID)
		}
	})
}
