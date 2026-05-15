package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"senpay/internal/types"
)

// ────────────────────────────────────────────────────────────────
// SNAP Adapter (Real)
// ────────────────────────────────────────────────────────────────

// SnapAdapter implements PaymentRail using the SNAP protocol.
// It signs requests with HMAC-SHA512 and sends them to the mock bank.
type SnapAdapter struct {
	baseURL      string // e.g., "http://127.0.0.1:8384/bank"
	clientSecret string // shared secret for HMAC signing
	partnerID    string
	channelID    string
	httpClient   *http.Client
}

// NewSnapAdapter creates a new SNAP adapter.
// The baseURL points to the mock bank server base path.
func NewSnapAdapter(baseURL, clientSecret, partnerID, channelID string) *SnapAdapter {
	return &SnapAdapter{
		baseURL:      baseURL,
		clientSecret: clientSecret,
		partnerID:    partnerID,
		channelID:    channelID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the adapter name.
func (s *SnapAdapter) Name() string { return "snap" }

// Credit sends a credit (top-up) request to the mock bank via SNAP protocol.
func (s *SnapAdapter) Credit(ctx context.Context, req CreditRequest) (*CreditResult, *types.DomainError) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, &types.ErrInternal
	}
	bodyStr := string(bodyBytes)
	timestamp := req.Timestamp.Format(SNAPTimeFormat)

	// Build SNAP request.
	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/credit",
		AccessToken: "",
		Body:        bodyStr,
		Timestamp:   timestamp,
	}

	// Generate signature.
	signature := Sign(snapReq, s.clientSecret)
	externalID := req.ExternalID

	// Build HTTP request with SNAP headers.
	httpReq, err := http.NewRequestWithContext(ctx, snapReq.HTTPMethod,
		s.baseURL+snapReq.EndpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &types.ErrInternal
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", signature)
	httpReq.Header.Set("X-PARTNER-ID", s.partnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", externalID)
	httpReq.Header.Set("CHANNEL-ID", s.channelID)

	// Send request.
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		slog.Error("bank credit request failed", "error", err)
		return nil, &ErrTimeout
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.ErrInternal
	}

	// Handle error responses.
	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(resp.StatusCode, respBody)
	}

	// Parse success response.
	var result CreditResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, &types.ErrInternal
	}

	return &result, nil
}

// Withdraw sends a withdraw request to the mock bank via SNAP protocol.
func (s *SnapAdapter) Withdraw(ctx context.Context, req WithdrawRequest) (*CreditResult, *types.DomainError) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, &types.ErrInternal
	}
	bodyStr := string(bodyBytes)
	timestamp := req.Timestamp.Format(SNAPTimeFormat)

	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/withdraw",
		AccessToken: "",
		Body:        bodyStr,
		Timestamp:   timestamp,
	}

	signature := Sign(snapReq, s.clientSecret)

	httpReq, err := http.NewRequestWithContext(ctx, snapReq.HTTPMethod,
		s.baseURL+snapReq.EndpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &types.ErrInternal
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", signature)
	httpReq.Header.Set("X-PARTNER-ID", s.partnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", req.ExternalID)
	httpReq.Header.Set("CHANNEL-ID", s.channelID)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		slog.Error("bank withdraw request failed", "error", err)
		return nil, &ErrTimeout
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.ErrInternal
	}

	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(resp.StatusCode, respBody)
	}

	var result CreditResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, &types.ErrInternal
	}
	return &result, nil
}

// Reversal sends a reversal request to undo a previous operation.
func (s *SnapAdapter) Reversal(ctx context.Context, req ReversalRequest) (*ReversalResult, *types.DomainError) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, &types.ErrInternal
	}
	bodyStr := string(bodyBytes)
	timestamp := req.Timestamp.Format(SNAPTimeFormat)

	snapReq := SNAPRequest{
		HTTPMethod:  "POST",
		EndpointURL: "/bank/api/v1/reversal",
		AccessToken: "",
		Body:        bodyStr,
		Timestamp:   timestamp,
	}

	signature := Sign(snapReq, s.clientSecret)

	httpReq, err := http.NewRequestWithContext(ctx, snapReq.HTTPMethod,
		s.baseURL+snapReq.EndpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &types.ErrInternal
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", signature)
	httpReq.Header.Set("X-PARTNER-ID", s.partnerID)
	httpReq.Header.Set("X-EXTERNAL-ID", req.ExternalID)
	httpReq.Header.Set("CHANNEL-ID", s.channelID)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		slog.Error("bank reversal request failed", "error", err)
		return nil, &ErrTimeout
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.ErrInternal
	}

	if resp.StatusCode != http.StatusOK {
		return nil, s.handleErrorResponseReversal(resp.StatusCode, respBody)
	}

	var result ReversalResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, &types.ErrInternal
	}
	return &result, nil
}

// handleErrorResponseReversal maps HTTP error status codes to DomainErrors for reversal requests.
func (s *SnapAdapter) handleErrorResponseReversal(statusCode int, body []byte) *types.DomainError {
	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Code != "" {
		switch errResp.Code {
		case "INVALID_SIGNATURE":
			return &types.DomainError{
				Code:       "INVALID_SIGNATURE",
				Message:    errResp.Message,
				HTTPStatus: 401,
			}
		case "DUPLICATE_EXTERNAL_ID":
			return &ErrDuplicateRequest
		case "BANK_REJECTION":
			return &ErrBankRejection
		}
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return &types.DomainError{
			Code:       "INVALID_SIGNATURE",
			Message:    "Signature tidak valid",
			HTTPStatus: 401,
		}
	case http.StatusBadRequest:
		return &types.DomainError{
			Code:       "INVALID_REQUEST",
			Message:    "Permintaan tidak valid",
			HTTPStatus: 400,
		}
	case http.StatusRequestTimeout:
		return &ErrTimeout
	default:
		return &ErrBankRejection
	}
}

// ParseWebhook parses a bank webhook callback from raw bytes.
func (s *SnapAdapter) ParseWebhook(body []byte) (*BankCallback, *types.DomainError) {
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

// handleErrorResponse maps HTTP error status codes to DomainErrors.
func (s *SnapAdapter) handleErrorResponse(statusCode int, body []byte) (*CreditResult, *types.DomainError) {
	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Code != "" {
		// Map known bank error codes to DomainErrors.
		switch errResp.Code {
		case "INVALID_SIGNATURE":
			return nil, &types.DomainError{
				Code:       "INVALID_SIGNATURE",
				Message:    errResp.Message,
				HTTPStatus: 401,
			}
		case "DUPLICATE_EXTERNAL_ID":
			return nil, &ErrDuplicateRequest
		case "INVALID_VA":
			return nil, &ErrInvalidVA
		case "BANK_REJECTION":
			return nil, &ErrBankRejection
		}
	}

	// Default error mapping based on status code.
	switch statusCode {
	case http.StatusUnauthorized:
		return nil, &types.DomainError{
			Code:       "INVALID_SIGNATURE",
			Message:    "Signature tidak valid",
			HTTPStatus: 401,
		}
	case http.StatusBadRequest:
		return nil, &types.DomainError{
			Code:       "INVALID_REQUEST",
			Message:    "Permintaan tidak valid",
			HTTPStatus: 400,
		}
	case http.StatusRequestTimeout:
		return nil, &ErrTimeout
	default:
		return nil, &ErrBankRejection
	}
}

// ensure interfaces are satisfied
var _ PaymentRail = (*SnapAdapter)(nil)

// NewSnapAdapterWithClient creates a SNAP adapter with a custom HTTP client.
// Used for testing with custom timeouts.
func NewSnapAdapterWithClient(baseURL, clientSecret, partnerID, channelID string, httpClient *http.Client) *SnapAdapter {
	return &SnapAdapter{
		baseURL:      baseURL,
		clientSecret: clientSecret,
		partnerID:    partnerID,
		channelID:    channelID,
		httpClient:   httpClient,
	}
}

// SetHTTPClient sets a custom HTTP client (used for testing).
func (s *SnapAdapter) SetHTTPClient(client *http.Client) {
	s.httpClient = client
}
