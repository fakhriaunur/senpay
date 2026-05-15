package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// BaseURL is the backend API base URL.
// Defaults to localhost:8384.
var BaseURL = "http://127.0.0.1:8384"

// HTTPClient is the HTTP client used for API calls.
var HTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// loginRequest is the JSON body for POST /v1/auth/login.
type loginRequest struct {
	Phone string `json:"phone"`
	PIN   string `json:"pin"`
}

// loginResponse is the JSON response for POST /v1/auth/login.
type loginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// errorResponse is the JSON error response from the API.
type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// apiError wraps an API error response.
type apiError struct {
	Code    string
	Message string
}

func (e *apiError) Error() string {
	return e.Message
}

// Login calls POST /v1/auth/login and returns the JWT tokens.
// Returns the token, refresh token, and any error.
func Login(phone, pin string) (string, string, error) {
	body := loginRequest{
		Phone: phone,
		PIN:   pin,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	resp, err := HTTPClient.Post(BaseURL+"/v1/auth/login", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", "", fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return "", "", &apiError{Code: errResp.Error.Code, Message: errResp.Error.Message}
		}
		return "", "", &apiError{
			Code:    "UNKNOWN",
			Message: fmt.Sprintf("Gagal terhubung ke server (status %d)", resp.StatusCode),
		}
	}

	var loginResp loginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return "", "", fmt.Errorf("parse response: %w", err)
	}

	return loginResp.Token, loginResp.RefreshToken, nil
}

// balanceResponse is the JSON response for GET /v1/wallet/balance.
type balanceResponse struct {
	Data struct {
		BalanceSen int64 `json:"balance_sen"`
		Version    int   `json:"version"`
	} `json:"data"`
}

// GetBalance calls GET /v1/wallet/balance with the given token.
// Returns balance in sen and version.
func GetBalance(token string) (int64, int, error) {
	req, err := http.NewRequest("GET", BaseURL+"/v1/wallet/balance", nil)
	if err != nil {
		return 0, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return 0, 0, &apiError{Code: errResp.Error.Code, Message: errResp.Error.Message}
		}
		return 0, 0, &apiError{
			Code:    "UNKNOWN",
			Message: fmt.Sprintf("Gagal mengambil saldo (status %d)", resp.StatusCode),
		}
	}

	var balResp balanceResponse
	if err := json.Unmarshal(respBody, &balResp); err != nil {
		return 0, 0, fmt.Errorf("parse response: %w", err)
	}

	return balResp.Data.BalanceSen, balResp.Data.Version, nil
}

// --- Transfer API ---

// transferRequest is the JSON body for POST /v1/transfer.
type transferRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	ToPhone        string `json:"to_phone"`
	AmountSen      int64  `json:"amount_sen"`
}

// TransferResponse is the JSON response from POST /v1/transfer.
type TransferResponse struct {
	TxID               string `json:"tx_id"`
	Status             string `json:"status"`
	AmountSen          int64  `json:"amount_sen"`
	FeeSen             int64  `json:"fee_sen"`
	SenderBalanceSen   int64  `json:"sender_balance_sen"`
	ReceiverBalanceSen int64  `json:"receiver_balance_sen"`
}

// PostTransfer calls POST /v1/transfer with the given parameters.
func PostTransfer(token, idempotencyKey, toPhone string, amountSen int64) (*TransferResponse, error) {
	body := transferRequest{
		IdempotencyKey: idempotencyKey,
		ToPhone:        toPhone,
		AmountSen:      amountSen,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", BaseURL+"/v1/transfer", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp errorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, &apiError{Code: errResp.Error.Code, Message: errResp.Error.Message}
		}
		return nil, &apiError{
			Code:    "UNKNOWN",
			Message: fmt.Sprintf("Transfer gagal (status %d)", resp.StatusCode),
		}
	}

	var apiResp struct {
		Data TransferResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &apiResp.Data, nil
}

// --- Transaction History API ---

// TransactionItem is a single transaction in the list.
type TransactionItem struct {
	ID                string `json:"id"`
	TxType            string `json:"tx_type"`
	SenderID          string `json:"sender_id,omitempty"`
	ReceiverID        string `json:"receiver_id,omitempty"`
	CounterpartyID    string `json:"counterparty_id,omitempty"`
	CounterpartyPhone string `json:"counterparty_phone,omitempty"`
	AmountSen         int64  `json:"amount_sen"`
	Currency          string `json:"currency"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
	CommittedAt       string `json:"committed_at,omitempty"`
}

// TransactionListResponse is the paginated list response.
type TransactionListResponse struct {
	Data       []TransactionItem `json:"data"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

// GetTransactions calls GET /v1/transactions with optional cursor and limit.
func GetTransactions(token, cursor string, limit int) (*TransactionListResponse, error) {
	url := BaseURL + "/v1/transactions?limit=" + strconv.Itoa(limit)
	if cursor != "" {
		url += "&cursor=" + cursor
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, &apiError{Code: errResp.Error.Code, Message: errResp.Error.Message}
		}
		return nil, &apiError{
			Code:    "UNKNOWN",
			Message: fmt.Sprintf("Gagal mengambil riwayat (status %d)", resp.StatusCode),
		}
	}

	var listResp TransactionListResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &listResp, nil
}

// GetTransactionDetail calls GET /v1/transactions/{id}.
func GetTransactionDetail(token, txID string) (*TransactionItem, error) {
	req, err := http.NewRequest("GET", BaseURL+"/v1/transactions/"+txID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, &apiError{Code: errResp.Error.Code, Message: errResp.Error.Message}
		}
		return nil, &apiError{
			Code:    "UNKNOWN",
			Message: fmt.Sprintf("Gagal mengambil detail transaksi (status %d)", resp.StatusCode),
		}
	}

	var detailResp struct {
		Data TransactionItem `json:"data"`
	}
	if err := json.Unmarshal(respBody, &detailResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &detailResp.Data, nil
}
