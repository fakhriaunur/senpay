//go:build integration

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"senpay/internal/store/storetest"
	"senpay/internal/types"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testJWTSecret = "test-secret-for-integration-tests"

func setupTestHandler(t *testing.T) (*Handler, func()) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pool, cleanup, err := storetest.NewTestPool(ctx)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	store := NewPostgresUserStore(pool)
	h := NewHandler(pool, store, testJWTSecret)

	return h, func() {
		cleanup()
	}
}

func mustRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func mustRequestWithToken(t *testing.T, method, path, body, token string) *http.Request {
	t.Helper()
	req := mustRequest(t, method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func readResponse(t *testing.T, w *httptest.ResponseRecorder) (int, map[string]interface{}) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return w.Code, resp
}

// serveWithAuth wraps a handler call with the auth middleware, using the given token.
// This ensures the request context has the authenticated user ID set by the middleware.
func serveWithAuth(handlerFn func(w http.ResponseWriter, r *http.Request), token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)

	middleware := AuthMiddleware(testJWTSecret)
	middleware(http.HandlerFunc(handlerFn)).ServeHTTP(w, r)

	return w
}

// serveWithAuthRequest wraps a handler call with the auth middleware and custom request.
func serveWithAuthRequest(handlerFn func(w http.ResponseWriter, r *http.Request), req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	middleware := AuthMiddleware(testJWTSecret)
	middleware(http.HandlerFunc(handlerFn)).ServeHTTP(w, req)
	return w
}

// mustLogin registers and logs in a test user, returning the access token and user ID.
func mustLogin(t *testing.T, h *Handler, phone string) (string, uuid.UUID) {
	t.Helper()

	w := httptest.NewRecorder()
	body := `{"phone":"` + phone + `","pin":"123456"}`
	h.Register(w, mustRequest(t, "POST", "/v1/auth/register", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("register failed: %d", w.Code)
	}
	var regResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&regResp)
	userIDStr, _ := regResp["user_id"].(string)
	userID := uuid.MustParse(userIDStr)

	w = httptest.NewRecorder()
	h.Login(w, mustRequest(t, "POST", "/v1/auth/login", body))
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}
	var loginResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&loginResp)
	token, _ := loginResp["token"].(string)

	return token, userID
}

func TestRegisterHandler(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	t.Run("success_returns_201_with_user_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"081234567890","pin":"123456"}`)
		h.Register(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %v", code, resp)
		}
		userIDStr, ok := resp["user_id"].(string)
		if !ok || userIDStr == "" {
			t.Fatal("expected non-empty user_id string")
		}
		id, err := uuid.Parse(userIDStr)
		if err != nil {
			t.Fatalf("user_id is not a valid UUID: %v", err)
		}
		if id == uuid.Nil {
			t.Fatal("user_id is nil UUID")
		}
	})

	t.Run("empty_phone_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"","pin":"123456"}`)
		h.Register(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", code, resp)
		}
		if err, ok := resp["error"].(map[string]interface{}); ok {
			if code, ok := err["code"].(string); ok && code != types.ErrCodeMissingField {
				t.Errorf("expected MISSING_FIELD error code, got %s", code)
			}
		}
	})

	t.Run("empty_pin_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"081234567890","pin":""}`)
		h.Register(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", code, resp)
		}
	})

	t.Run("short_pin_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"081234567890","pin":"12"}`)
		h.Register(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", code, resp)
		}
		errData, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error object in response")
		}
		msg, _ := errData["message"].(string)
		if !strings.Contains(msg, "PIN") {
			t.Errorf("expected message to mention PIN, got %q", msg)
		}
	})

	t.Run("invalid_phone_format_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"12345","pin":"123456"}`)
		h.Register(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", code, resp)
		}
	})

	t.Run("duplicate_phone_returns_409", func(t *testing.T) {
		// Register first user.
		w1 := httptest.NewRecorder()
		req1 := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"089999999999","pin":"123456"}`)
		h.Register(w1, req1)

		// Register second user with same phone.
		w2 := httptest.NewRecorder()
		req2 := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"089999999999","pin":"654321"}`)
		h.Register(w2, req2)

		code, resp := readResponse(t, w2)
		if code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %v", code, resp)
		}
		errData, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error object in response")
		}
		if code, ok := errData["code"].(string); ok && code != types.ErrCodePhoneAlreadyRegistered {
			t.Errorf("expected PHONE_ALREADY_REGISTERED, got %s", code)
		}
	})

	t.Run("empty_body_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{}`)
		h.Register(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", code, resp)
		}
	})

	t.Run("phone_only_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"081234567890"}`)
		h.Register(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
	})

	t.Run("pin_not_in_response", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"085555555555","pin":"123456"}`)
		h.Register(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %v", code, resp)
		}
		// Ensure pin is not in response.
		if _, ok := resp["pin"]; ok {
			t.Error("pin field should not be in response")
		}
		if _, ok := resp["pin_hash"]; ok {
			t.Error("pin_hash field should not be in response")
		}
	})
}

func TestLoginHandler(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	// Register a test user first.
	w := httptest.NewRecorder()
	req := mustRequest(t, "POST", "/v1/auth/register", `{"phone":"081111111111","pin":"123456"}`)
	h.Register(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to register test user: %d", w.Code)
	}

	t.Run("success_returns_tokens", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/login", `{"phone":"081111111111","pin":"123456"}`)
		h.Login(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", code, resp)
		}

		token, ok := resp["token"].(string)
		if !ok || token == "" {
			t.Fatal("expected non-empty token")
		}

		refreshToken, ok := resp["refresh_token"].(string)
		if !ok || refreshToken == "" {
			t.Fatal("expected non-empty refresh_token")
		}
		_ = refreshToken

		// Validate the access token can be parsed.
		claims, err := ValidateToken(token, testJWTSecret)
		if err != nil {
			t.Fatalf("access token should be valid: %v", err)
		}
		if claims.TokenType != TokenTypeAccess {
			t.Errorf("expected access token type, got %s", claims.TokenType)
		}
	})

	t.Run("wrong_pin_returns_401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/login", `{"phone":"081111111111","pin":"999999"}`)
		h.Login(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %v", code, resp)
		}
		errData, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error object")
		}
		if code, ok := errData["code"].(string); ok && code != types.ErrCodeInvalidPIN {
			t.Errorf("expected INVALID_PIN, got %s", code)
		}
		msg, ok := errData["message"].(string)
		if !ok || !strings.Contains(msg, "PIN") {
			t.Errorf("expected Indonesian message about PIN, got %q", msg)
		}
	})

	t.Run("nonexistent_phone_returns_404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/login", `{"phone":"089999999999","pin":"123456"}`)
		h.Login(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %v", code, resp)
		}
		errData, ok := resp["error"].(map[string]interface{})
		if !ok {
			t.Fatal("expected error object")
		}
		if code, ok := errData["code"].(string); ok && code != types.ErrCodeUserNotFound {
			t.Errorf("expected USER_NOT_FOUND, got %s", code)
		}
	})

	t.Run("missing_phone_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/login", `{"pin":"123456"}`)
		h.Login(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
	})

	t.Run("missing_pin_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/login", `{"phone":"081111111111"}`)
		h.Login(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
	})

	t.Run("empty_body_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/login", `{}`)
		h.Login(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
	})

	t.Run("pin_not_in_response", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/login", `{"phone":"081111111111","pin":"123456"}`)
		h.Login(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", code, resp)
		}
		if _, ok := resp["pin"]; ok {
			t.Error("pin should not be in login response")
		}
		if _, ok := resp["pin_hash"]; ok {
			t.Error("pin_hash should not be in login response")
		}
	})
}

func TestRefreshHandler(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	// Register and login to get tokens.
	token, _ := mustLogin(t, h, "082222222222")

	w := httptest.NewRecorder()
	h.Login(w, mustRequest(t, "POST", "/v1/auth/login", `{"phone":"082222222222","pin":"123456"}`))
	_, loginResp := readResponse(t, w)
	refreshToken, _ := loginResp["refresh_token"].(string)

	t.Run("valid_refresh_returns_new_tokens", func(t *testing.T) {
		w := httptest.NewRecorder()
		body := `{"refresh_token":"` + refreshToken + `"}`
		req := mustRequest(t, "POST", "/v1/auth/refresh", body)
		h.Refresh(w, req)

		code, resp := readResponse(t, w)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", code, resp)
		}

		newToken, ok := resp["token"].(string)
		if !ok || newToken == "" {
			t.Fatal("expected new access token")
		}
		newRefresh, ok := resp["refresh_token"].(string)
		if !ok || newRefresh == "" {
			t.Fatal("expected new refresh token")
		}

		// New tokens should be valid JWTs.
		claims, err := ValidateToken(newToken, testJWTSecret)
		if err != nil {
			t.Fatalf("new access token should be valid: %v", err)
		}
		if claims.TokenType != TokenTypeAccess {
			t.Errorf("expected access token type, got %s", claims.TokenType)
		}

		// Verify the new access token works with auth middleware.
		w2 := httptest.NewRecorder()
		r2 := httptest.NewRequest("GET", "/v1/auth/me", nil)
		r2.Header.Set("Authorization", "Bearer "+newToken)
		AuthMiddleware(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, ok := UserIDFromContext(r.Context())
			if !ok {
				t.Error("user ID not in context")
			}
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w2, r2)
		if w2.Code != http.StatusOK {
			t.Errorf("new token rejected by middleware: %d", w2.Code)
		}
	})

	t.Run("access_token_rejected_as_refresh", func(t *testing.T) {
		// Try using access token as refresh token.
		w := httptest.NewRecorder()
		body := `{"refresh_token":"` + token + `"}`
		req := mustRequest(t, "POST", "/v1/auth/refresh", body)
		h.Refresh(w, req)
		code, resp := readResponse(t, w)
		if code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %v", code, resp)
		}
	})

	t.Run("missing_refresh_token_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/refresh", `{}`)
		h.Refresh(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
	})

	t.Run("empty_refresh_token_returns_400", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/refresh", `{"refresh_token":""}`)
		h.Refresh(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", code)
		}
	})

	t.Run("expired_refresh_token_returns_401", func(t *testing.T) {
		// Create an expired refresh token using past timestamps.
		userID := uuid.Must(uuid.NewV7())
		now := time.Now()
		claims := CustomClaims{
			TokenType: TokenTypeRefresh,
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        uuid.Must(uuid.NewV7()).String(),
				Subject:   userID.String(),
				IssuedAt:  jwt.NewNumericDate(now.Add(-8 * 24 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		expiredToken, err := token.SignedString([]byte(testJWTSecret))
		if err != nil {
			t.Fatalf("sign expired token: %v", err)
		}

		w := httptest.NewRecorder()
		body := `{"refresh_token":"` + expiredToken + `"}`
		req := mustRequest(t, "POST", "/v1/auth/refresh", body)
		h.Refresh(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for expired refresh token, got %d", code)
		}
	})

	t.Run("reused_refresh_token_returns_401", func(t *testing.T) {
		// Login to get a refresh token.
		w := httptest.NewRecorder()
		h.Login(w, mustRequest(t, "POST", "/v1/auth/login", `{"phone":"082222222222","pin":"123456"}`))
		_, loginResp := readResponse(t, w)
		rt, _ := loginResp["refresh_token"].(string)

		// First use: should succeed.
		w = httptest.NewRecorder()
		body := `{"refresh_token":"` + rt + `"}`
		h.Refresh(w, mustRequest(t, "POST", "/v1/auth/refresh", body))
		if w.Code != http.StatusOK {
			t.Fatalf("first refresh should succeed, got %d", w.Code)
		}

		// Second use with same token: should be rejected (single-use rotation).
		w = httptest.NewRecorder()
		h.Refresh(w, mustRequest(t, "POST", "/v1/auth/refresh", body))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for reused refresh token, got %d", w.Code)
		}
	})
}

func TestKYCHandler(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	token, userID := mustLogin(t, h, "083333333333")

	t.Run("upgrade_to_verified_returns_200", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/auth/kyc", bytes.NewBufferString(`{"kyc_level":"verified"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		AuthMiddleware(testJWTSecret)(http.HandlerFunc(h.KYC)).ServeHTTP(w, r)

		code, resp := readResponse(t, w)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", code, resp)
		}
		level, ok := resp["kyc_level"].(string)
		if !ok || level != "verified" {
			t.Errorf("expected kyc_level 'verified', got %q", level)
		}
	})

	t.Run("requires_authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "POST", "/v1/auth/kyc", `{"kyc_level":"verified"}`)
		h.KYC(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", code)
		}
	})
	_ = userID
}

func TestMeHandler(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	token, userID := mustLogin(t, h, "084444444444")

	t.Run("returns_user_profile", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth/me", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		AuthMiddleware(testJWTSecret)(http.HandlerFunc(h.Me)).ServeHTTP(w, r)

		code, resp := readResponse(t, w)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", code, resp)
		}

		id, ok := resp["id"].(string)
		if !ok || id != userID.String() {
			t.Errorf("expected id %s, got %q", userID.String(), id)
		}
		phone, ok := resp["phone"].(string)
		if !ok || phone != "084444444444" {
			t.Errorf("expected phone 084444444444, got %q", phone)
		}
		kyc, ok := resp["kyc_level"].(string)
		if !ok || kyc != types.KYCLevelBasic {
			t.Errorf("expected kyc_level basic, got %q", kyc)
		}
	})

	t.Run("requires_authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "GET", "/v1/auth/me", "")
		h.Me(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", code)
		}
	})
}

func TestBalanceHandler(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	token, _ := mustLogin(t, h, "085555555555")

	t.Run("returns_zero_balance_for_new_user", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/balance", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		AuthMiddleware(testJWTSecret)(http.HandlerFunc(h.Balance)).ServeHTTP(w, r)

		code, resp := readResponse(t, w)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", code, resp)
		}
		// balance_sen should be 0 for new user.
		balance, ok := resp["balance_sen"].(float64)
		if !ok || int64(balance) != 0 {
			t.Errorf("expected balance_sen 0, got %v", balance)
		}
		version, ok := resp["version"].(float64)
		if !ok || int(version) < 1 {
			t.Errorf("expected version >= 1, got %v", version)
		}
	})

	t.Run("requires_authentication", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := mustRequest(t, "GET", "/v1/balance", "")
		h.Balance(w, req)

		code, _ := readResponse(t, w)
		if code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", code)
		}
	})
}

func TestErrorResponsesAreInIndonesian(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	tests := []struct {
		name       string
		handler    func(w http.ResponseWriter, r *http.Request)
		req        *http.Request
		wantCode   int
		wantErrMsg string // part of Indonesian message to check
	}{
		{
			name:       "register_empty_phone",
			handler:    h.Register,
			req:        mustRequest(t, "POST", "/v1/auth/register", `{"phone":"","pin":"123456"}`),
			wantCode:   http.StatusBadRequest,
			wantErrMsg: "wajib diisi",
		},
		{
			name:       "register_invalid_phone",
			handler:    h.Register,
			req:        mustRequest(t, "POST", "/v1/auth/register", `{"phone":"12345","pin":"123456"}`),
			wantCode:   http.StatusBadRequest,
			wantErrMsg: "nomor",
		},
		{
			name:       "register_short_pin",
			handler:    h.Register,
			req:        mustRequest(t, "POST", "/v1/auth/register", `{"phone":"081234567890","pin":"12"}`),
			wantCode:   http.StatusBadRequest,
			wantErrMsg: "PIN",
		},
		{
			name:       "login_wrong_pin",
			handler:    h.Login,
			req:        mustRequest(t, "POST", "/v1/auth/login", `{"phone":"089999999999","pin":"wrong"}`),
			wantCode:   http.StatusNotFound, // phone doesn't exist
			wantErrMsg: "tidak ditemukan",
		},
	}

	// Pre-register a user for login tests.
	w := httptest.NewRecorder()
	h.Register(w, mustRequest(t, "POST", "/v1/auth/register", `{"phone":"089999999991","pin":"123456"}`))
	if w.Code == http.StatusCreated {
		// Add a test for wrong PIN with existing user.
		tests = append(tests, struct {
			name       string
			handler    func(w http.ResponseWriter, r *http.Request)
			req        *http.Request
			wantCode   int
			wantErrMsg string
		}{
			name:       "login_wrong_pin_existing_user",
			handler:    h.Login,
			req:        mustRequest(t, "POST", "/v1/auth/login", `{"phone":"089999999991","pin":"999999"}`),
			wantCode:   http.StatusUnauthorized,
			wantErrMsg: "PIN",
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.handler(w, tt.req)

			code, resp := readResponse(t, w)
			if code != tt.wantCode {
				t.Fatalf("expected status %d, got %d: %v", tt.wantCode, code, resp)
			}
			errData, ok := resp["error"].(map[string]interface{})
			if !ok {
				t.Fatal("expected error in response")
			}
			msg, ok := errData["message"].(string)
			if !ok {
				t.Fatal("expected message in error")
			}
			if !strings.Contains(msg, tt.wantErrMsg) {
				t.Errorf("expected message containing %q, got %q", tt.wantErrMsg, msg)
			}
		})
	}
}

// TestPINNeverInResponse ensures PIN is never leaked in any API response.
func TestPINNeverInResponse(t *testing.T) {
	h, cleanup := setupTestHandler(t)
	defer cleanup()

	// Register and login.
	phone := "086666666666"
	pin := "123456"

	w := httptest.NewRecorder()
	h.Register(w, mustRequest(t, "POST", "/v1/auth/register", `{"phone":"`+phone+`","pin":"`+pin+`"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("register failed: %d", w.Code)
	}

	// Check register response body.
	var regResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&regResp)
	if _, ok := regResp["pin"]; ok {
		t.Error("register response must not contain pin")
	}
	if _, ok := regResp["pin_hash"]; ok {
		t.Error("register response must not contain pin_hash")
	}

	w = httptest.NewRecorder()
	h.Login(w, mustRequest(t, "POST", "/v1/auth/login", `{"phone":"`+phone+`","pin":"`+pin+`"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}
	var loginResp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&loginResp)
	if _, ok := loginResp["pin"]; ok {
		t.Error("login response must not contain pin")
	}
	if _, ok := loginResp["pin_hash"]; ok {
		t.Error("login response must not contain pin_hash")
	}
}

// TestAuthMiddleware verifies that the auth middleware correctly validates tokens.
func TestAuthMiddleware(t *testing.T) {
	secret := "middleware-test-secret"
	userID := uuid.Must(uuid.NewV7())

	// Create a test handler that returns the user ID from context.
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := UserIDFromContext(r.Context())
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"user_id": id.String()})
	})

	// Wrap with auth middleware.
	wrapped := AuthMiddleware(secret)(testHandler)

	t.Run("valid_token_passes", func(t *testing.T) {
		token, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth/me", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var resp map[string]string
		json.NewDecoder(w.Body).Decode(&resp)
		if resp["user_id"] != userID.String() {
			t.Errorf("expected user_id %s, got %s", userID.String(), resp["user_id"])
		}
	})

	t.Run("missing_header_returns_401", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth/me", nil)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("missing_bearer_prefix_returns_401", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth/me", nil)
		r.Header.Set("Authorization", userID.String()) // no Bearer prefix
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("expired_token_returns_401", func(t *testing.T) {
		// Create an expired token.
		now := time.Now()
		claims := CustomClaims{
			TokenType: TokenTypeAccess,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   userID.String(),
				IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth/me", nil)
		r.Header.Set("Authorization", "Bearer "+tokenStr)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("tampered_token_returns_401", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth/me", nil)
		r.Header.Set("Authorization", "Bearer tampered.jwt.here")
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("malformed_token_returns_401", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/auth/me", nil)
		r.Header.Set("Authorization", "Bearer not-a-jwt")
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
}
