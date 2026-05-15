// Package bank provides the mock bank server that simulates an Indonesian
// bank's VA (Virtual Account) payment infrastructure.
//
// The mock bank:
//   - Runs in-process as a net/http handler mounted under /bank/
//   - Validates SNAP protocol headers (X-TIMESTAMP, X-SIGNATURE, X-PARTNER-ID,
//     X-EXTERNAL-ID, CHANNEL-ID) on incoming requests
//   - Rejects invalid/missing signatures with 401
//   - Rejects missing headers with 400
//   - Simulates processing delay (500ms-2s)
//   - Provides webhook callback to notify Senpay of successful VA payment
//   - Exposes health check endpoint at GET /bank/health
package bank

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"

	"senpay/internal/types"
	"time"
)

// ────────────────────────────────────────────────────────────────
// Mock Bank Server
// ────────────────────────────────────────────────────────────────

// MockBankConfig configures the mock bank server behavior.
type MockBankConfig struct {
	// ClientSecret is the shared secret for HMAC-SHA512 signature verification.
	ClientSecret string
	// PartnerID is the expected X-PARTNER-ID header value.
	PartnerID string
	// ChannelID is the expected CHANNEL-ID header value.
	ChannelID string
	// MinDelay is the minimum simulated processing delay.
	MinDelay time.Duration
	// MaxDelay is the maximum simulated processing delay.
	MaxDelay time.Duration
	// WebhookURL is the URL to call when a VA payment is simulated.
	// The mock bank will POST a BankCallback to this URL.
	WebhookURL string
	// AlwaysTimeout, if true, simulates timeout on all operations.
	AlwaysTimeout bool
}

// DefaultMockBankConfig returns a default configuration for the mock bank.
func DefaultMockBankConfig() MockBankConfig {
	return MockBankConfig{
		ClientSecret: "senpay-mock-secret",
		PartnerID:    SNAPPartnerID,
		ChannelID:    SNAPChannelID,
		MinDelay:     500 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		WebhookURL:   "", // set when registering the webhook handler
		AlwaysTimeout: false,
	}
}

// MockBank is an in-process mock bank that validates SNAP signatures and
// simulates bank processing.
type MockBank struct {
	config     MockBankConfig
	rand       *rand.Rand
	mu         sync.Mutex
	// log of received requests (for test assertions)
	requests []MockBankRequestLog
}

// MockBankRequestLog records information about a received request.
type MockBankRequestLog struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	Headers     map[string]string   `json:"headers"`
	Body        string              `json:"body"`
	ReceivedAt  time.Time           `json:"received_at"`
	SignatureValid bool             `json:"signature_valid"`
}

// NewMockBank creates a new mock bank server with the given configuration.
func NewMockBank(config MockBankConfig) *MockBank {
	return &MockBank{
		config:   config,
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
		requests: make([]MockBankRequestLog, 0),
	}
}

// Handler returns an http.Handler that implements the mock bank endpoints.
//
// Routes:
//   - GET /bank/health — health check (no SNAP auth required)
//   - POST /bank/api/v1/credit — credit VA (SNAP auth required)
//   - POST /bank/api/v1/withdraw — withdraw (SNAP auth required)
//   - POST /bank/api/v1/reversal — reversal (SNAP auth required)
func (m *MockBank) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /bank/health", m.handleHealth)
	mux.HandleFunc("POST /bank/api/v1/credit", m.handleCredit)
	mux.HandleFunc("POST /bank/api/v1/withdraw", m.handleWithdraw)
	mux.HandleFunc("POST /bank/api/v1/reversal", m.handleReversal)
	return mux
}

// GetRequests returns a copy of the request log for testing.
func (m *MockBank) GetRequests() []MockBankRequestLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	logCopy := make([]MockBankRequestLog, len(m.requests))
	copy(logCopy, m.requests)
	return logCopy
}

// ClearRequests clears the request log.
func (m *MockBank) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = make([]MockBankRequestLog, 0)
}

// logRequest records a request in the log.
func (m *MockBank) logRequest(r *http.Request, body string, sigValid bool) {
	headers := make(map[string]string)
	for k, v := range r.Header {
		headers[k] = strings.Join(v, ", ")
	}

	m.mu.Lock()
	m.requests = append(m.requests, MockBankRequestLog{
		Method:      r.Method,
		Path:        r.URL.Path,
		Headers:     headers,
		Body:        body,
		ReceivedAt:  time.Now().UTC(),
		SignatureValid: sigValid,
	})
	m.mu.Unlock()
}

// requireSNAPHeaders validates that all mandatory SNAP headers are present.
// Returns nil on success, or an error response with specific missing header.
func (m *MockBank) requireSNAPHeaders(r *http.Request) *snapError {
	required := []struct {
		name      string
		headerKey string
	}{
		{"X-TIMESTAMP", "X-TIMESTAMP"},
		{"X-SIGNATURE", "X-SIGNATURE"},
		{"X-PARTNER-ID", "X-PARTNER-ID"},
		{"X-EXTERNAL-ID", "X-EXTERNAL-ID"},
		{"CHANNEL-ID", "CHANNEL-ID"},
	}

	for _, h := range required {
		if r.Header.Get(h.headerKey) == "" {
			return &snapError{
				Code:    "MISSING_HEADER",
				Message: fmt.Sprintf("Header %s wajib diisi", h.name),
			}
		}
	}
	return nil
}

// validateSNAPSignature validates the HMAC-SHA512 signature of an incoming request.
func (m *MockBank) validateSNAPSignature(r *http.Request, body string) bool {
	timestamp := r.Header.Get("X-TIMESTAMP")
	signature := r.Header.Get("X-SIGNATURE")

	snapReq := SNAPRequest{
		HTTPMethod:  r.Method,
		EndpointURL: r.URL.Path,
		AccessToken: "", // no bearer token for mock bank
		Body:        body,
		Timestamp:   timestamp,
	}

	// Recreate the signature using our client secret.
	stringToSign := StringToSign(snapReq)
	mac := hmac.New(sha512.New, []byte(m.config.ClientSecret))
	mac.Write([]byte(stringToSign))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// snapError represents an error response from the mock bank.
type snapError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeSNAPError writes a SNAP error response as JSON.
func writeSNAPError(w http.ResponseWriter, statusCode int, err snapError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": err,
	})
}

// simulateDelay adds a random processing delay between MinDelay and MaxDelay.
func (m *MockBank) simulateDelay() {
	if m.config.MinDelay >= m.config.MaxDelay {
		return // no delay variation
	}
	m.mu.Lock()
	delayRange := int64(m.config.MaxDelay - m.config.MinDelay)
	if delayRange <= 0 {
		m.mu.Unlock()
		return
	}
	delay := m.config.MinDelay + time.Duration(m.rand.Int63n(delayRange))
	m.mu.Unlock()
	time.Sleep(delay)
}

// ── Handlers ────────────────────────────────────────────────────

// handleHealth serves GET /bank/health.
func (m *MockBank) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleCredit serves POST /bank/api/v1/credit.
func (m *MockBank) handleCredit(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	bodyStr := string(body)

	// Validate SNAP headers.
	if headerErr := m.requireSNAPHeaders(r); headerErr != nil {
		writeSNAPError(w, http.StatusBadRequest, *headerErr)
		m.logRequest(r, bodyStr, false)
		return
	}

	// Validate signature.
	if !m.validateSNAPSignature(r, bodyStr) {
		writeSNAPError(w, http.StatusUnauthorized, snapError{
			Code:    "INVALID_SIGNATURE",
			Message: "Signature tidak valid",
		})
		m.logRequest(r, bodyStr, false)
		return
	}

	// Simulate processing delay.
	if !m.config.AlwaysTimeout {
		m.simulateDelay()
	}

	m.logRequest(r, bodyStr, true)

	// Return success.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(CreditResult{
		Success:      true,
		ReferenceID:  fmt.Sprintf("BANK-REF-%s", r.Header.Get("X-EXTERNAL-ID")),
		BankResponse: "mock: credit approved",
	})

	// Trigger webhook callback asynchronously.
	if m.config.WebhookURL != "" {
		go m.triggerWebhook(r.Header.Get("X-EXTERNAL-ID"), bodyStr)
	}
}

// handleWithdraw serves POST /bank/api/v1/withdraw.
func (m *MockBank) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	bodyStr := string(body)

	// Validate SNAP headers.
	if headerErr := m.requireSNAPHeaders(r); headerErr != nil {
		writeSNAPError(w, http.StatusBadRequest, *headerErr)
		m.logRequest(r, bodyStr, false)
		return
	}

	// Validate signature.
	if !m.validateSNAPSignature(r, bodyStr) {
		writeSNAPError(w, http.StatusUnauthorized, snapError{
			Code:    "INVALID_SIGNATURE",
			Message: "Signature tidak valid",
		})
		m.logRequest(r, bodyStr, false)
		return
	}

	// Simulate processing delay.
	if !m.config.AlwaysTimeout {
		m.simulateDelay()
	}

	m.logRequest(r, bodyStr, true)

	// Return success.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(CreditResult{
		Success:      true,
		ReferenceID:  fmt.Sprintf("BANK-WD-%s", r.Header.Get("X-EXTERNAL-ID")),
		BankResponse: "mock: withdraw approved",
	})
}

// handleReversal serves POST /bank/api/v1/reversal.
func (m *MockBank) handleReversal(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	bodyStr := string(body)

	// Validate SNAP headers.
	if headerErr := m.requireSNAPHeaders(r); headerErr != nil {
		writeSNAPError(w, http.StatusBadRequest, *headerErr)
		m.logRequest(r, bodyStr, false)
		return
	}

	// Validate signature.
	if !m.validateSNAPSignature(r, bodyStr) {
		writeSNAPError(w, http.StatusUnauthorized, snapError{
			Code:    "INVALID_SIGNATURE",
			Message: "Signature tidak valid",
		})
		m.logRequest(r, bodyStr, false)
		return
	}

	// Simulate processing delay.
	if !m.config.AlwaysTimeout {
		m.simulateDelay()
	}

	m.logRequest(r, bodyStr, true)

	// Return success.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ReversalResult{
		Success:     true,
		ReferenceID: fmt.Sprintf("BANK-REV-%s", r.Header.Get("X-EXTERNAL-ID")),
	})
}

// triggerWebhook sends a webhook callback to the configured WebhookURL.
// This simulates the bank notifying Senpay that a VA payment was received.
func (m *MockBank) triggerWebhook(externalID, creditBody string) {
	// Parse the credit request body to extract VA number and amount.
	var creditReq CreditRequest
	_ = json.Unmarshal([]byte(creditBody), &creditReq)

	callback := BankCallback{
		VANumber:   creditReq.VANumber,
		AmountSen:  creditReq.AmountSen,
		ExternalID: externalID,
		Status:     types.CallbackSuccess,
		ReferenceID: fmt.Sprintf("BANK-CALLBACK-%s", externalID),
	}

	callbackBody, err := json.Marshal(callback)
	if err != nil {
		slog.Error("failed to marshal webhook callback", "error", err)
		return
	}

	// Small delay before sending webhook (simulates bank processing time).
	time.Sleep(1 * time.Second)

	slog.Info("sending webhook callback",
		"url", m.config.WebhookURL,
		"va_number", creditReq.VANumber,
		"external_id", externalID)

	resp, err := http.Post(m.config.WebhookURL, "application/json", bytes.NewReader(callbackBody))
	if err != nil {
		slog.Error("webhook callback failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("webhook callback returned non-200", "status", resp.StatusCode)
	}

	slog.Info("webhook callback sent successfully",
		"status", resp.StatusCode,
		"external_id", externalID)
}
