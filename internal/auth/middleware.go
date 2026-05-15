package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"senpay/internal/types"

	"github.com/google/uuid"
)

// ContextKey is used for storing values in request context.
type ContextKey string

const (
	// CtxKeyUserID is the context key for the authenticated user's UUID.
	CtxKeyUserID ContextKey = "user_id"
)

// UserIDFromContext extracts the authenticated user's UUID from the request context.
// Returns zero UUID and false if not present.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(CtxKeyUserID).(uuid.UUID)
	return id, ok
}

// AuthMiddleware returns an HTTP middleware that validates JWT Bearer tokens.
//
// It extracts the token from the Authorization header, validates the JWT signature
// and expiry using the provided secret, and injects the user UUID into the request
// context via CtxKeyUserID.
//
// Returns 401 with Indonesian error message on:
//   - Missing Authorization header
//   - Missing Bearer prefix
//   - Invalid/expired/tampered token
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSONError(w, types.ErrUnauthorized)
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeJSONError(w, types.ErrUnauthorized)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == "" {
				writeJSONError(w, types.ErrUnauthorized)
				return
			}

			claims, err := ValidateToken(tokenString, secret)
			if err != nil {
				writeJSONError(w, types.ErrUnauthorized)
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				writeJSONError(w, types.ErrUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), CtxKeyUserID, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeJSONError writes a DomainError as a JSON response.
func writeJSONError(w http.ResponseWriter, err types.DomainError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.HTTPStatus)
	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    err.Code,
			"message": err.Message,
		},
	}); encodeErr != nil {
		slog.Error("failed to encode error response", "error", encodeErr)
	}
}

// writeJSONResponse writes a success JSON response with the given data.
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(data); encodeErr != nil {
		slog.Error("failed to encode response", "error", encodeErr)
	}
}
