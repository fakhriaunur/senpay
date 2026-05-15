package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
