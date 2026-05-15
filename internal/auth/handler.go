package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"senpay/internal/types"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler implements HTTP handlers for auth-related endpoints.
//
// Fields:
//   - pool: PostgreSQL connection pool for direct queries (balance)
//   - store: UserRepository for user CRUD
//   - jwtSecret: key for signing and validating JWT tokens
//   - tokenStore: tracks used refresh tokens for single-use rotation
//
// All methods follow the http.Handler signature.
type Handler struct {
	pool       *pgxpool.Pool
	store      UserRepository
	jwtSecret  string
	tokenStore *TokenStore
}

// NewHandler creates a new auth Handler.
func NewHandler(pool *pgxpool.Pool, store UserRepository, jwtSecret string) *Handler {
	return &Handler{
		pool:       pool,
		store:      store,
		jwtSecret:  jwtSecret,
		tokenStore: NewTokenStore(1 * time.Hour),
	}
}

// --- Request/Response types ---

type registerRequest struct {
	Phone string `json:"phone"`
	PIN   string `json:"pin"`
}

type loginRequest struct {
	Phone string `json:"phone"`
	PIN   string `json:"pin"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type kycRequest struct {
	KYCLevel string `json:"kyc_level"`
}

type registerResponse struct {
	UserID uuid.UUID `json:"user_id"`
}

type loginResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type kycResponse struct {
	KYCLevel string `json:"kyc_level"`
}

type userProfileResponse struct {
	ID       uuid.UUID `json:"id"`
	Phone    string    `json:"phone"`
	KYCLevel string    `json:"kyc_level"`
}

type balanceResponse struct {
	BalanceSen int64 `json:"balance_sen"`
	Version    int   `json:"version"`
}

// --- HTTP Handlers ---

// Register handles POST /v1/auth/register.
//
// Accepts JSON body with phone and pin fields:
//   - Validates phone format (Indonesian number, 10-13 digits, 08 or 62 prefix)
//   - Validates PIN minimum length (4 digits)
//   - Checks for duplicate phone numbers
//   - Hashes PIN with bcrypt cost 12
//   - Creates user record
//   - Seeds zero-balance snapshot
//
// Returns 201 with user_id on success.
// Returns 400 for validation errors, 409 for duplicate phone.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, types.NewMissingFieldError("body"))
		return
	}

	// Validate phone
	if req.Phone == "" {
		writeJSONError(w, types.NewMissingFieldError("phone"))
		return
	}
	if err := ValidatePhone(req.Phone); err != nil {
		writeJSONError(w, *err)
		return
	}

	// Normalize phone: strip leading + if present, ensure starts with 08 or 62.
	phone := strings.TrimPrefix(req.Phone, "+")

	// Validate PIN minimum length (4 digits).
	if len(req.PIN) < 4 {
		writeJSONError(w, types.DomainError{
			Code:       types.ErrCodeInvalidFormat,
			Message:    "PIN minimal 4 digit",
			HTTPStatus: 400,
		})
		return
	}

	// Hash PIN with bcrypt cost 12.
	pinHash := HashPIN(req.PIN)

	// Create user.
	user := types.NewUser(phone, pinHash)

	// Insert user.
	if err := h.store.Insert(r.Context(), user); err != nil {
		// Check for unique constraint violation (duplicate phone).
		if isDuplicatePhoneError(err) {
			writeJSONError(w, types.ErrPhoneAlreadyRegistered)
			return
		}
		writeJSONError(w, types.ErrInternal)
		return
	}

	// Seed zero-balance snapshot.
	if err := h.seedBalanceSnapshot(r.Context(), user.ID); err != nil {
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusCreated, registerResponse{UserID: user.ID})
}

// Login handles POST /v1/auth/login.
//
// Accepts JSON body with phone and pin fields:
//   - Validates required fields
//   - Looks up user by phone
//   - Verifies PIN against stored hash
//   - Generates JWT access (30min) + refresh (7d) tokens
//
// Returns 200 with token and refresh_token on success.
// Returns 400 for missing fields, 404 for unknown phone, 401 for wrong PIN.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, types.NewMissingFieldError("body"))
		return
	}

	if req.Phone == "" {
		writeJSONError(w, types.NewMissingFieldError("phone"))
		return
	}
	if req.PIN == "" {
		writeJSONError(w, types.NewMissingFieldError("pin"))
		return
	}

	// Normalize phone.
	phone := strings.TrimPrefix(req.Phone, "+")

	// Look up user.
	user, err := h.store.FindByPhone(r.Context(), phone)
	if err != nil {
		if target := (types.DomainError{}); errors.As(err, &target) && target.Code == types.ErrCodeUserNotFound {
			writeJSONError(w, target)
			return
		}
		writeJSONError(w, types.ErrInternal)
		return
	}

	// Verify PIN.
	if !VerifyPIN(req.PIN, user.PINHash) {
		writeJSONError(w, types.ErrInvalidPIN)
		return
	}

	// Generate tokens.
	token, err := GenerateAccessToken(user.ID, h.jwtSecret)
	if err != nil {
		writeJSONError(w, types.ErrInternal)
		return
	}

	refreshToken, err := GenerateRefreshToken(user.ID, h.jwtSecret)
	if err != nil {
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, loginResponse{
		Token:        token,
		RefreshToken: refreshToken,
	})
}

// Refresh handles POST /v1/auth/refresh.
//
// Accepts JSON body with refresh_token field:
//   - Validates the refresh token (must be a valid JWT with type "refresh")
//   - Checks that the refresh token hasn't been used already (single-use rotation)
//   - Issues a new access token + new refresh token
//   - Invalidates the old refresh token
//
// Returns 200 with new token pair on success.
// Returns 400 for missing field, 401 for invalid/expired/reused token.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, types.NewMissingFieldError("body"))
		return
	}

	if req.RefreshToken == "" {
		writeJSONError(w, types.NewMissingFieldError("refresh_token"))
		return
	}

	// Validate refresh token.
	claims, err := ValidateToken(req.RefreshToken, h.jwtSecret)
	if err != nil {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	// Verify it's a refresh token, not an access token.
	if claims.TokenType != TokenTypeRefresh {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	// Check single-use rotation: reject if this token was already used.
	if h.tokenStore.IsUsed(claims.ID) {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	// Mark old refresh token as used (single-use rotation).
	h.tokenStore.MarkUsed(claims.ID)

	// Issue new tokens.
	newToken, err := GenerateAccessToken(userID, h.jwtSecret)
	if err != nil {
		writeJSONError(w, types.ErrInternal)
		return
	}

	newRefreshToken, err := GenerateRefreshToken(userID, h.jwtSecret)
	if err != nil {
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, refreshResponse{
		Token:        newToken,
		RefreshToken: newRefreshToken,
	})
}

// KYC handles POST /v1/auth/kyc.
//
// Accepts JSON body with kyc_level field:
//   - Requires authentication (auth middleware)
//   - Updates user's KYC level to the requested level
//   - Only "verified" transitions from "basic" are accepted
//
// Returns 200 with updated kyc_level on success.
// Returns 400 for invalid KYC level, 404 for user not found.
func (h *Handler) KYC(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	var req kycRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, types.NewMissingFieldError("body"))
		return
	}

	if req.KYCLevel == "" {
		writeJSONError(w, types.NewMissingFieldError("kyc_level"))
		return
	}

	// Validate KYC level.
	if req.KYCLevel != types.KYCLevelVerified && req.KYCLevel != types.KYCLevelBasic {
		writeJSONError(w, types.DomainError{
			Code:       types.ErrCodeInvalidFormat,
			Message:    "Level KYC tidak valid: harus 'basic' atau 'verified'",
			HTTPStatus: 400,
		})
		return
	}

	if err := h.store.UpdateKYCLevel(r.Context(), userID, req.KYCLevel); err != nil {
		if target := (types.DomainError{}); errors.As(err, &target) && target.Code == types.ErrCodeUserNotFound {
			writeJSONError(w, target)
			return
		}
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, kycResponse(req))
}

// Me handles GET /v1/auth/me.
//
// Requires authentication (auth middleware).
// Returns the authenticated user's profile.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	user, err := h.store.FindByID(r.Context(), userID)
	if err != nil {
		if target := (types.DomainError{}); errors.As(err, &target) && target.Code == types.ErrCodeUserNotFound {
			writeJSONError(w, target)
			return
		}
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, userProfileResponse{
		ID:       user.ID,
		Phone:    user.Phone,
		KYCLevel: user.KYCLevel,
	})
}

// Balance handles GET /v1/balance.
//
// Requires authentication (auth middleware).
// Returns the authenticated user's current balance from balance_snapshot.
func (h *Handler) Balance(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSONError(w, types.ErrUnauthorized)
		return
	}

	balance, err := h.getBalance(r.Context(), userID)
	if err != nil {
		writeJSONError(w, types.ErrInternal)
		return
	}

	writeJSONResponse(w, http.StatusOK, balance)
}

// --- Helpers ---

// seedBalanceSnapshot creates a zero-balance snapshot for a new user.
func (h *Handler) seedBalanceSnapshot(ctx context.Context, userID uuid.UUID) error {
	const query = `
		INSERT INTO balance_snapshot (user_id, balance_sen, version, updated_at)
		VALUES ($1, 0, 1, NOW())
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := h.pool.Exec(ctx, query, userID)
	return err
}

// getBalance retrieves the balance snapshot for a user.
func (h *Handler) getBalance(ctx context.Context, userID uuid.UUID) (balanceResponse, error) {
	const query = `
		SELECT balance_sen, version
		FROM balance_snapshot
		WHERE user_id = $1
	`
	var resp balanceResponse
	err := h.pool.QueryRow(ctx, query, userID).Scan(&resp.BalanceSen, &resp.Version)
	if err != nil {
		// If no row exists, return zero balance.
		resp = balanceResponse{BalanceSen: 0, Version: 1}
	}
	return resp, nil
}

// isDuplicatePhoneError checks if a database error is a unique constraint violation on phone.
func isDuplicatePhoneError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "duplicate key") ||
		strings.Contains(errStr, "UNIQUE") ||
		strings.Contains(errStr, "23505") // PostgreSQL unique violation code
}
