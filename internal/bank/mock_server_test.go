package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMockBank_Health(t *testing.T) {
	t.Parallel()

	mockBank := NewMockBank(DefaultMockBankConfig())
	handler := mockBank.Handler()

	req := httptest.NewRequest(http.MethodGet, "/bank/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %q", resp["status"])
	}
}

func TestMockBank_Credit_ValidSNAP(t *testing.T) {
	t.Parallel()

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 10 * time.Millisecond // minimal delay for tests

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	// Build a valid SNAP request.
	creditReq := CreditRequest{
		VANumber:  "8999123456",
		AmountSen: 10000000,
		PartnerID: config.PartnerID,
		ExternalID: "ext-001",
	}
	bodyBytes, _ := json.Marshal(creditReq)
	timestamp := "2026-05-15T10:00:00Z"

	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/credit",
		Body:        string(bodyBytes),
		Timestamp:   timestamp,
	}
	signature := Sign(snapReq, config.ClientSecret)

	httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/credit",
		bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", signature)
	httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", "ext-001")
	httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var result CreditResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
}

func TestMockBank_Credit_InvalidSignature(t *testing.T) {
	t.Parallel()

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 0

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	creditReq := CreditRequest{
		VANumber:  "8999123456",
		AmountSen: 10000000,
	}
	bodyBytes, _ := json.Marshal(creditReq)

	httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/credit",
		bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-TIMESTAMP", "2026-05-15T10:00:00Z")
	httpReq.Header.Set("X-SIGNATURE", "invalid-signature")
	httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", "ext-002")
	httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var errResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error: %v", err)
	}

	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["code"] != "INVALID_SIGNATURE" {
		t.Errorf("expected INVALID_SIGNATURE, got %q", errObj["code"])
	}
}

func TestMockBank_Credit_MissingHeaders(t *testing.T) {
	t.Parallel()

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 0

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	// Helper to test a request missing a specific header.
	testMissingHeader := func(t *testing.T, missingHeader string) {
		t.Helper()

		bodyBytes, _ := json.Marshal(CreditRequest{
			VANumber:  "8999123456",
			AmountSen: 5000000,
		})

		httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/credit",
			bytes.NewReader(bodyBytes))

		// Set all headers except the one we want to test.
		httpReq.Header.Set("X-TIMESTAMP", "2026-05-15T10:00:00Z")
		httpReq.Header.Set("X-SIGNATURE", "test-sig")
		httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
		httpReq.Header.Set("X-EXTERNAL-ID", "ext-003")
		httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

		// Remove the header being tested.
		httpReq.Header.Del(missingHeader)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httpReq)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("missing %s: expected 400, got %d", missingHeader, rec.Code)
		}
	}

	headers := []string{"X-TIMESTAMP", "X-SIGNATURE", "X-PARTNER-ID", "X-EXTERNAL-ID", "CHANNEL-ID"}
	for _, h := range headers {
		t.Run("missing_"+h, func(t *testing.T) {
			testMissingHeader(t, h)
		})
	}
}

func TestMockBank_Withdraw_ValidSNAP(t *testing.T) {
	t.Parallel()

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 10 * time.Millisecond

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	withdrawReq := WithdrawRequest{
		BankAccount: "1234567890",
		AmountSen:   5000000,
		PartnerID:   config.PartnerID,
		ExternalID:  "ext-wd-001",
	}
	bodyBytes, _ := json.Marshal(withdrawReq)
	timestamp := "2026-05-15T10:00:00Z"

	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/withdraw",
		Body:        string(bodyBytes),
		Timestamp:   timestamp,
	}
	signature := Sign(snapReq, config.ClientSecret)

	httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/withdraw",
		bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", signature)
	httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", "ext-wd-001")
	httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var result CreditResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
}

func TestMockBank_Reversal_ValidSNAP(t *testing.T) {
	t.Parallel()

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 10 * time.Millisecond

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	revReq := ReversalRequest{
		ExternalID: "ext-rev-001",
		AmountSen:  5000000,
		PartnerID:  config.PartnerID,
	}
	bodyBytes, _ := json.Marshal(revReq)
	timestamp := "2026-05-15T10:00:00Z"

	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/reversal",
		Body:        string(bodyBytes),
		Timestamp:   timestamp,
	}
	signature := Sign(snapReq, config.ClientSecret)

	httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/reversal",
		bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", signature)
	httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", "ext-rev-001")
	httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var result ReversalResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
}

func TestMockBank_RequestLogging(t *testing.T) {
	t.Parallel()

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 0

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	// Make a valid credit request.
	creditReq := CreditRequest{
		VANumber:  "8999123456",
		AmountSen: 10000000,
	}
	bodyBytes, _ := json.Marshal(creditReq)
	timestamp := "2026-05-15T10:00:00Z"
	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/credit",
		Body:        string(bodyBytes),
		Timestamp:   timestamp,
	}
	sig := Sign(snapReq, config.ClientSecret)

	httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/credit",
		bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", sig)
	httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", "ext-log-001")
	httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	// Also make an invalid request (no signature).
	httpReq2 := httptest.NewRequest(http.MethodPost, "/bank/api/v1/credit",
		bytes.NewReader(bodyBytes))
	httpReq2.Header.Set("X-TIMESTAMP", timestamp)
	httpReq2.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq2.Header.Set("X-EXTERNAL-ID", "ext-log-002")
	// Deliberately missing X-SIGNATURE and CHANNEL-ID

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httpReq2)

	// Check request log.
	requests := mockBank.GetRequests()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests logged, got %d", len(requests))
	}

	if !requests[0].SignatureValid {
		t.Error("first request should have valid signature")
	}
	if requests[1].SignatureValid {
		t.Error("second request (missing headers) should have invalid signature")
	}
}

func TestMockBank_AlwaysTimeout(t *testing.T) {
	t.Parallel()

	config := DefaultMockBankConfig()
	config.AlwaysTimeout = true
	config.MinDelay = 100 * time.Millisecond // should not matter

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	// Build a valid SNAP request.
	bodyBytes, _ := json.Marshal(CreditRequest{
		VANumber:  "8999123456",
		AmountSen: 10000000,
	})
	timestamp := "2026-05-15T10:00:00Z"

	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/credit",
		Body:        string(bodyBytes),
		Timestamp:   timestamp,
	}
	sig := Sign(snapReq, config.ClientSecret)

	httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/credit",
		bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", sig)
	httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", "ext-timeout-001")
	httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

	rec := httptest.NewRecorder()

	// This should not take long (AlwaysTimeout means no delay).
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, httpReq)
		close(done)
	}()

	select {
	case <-done:
		// Should have responded quickly.
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("request took too long despite AlwaysTimeout=false")
	}
}

func TestMockBank_WebhookTrigger(t *testing.T) {
	t.Parallel()

	// Create a test server to receive the webhook callback.
	webhookReceived := make(chan BankCallback, 1)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cb BankCallback
		if err := json.NewDecoder(r.Body).Decode(&cb); err != nil {
			t.Errorf("failed to parse webhook body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		webhookReceived <- cb
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	config := DefaultMockBankConfig()
	config.MinDelay = 0
	config.MaxDelay = 0
	config.WebhookURL = webhookServer.URL

	mockBank := NewMockBank(config)
	handler := mockBank.Handler()

	// Send a valid credit request.
	creditReq := CreditRequest{
		VANumber:  "8999555555",
		AmountSen: 25000000,
		PartnerID: config.PartnerID,
		ExternalID: "ext-webhook-001",
	}
	bodyBytes, _ := json.Marshal(creditReq)
	timestamp := "2026-05-15T10:00:00Z"

	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/credit",
		Body:        string(bodyBytes),
		Timestamp:   timestamp,
	}
	sig := Sign(snapReq, config.ClientSecret)

	httpReq := httptest.NewRequest(http.MethodPost, "/bank/api/v1/credit",
		bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", sig)
	httpReq.Header.Set("X-PARTNER-ID", config.PartnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", "ext-webhook-001")
	httpReq.Header.Set("CHANNEL-ID", config.ChannelID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Wait for webhook callback.
	select {
	case cb := <-webhookReceived:
		if cb.VANumber != "8999555555" {
			t.Errorf("webhook VA: got %q, want %q", cb.VANumber, "8999555555")
		}
		if cb.AmountSen != 25000000 {
			t.Errorf("webhook amount: got %d, want %d", cb.AmountSen, 25000000)
		}
		if cb.Status != "success" {
			t.Errorf("webhook status: got %q, want %q", cb.Status, "success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for webhook callback")
	}
}

func TestStubAdapter_Credit(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()

	result, err := stub.Credit(nil, CreditRequest{
		VANumber:   "8999123456",
		AmountSen:  10000000,
		ExternalID: "stub-test-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.ReferenceID != "STUB-CREDIT-stub-test-001" {
		t.Errorf("reference: got %q, expected stub format", result.ReferenceID)
	}
}

func TestStubAdapter_AlwaysFail(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()
	stub.SetAlwaysFail(true)

	_, err := stub.Credit(nil, CreditRequest{
		VANumber:  "8999123456",
		AmountSen: 10000000,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != ErrBankRejection.Code {
		t.Errorf("code: got %q, want %q", err.Code, ErrBankRejection.Code)
	}

	_, err = stub.Withdraw(nil, WithdrawRequest{
		BankAccount: "1234567890",
		AmountSen:   5000000,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Code != ErrBankRejection.Code {
		t.Errorf("code: got %q, want %q", err.Code, ErrBankRejection.Code)
	}
}

func TestStubAdapter_Withdraw(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()

	result, err := stub.Withdraw(nil, WithdrawRequest{
		BankAccount: "1234567890",
		AmountSen:   5000000,
		ExternalID:  "stub-wd-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
}

func TestSnapAdapter_ParseWebhook(t *testing.T) {
	t.Parallel()

	adapter := NewSnapAdapter("http://localhost:9999", "secret", "test", "test")

	validBody := `{"va_number":"8999123456","amount_sen":10000000,"external_id":"ext-001","status":"success"}`
	callback, err := adapter.ParseWebhook([]byte(validBody))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callback.VANumber != "8999123456" {
		t.Errorf("VA: got %q, want %q", callback.VANumber, "8999123456")
	}
	if callback.AmountSen != 10000000 {
		t.Errorf("amount: got %d, want %d", callback.AmountSen, 10000000)
	}

	// Invalid body.
	_, err = adapter.ParseWebhook([]byte("not-json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	// Missing va_number.
	_, err = adapter.ParseWebhook([]byte(`{"status":"success"}`))
	if err == nil {
		t.Error("expected error for missing va_number")
	}
}

// ── Stub Adapter Behavior Tests ────────────────────────────────

func TestStubAdapter_Behavior_Success(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()
	stub.SetBehavior(StubBehaviorSuccess)

	result, err := stub.Withdraw(context.Background(), WithdrawRequest{
		BankAccount: "1234567890",
		AmountSen:   5000000,
		ExternalID:  "test-success",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.ReferenceID != "STUB-WITHDRAW-test-success" {
		t.Errorf("reference: got %q, wanted stub format", result.ReferenceID)
	}
}

func TestStubAdapter_Behavior_Rejection(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()
	stub.SetBehavior(StubBehaviorRejection)

	_, err := stub.Withdraw(context.Background(), WithdrawRequest{
		BankAccount: "1234567890",
		AmountSen:   5000000,
		ExternalID:  "test-rejection",
	})
	if err == nil {
		t.Fatal("expected error for rejection behavior")
	}
	if err.Code != ErrBankRejection.Code {
		t.Errorf("code: got %q, want %q", err.Code, ErrBankRejection.Code)
	}
	// Verify the error has Indonesian message.
	if err.Message == "" {
		t.Error("error message must not be empty")
	}
}

func TestStubAdapter_Behavior_Timeout(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()
	stub.SetBehavior(StubBehaviorTimeout)

	// Create a context with a short timeout so the stub's sleep is cut short.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := stub.Withdraw(ctx, WithdrawRequest{
		BankAccount: "1234567890",
		AmountSen:   5000000,
		ExternalID:  "test-timeout",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if err.Code != ErrTimeout.Code {
		t.Errorf("code: got %q, want %q", err.Code, ErrTimeout.Code)
	}
	// Verify the error has Indonesian message.
	if err.Message == "" {
		t.Error("error message must not be empty")
	}
}

func TestStubAdapter_CreditBehavior_Rejection(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()
	stub.SetCreditBehavior(StubBehaviorRejection)

	_, err := stub.Credit(context.Background(), CreditRequest{
		VANumber:  "8999123456",
		AmountSen: 10000000,
	})
	if err == nil {
		t.Fatal("expected error for rejection behavior")
	}
	if err.Code != ErrBankRejection.Code {
		t.Errorf("code: got %q, want %q", err.Code, ErrBankRejection.Code)
	}
}

func TestStubAdapter_ReversalBehavior_Rejection(t *testing.T) {
	t.Parallel()

	stub := NewStubAdapter()
	stub.SetReversalBehavior(StubBehaviorRejection)

	_, err := stub.Reversal(context.Background(), ReversalRequest{
		ExternalID: "test-rev-rejection",
		AmountSen:  5000000,
	})
	if err == nil {
		t.Fatal("expected error for rejection behavior")
	}
	if err.Code != ErrBankRejection.Code {
		t.Errorf("code: got %q, want %q", err.Code, ErrBankRejection.Code)
	}
}

// TestStubAdapter_Behavior_Slow verifies the slow behavior returns success after 25s
// when the context has a long enough timeout.
func TestStubAdapter_Behavior_Slow(t *testing.T) {
	// Not t.Parallel() — this test takes 25 seconds.
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	stub := NewStubAdapter()
	stub.SetBehavior(StubBehaviorSlow)

	// Create a context with enough time for the 25s delay.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	result, err := stub.Withdraw(ctx, WithdrawRequest{
		BankAccount: "1234567890",
		AmountSen:   5000000,
		ExternalID:  "test-slow",
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if elapsed < 24*time.Second {
		t.Errorf("expected ~25s delay, got %v", elapsed)
	}
}

// Use the time import.
var _ = time.Now
