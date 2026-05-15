package bank

import (
	"strings"
	"testing"

	"senpay/internal/types"

	"github.com/google/uuid"
)

func TestStringToSign(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  SNAPRequest
	}{
		{
			name: "post_with_body",
			req: SNAPRequest{
				HTTPMethod:  "POST",
				EndpointURL: "/api/v1/transfer/va",
				AccessToken: "",
				Body:        `{"amount":100000}`,
				Timestamp:   "2026-05-15T10:00:00Z",
			},
		},
		{
			name: "get_no_body",
			req: SNAPRequest{
				HTTPMethod:  "GET",
				EndpointURL: "/api/v1/va/1234567890",
				AccessToken: "bearer-token",
				Body:        "",
				Timestamp:   "2026-05-15T10:00:00Z",
			},
		},
		{
			name: "empty_fields",
			req: SNAPRequest{
				HTTPMethod:  "",
				EndpointURL: "",
				AccessToken: "",
				Body:        "",
				Timestamp:   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := StringToSign(tt.req)
			if s == "" {
				t.Error("stringToSign must not be empty")
			}

			// Verify the format: HTTPMethod:EndpointURL:AccessToken:BodyHash:Timestamp
			// Body hash should be 128 hex chars (SHA-512).
			parts := splitStringToSign(s)
			if len(parts) != 5 {
				t.Fatalf("expected 5 colon-separated parts, got %d: %q", len(parts), s)
			}

			if parts[0] != tt.req.HTTPMethod {
				t.Errorf("method: got %q, want %q", parts[0], tt.req.HTTPMethod)
			}
			if parts[1] != tt.req.EndpointURL {
				t.Errorf("endpoint: got %q, want %q", parts[1], tt.req.EndpointURL)
			}
			if parts[2] != tt.req.AccessToken {
				t.Errorf("token: got %q, want %q", parts[2], tt.req.AccessToken)
			}

			// Body hash should be 128 lowercase hex chars (SHA-512).
			if len(parts[3]) != 128 {
				t.Errorf("body hash length: got %d, want 128", len(parts[3]))
			}
			for _, c := range parts[3] {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("body hash contains non-hex char: %c", c)
				}
			}
		})
	}
}

func TestStringToSign_BodyHashDeterministic(t *testing.T) {
	t.Parallel()

	req1 := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/api/v1/transfer/va",
		Body:        `{"amount":100000}`,
		Timestamp:   "2026-05-15T10:00:00Z",
	}
	req2 := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/api/v1/transfer/va",
		Body:        `{"amount":100000}`,
		Timestamp:   "2026-05-15T10:00:00Z",
	}

	s1 := StringToSign(req1)
	s2 := StringToSign(req2)
	if s1 != s2 {
		t.Error("identical inputs must produce identical stringToSign")
	}
}

func TestSignAndVerify(t *testing.T) {
	t.Parallel()

	secret := "test-client-secret-12345"

	req := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/api/v1/transfer/va",
		AccessToken: "",
		Body:        `{"amount_sen":10000000}`,
		Timestamp:   "2026-05-15T10:00:00Z",
	}

	signature := Sign(req, secret)
	if signature == "" {
		t.Fatal("signature must not be empty")
	}

	// Verify the same signature works.
	if !VerifySignature(req, secret, signature) {
		t.Error("VerifySignature should return true for correct signature")
	}

	// Wrong secret should fail verification.
	if VerifySignature(req, "wrong-secret", signature) {
		t.Error("VerifySignature should return false for wrong secret")
	}

	// Tampered body should fail.
	tamperedReq := req
	tamperedReq.Body = `{"amount_sen":99999999}`
	if VerifySignature(tamperedReq, secret, signature) {
		t.Error("VerifySignature should return false for tampered body")
	}

	// Wrong timestamp should fail.
	wrongTimeReq := req
	wrongTimeReq.Timestamp = "2026-05-15T12:00:00Z"
	if VerifySignature(wrongTimeReq, secret, signature) {
		t.Error("VerifySignature should return false for different timestamp")
	}

	// Empty signature should fail.
	if VerifySignature(req, secret, "") {
		t.Error("VerifySignature should return false for empty signature")
	}
}

func TestSign_Deterministic(t *testing.T) {
	t.Parallel()

	secret := "fixed-secret"
	req := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/test",
		Body:        `{"key":"value"}`,
		Timestamp:   "2026-01-01T00:00:00Z",
	}

	sig1 := Sign(req, secret)
	sig2 := Sign(req, secret)
	if sig1 != sig2 {
		t.Error("signature should be deterministic for same inputs")
	}
}

func TestGenerateVANumber(t *testing.T) {
	t.Parallel()

	va, err := GenerateVANumber()
	if err != nil {
		t.Fatalf("GenerateVANumber returned error: %v", err)
	}

	if len(va) != VALength {
		t.Errorf("VA length: got %d, want %d", len(va), VALength)
	}

	if !ValidateVANumber(va) {
		t.Errorf("VA number %q should be valid", va)
	}

	// Verify prefix.
	expectedPrefix := VAPrefix
	if len(va) < len(expectedPrefix) || va[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("VA %q does not have expected prefix %q", va, expectedPrefix)
	}

	// All digits.
	for _, c := range va {
		if c < '0' || c > '9' {
			t.Errorf("VA %q contains non-digit char %c", va, c)
		}
	}
}

func TestGenerateVANumber_Unique(t *testing.T) {
	t.Parallel()

	// Generate multiple VAs and check uniqueness.
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		va, err := GenerateVANumber()
		if err != nil {
			t.Fatalf("GenerateVANumber returned error: %v", err)
		}
		if seen[va] {
			t.Errorf("duplicate VA number generated: %s", va)
		}
		seen[va] = true
	}
}

func TestValidateVANumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		va    string
		valid bool
	}{
		{name: "valid_va", va: "8999123456", valid: true},
		{name: "valid_va_diff_suffix", va: "8999000001", valid: true},
		{name: "valid_va_max", va: "8999999999", valid: true},
		{name: "too_short", va: "8999123", valid: false},
		{name: "too_long", va: "89991234567", valid: false},
		{name: "wrong_prefix", va: "7999123456", valid: false},
		{name: "contains_letters", va: "8999a23456", valid: false},
		{name: "empty", va: "", valid: false},
		{name: "all_zeros", va: "0000000000", valid: false}, // no prefix
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateVANumber(tt.va)
			if got != tt.valid {
				t.Errorf("ValidateVANumber(%q) = %v, want %v", tt.va, got, tt.valid)
			}
		})
	}
}

func TestGenerateTopupCore_Valid(t *testing.T) {
	t.Parallel()

	req := TopupRequest{
		IdempotencyKey: "test-key-123",
		AmountSen:      10000000, // Rp 100,000
	}

	result, err := GenerateTopupCore(req)
	if err != nil {
		t.Fatalf("GenerateTopupCore returned error: %v", err)
	}

	if result == nil {
		t.Fatal("result must not be nil")
	}

	if result.VANumber == "" {
		t.Error("VA number must not be empty")
	}
	if !ValidateVANumber(result.VANumber) {
		t.Errorf("VA number %q is not valid", result.VANumber)
	}
	if result.AmountSen != req.AmountSen {
		t.Errorf("amount: got %d, want %d", result.AmountSen, req.AmountSen)
	}
	if result.Status != types.TxStatusPending {
		t.Errorf("status: got %q, want %q", result.Status, types.TxStatusPending)
	}
	if result.CreatedAt.IsZero() {
		t.Error("created_at must not be zero")
	}
	if result.ExpiresAt.Before(result.CreatedAt) {
		t.Error("expires_at must be after created_at")
	}
	if result.ExpiresAt.Sub(result.CreatedAt) != VATTL {
		t.Errorf("VA TTL: got %v, want %v", result.ExpiresAt.Sub(result.CreatedAt), VATTL)
	}
}

func TestGenerateTopupCore_InvalidAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		amount   int64
		wantCode string
	}{
		{name: "zero_amount", amount: 0, wantCode: types.ErrCodeInvalidAmount},
		{name: "negative_amount", amount: -1000, wantCode: types.ErrCodeInvalidAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TopupRequest{
				IdempotencyKey: "test-key",
				AmountSen:      tt.amount,
			}
			_, err := GenerateTopupCore(req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Code != tt.wantCode {
				t.Errorf("code: got %q, want %q", err.Code, tt.wantCode)
			}
		})
	}
}

func TestGenerateTopupCore_NoIdempotencyKey(t *testing.T) {
	t.Parallel()

	req := TopupRequest{
		IdempotencyKey: "",
		AmountSen:      10000000,
	}
	_, err := GenerateTopupCore(req)
	if err == nil {
		t.Fatal("expected error for missing idempotency_key, got nil")
	}
	if err.Code != types.ErrCodeMissingField {
		t.Errorf("code: got %q, want %q", err.Code, types.ErrCodeMissingField)
	}
}

// ─── Helper ─────────────────────────────────────────────────────

// splitStringToSign splits a stringToSign into its 5 component parts.
// The format is: method:endpoint:token:bodyhash:timestamp
// Uses strings.SplitN with limit 5 to handle trailing empty strings correctly.
func splitStringToSign(s string) []string {
	return strings.SplitN(s, ":", 5)
}

// Test property-based SNAP signing invariants.
func TestProperty_SNAP_SignVerify(t *testing.T) {
	t.Parallel()

	secrets := []string{
		"short",
		"a-very-long-client-secret-that-is-used-for-hmac",
		"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		"",
	}

	methods := []string{"GET", "POST", "PUT", "DELETE"}
	endpoints := []string{"/api/v1/transfer/va", "/api/v1/va/status", "/health"}
	bodies := []string{
		"",
		`{"amount_sen":10000000}`,
		`{"amount_sen":5000000,"va_number":"8999123456"}`,
		`{"data":{"key":"value","nested":{"a":1}}}`,
	}
	timestamps := []string{
		"2026-05-15T10:00:00Z",
		"2026-05-15T10:00:00.123456789Z",
	}

	for _, secret := range secrets {
		for _, method := range methods {
			for _, ep := range endpoints {
				for _, body := range bodies {
					for _, ts := range timestamps {
						req := SNAPRequest{
							HTTPMethod:  method,
							EndpointURL: ep,
							Body:        body,
							Timestamp:   ts,
						}

						sig := Sign(req, secret)
						if !VerifySignature(req, secret, sig) {
							t.Errorf("self-verification failed: secret=%q method=%q ep=%q body=%q ts=%q",
								secret, method, ep, body, ts)
						}

						// Different secret should not verify.
						wrongSecret := secret + "x"
						if VerifySignature(req, wrongSecret, sig) {
							t.Errorf("wrong secret should not verify: secret=%q wrong=%q",
								secret, wrongSecret)
						}
					}
				}
			}
		}
	}
}

func TestGenerateVANumber_UniqueAcrossGoroutines(t *testing.T) {
	t.Parallel()

	// Generate VAs concurrently and check for duplicates.
	const count = 50
	results := make(chan string, count)
	for i := 0; i < count; i++ {
		go func() {
			va, err := GenerateVANumber()
			if err != nil {
				results <- ""
				return
			}
			results <- va
		}()
	}

	seen := make(map[string]bool)
	for i := 0; i < count; i++ {
		va := <-results
		if va == "" {
			t.Fatal("empty VA from concurrent generation")
		}
		if seen[va] {
			t.Errorf("duplicate VA from concurrent generation: %s", va)
		}
		seen[va] = true
	}
}

func TestGenerateTopupCore_ResultFields(t *testing.T) {
	t.Parallel()

	req := TopupRequest{
		IdempotencyKey: "test-key-fields",
		AmountSen:      25000000,
	}

	result, err := GenerateTopupCore(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID == uuid.Nil {
		t.Error("ID must not be nil UUID")
	}
	if result.VANumber == "" {
		t.Error("VA number must not be empty")
	}
	if result.AmountSen != 25000000 {
		t.Errorf("AmountSen: got %d, want %d", result.AmountSen, 25000000)
	}
	if result.Status != types.TxStatusPending {
		t.Errorf("Status: got %q, want %q", result.Status, types.TxStatusPending)
	}
	if result.CreatedAt.IsZero() {
		t.Error("CreatedAt must not be zero")
	}
	if result.ExpiresAt.IsZero() {
		t.Error("ExpiresAt must not be zero")
	}
	if !result.ExpiresAt.After(result.CreatedAt) {
		t.Error("ExpiresAt must be after CreatedAt")
	}
}


