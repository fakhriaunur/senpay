// Package bank implements SNAP protocol (Indonesian payment standard),
// mock bank server, VA (Virtual Account) generation, and bank adapters.
//
// FCIS structure:
//   - core.go: Pure functions — HMAC_SHA512 signing, VA generation, stringToSign construction
//   - provider.go: PaymentRail interface (port)
//   - provider_snap.go: Shell — real SNAP adapter (calls mock bank via HTTP)
//   - provider_stub.go: Shell — stub adapter with canned responses
//   - mock_server.go: Shell — in-process mock bank HTTP handler
//   - service.go: Shell — orchestrates core + stores + adapters
//   - handler.go: Shell — HTTP handler for top-up endpoints
package bank

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"senpay/internal/types"

	"github.com/google/uuid"
)

// ────────────────────────────────────────────────────────────────
// SNAP Protocol Constants
// ────────────────────────────────────────────────────────────────

const (
	// SNAPTimeFormat is the ISO 8601 timestamp format used in X-TIMESTAMP header.
	SNAPTimeFormat = time.RFC3339Nano

	// SNAPPartnerID is the default partner ID for Senpay.
	SNAPPartnerID = "SENPAY"

	// SNAPChannelID is the default channel ID.
	SNAPChannelID = "SENPAY_WEB"
)

// ────────────────────────────────────────────────────────────────
// SNAP Signature Generation (Pure)
// ────────────────────────────────────────────────────────────────

// SNAPRequest represents the components needed to sign a SNAP request.
type SNAPRequest struct {
	HTTPMethod  string // e.g., "POST", "GET"
	EndpointURL string // e.g., "/api/v1/transfer/va"
	AccessToken string // Bearer token or empty
	Body        string // JSON request body as string
	Timestamp   string // ISO 8601 timestamp
}

// StringToSign constructs the string that will be signed with HMAC-SHA512.
//
// Format:
//
//	HTTPMethod:EndpointURL:AccessToken:Lowercase(Hex(SHA256(Body))):Timestamp
//
// If Body is empty, the hex digest of an empty string is used.
// If AccessToken is empty, a colon placeholder remains.
func StringToSign(req SNAPRequest) string {
	bodyHash := sha512Hex(req.Body)
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		req.HTTPMethod,
		req.EndpointURL,
		req.AccessToken,
		bodyHash,
		req.Timestamp,
	)
}

// sha512Hex returns the lowercase hex-encoded SHA-512 digest of the input string.
func sha512Hex(input string) string {
	h := sha512.New()
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}

// Sign generates an HMAC-SHA512 signature for a SNAP request using the shared secret.
//
// The signature is the lowercase hex-encoded HMAC-SHA512 of the stringToSign.
// Format:
//
//	stringToSign = HTTPMethod:EndpointURL:AccessToken:Lowercase(Hex(SHA256(Body))):Timestamp
//	signature = Lowercase(Hex(HMAC_SHA512(stringToSign, clientSecret)))
func Sign(req SNAPRequest, clientSecret string) string {
	stringToSign := StringToSign(req)
	mac := hmac.New(sha512.New, []byte(clientSecret))
	mac.Write([]byte(stringToSign))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks whether the provided signature matches the expected
// HMAC-SHA512 signature for the given request and client secret.
func VerifySignature(req SNAPRequest, clientSecret, signature string) bool {
	expected := Sign(req, clientSecret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ────────────────────────────────────────────────────────────────
// VA (Virtual Account) Generation (Pure)
// ────────────────────────────────────────────────────────────────

const (
	// VAPrefix is the prefix for Senpay VA numbers (BRI-like format).
	VAPrefix = "8999"

	// VALength is the total length of a VA number (including prefix).
	VALength = 10

	// VATTL is the time-to-live for a VA number (1 hour).
	VATTL = 1 * time.Hour

	// MinTopupSen is the minimum top-up amount (Rp 1.000 = 100.000 sen).
	// This is a conceptual minimum; practical minimums depend on BI rules.
	MinTopupSen int64 = 100_000 // Rp 1.000
)

// GenerateVANumber generates a unique VA number for a top-up request.
//
// Format: BRI-like 10-digit VA number.
// Prefix "8999" identifies Senpay as the partner.
// The remaining 6 digits are random, making collisions extremely unlikely.
//
// Returns the VA number as a string.
func GenerateVANumber() (string, error) {
	// Generate 6 random digits.
	max := big.NewInt(1000000) // 0 to 999999
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generate random digits: %w", err)
	}
	digits := fmt.Sprintf("%06d", n.Int64())
	return VAPrefix + digits, nil
}

// ValidateVANumber checks whether a VA number matches the expected format.
func ValidateVANumber(vaNumber string) bool {
	if len(vaNumber) != VALength {
		return false
	}
	if len(vaNumber) < len(VAPrefix) || vaNumber[:len(VAPrefix)] != VAPrefix {
		return false
	}
	for _, c := range vaNumber {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ────────────────────────────────────────────────────────────────
// Top-up Domain Types
// ────────────────────────────────────────────────────────────────

// TopupRequest represents the core parameters for initiating a top-up.
type TopupRequest struct {
	UserID         uuid.UUID
	IdempotencyKey string
	AmountSen      int64
}

// TopupResult represents the result of a top-up VA creation.
type TopupResult struct {
	ID             uuid.UUID `json:"id"`
	TxLogID        uuid.UUID `json:"tx_log_id"`
	VANumber       string    `json:"va_number"`
	AmountSen      int64     `json:"amount_sen"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// TopupHandlerCore defines the pure core function signature for top-up processing.
// This is what the service layer calls.
type TopupHandlerCore interface {
	GenerateTopup(req TopupRequest) (*TopupResult, *types.DomainError)
}

// GenerateTopupCore is a pure function that validates a top-up request and
// generates VA details. No I/O, no side effects.
//
// Validation:
//   - amount_sen must be > 0
//   - idempotency_key must be non-empty
//
// Returns TopupResult with VA number, expiration (1 hour), and pending status.
func GenerateTopupCore(req TopupRequest) (*TopupResult, *types.DomainError) {
	if req.IdempotencyKey == "" {
		err := types.NewMissingFieldError("idempotency_key")
		return nil, &err
	}

	if req.AmountSen <= 0 {
		return nil, &types.ErrInvalidAmount
	}

	vaNumber, err := GenerateVANumber()
	if err != nil {
		return nil, &types.ErrInternal
	}

	now := time.Now().UTC()
	id := uuid.Must(uuid.NewV7())

	return &TopupResult{
		ID:        id,
		VANumber:  vaNumber,
		AmountSen: req.AmountSen,
		Status:    types.TxStatusPending,
		CreatedAt: now,
		ExpiresAt: now.Add(VATTL),
	}, nil
}

// ────────────────────────────────────────────────────────────────
// Bank Domain Errors
// ────────────────────────────────────────────────────────────────

var (
	// ErrTimeout is returned when a bank request times out (30s default).
	ErrTimeout = types.DomainError{
		Code:       "BANK_TIMEOUT",
		Message:    "Bank timeout, silakan coba lagi",
		HTTPStatus: 504,
	}

	// ErrInvalidVA is returned when the VA number is invalid or expired.
	ErrInvalidVA = types.DomainError{
		Code:       "INVALID_VA",
		Message:    "Nomor VA tidak valid",
		HTTPStatus: 400,
	}

	// ErrBankRejection is returned when the bank rejects a request.
	ErrBankRejection = types.DomainError{
		Code:       "BANK_REJECTION",
		Message:    "Bank menolak permintaan",
		HTTPStatus: 502,
	}

	// ErrDuplicateRequest is returned when a duplicate request is detected.
	ErrDuplicateRequest = types.DomainError{
		Code:       "DUPLICATE_REQUEST",
		Message:    "Permintaan duplikat",
		HTTPStatus: 409,
	}
)
