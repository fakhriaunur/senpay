package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"senpay/internal/auth"
	"senpay/internal/types"

	"github.com/google/uuid"
)

// contextKey is used for storing values in request context.
type contextKey string

const (
	// CtxKeyRequestID is the context key for the request ID.
	CtxKeyRequestID contextKey = "request_id"

	// BILimitBasicSen is the per-transaction limit for basic KYC users in sen.
	// Rp 2.000.000 = 200,000,000 sen.
	BILimitBasicSen types.Money = 200_000_000

	// BILimitVerifiedSen is the per-transaction limit for verified KYC users in sen.
	// Rp 10.000.000 = 1,000,000,000 sen.
	BILimitVerifiedSen types.Money = 1_000_000_000
)

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(CtxKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader captures the status code before delegating.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Write delegates to the wrapped ResponseWriter, ensuring a default status code.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// --- Middleware ---

// Recovery recovers from panics in downstream handlers.
// Logs the panic and stack trace, returns 500 Internal Server Error.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				requestID := GetRequestID(r.Context())
				slog.Error("panic recovered",
					"request_id", requestID,
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(stack),
					"method", r.Method,
					"path", r.URL.Path,
				)
				writeJSONError(w, types.ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID injects a request ID into the request context and response headers.
// Uses the X-Request-ID header from the incoming request if present,
// otherwise generates a UUID v7. Sets the X-Request-ID response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.Must(uuid.NewV7()).String()
		}

		ctx := context.WithValue(r.Context(), CtxKeyRequestID, requestID)
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logging logs request method, path, status, duration, and request_id
// at INFO level for every request.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		requestID := GetRequestID(r.Context())

		next.ServeHTTP(ww, r)

		duration := time.Since(start)
		statusCode := ww.statusCode

		attrs := []slog.Attr{
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", statusCode),
			slog.Int64("duration_ms", duration.Milliseconds()),
		}

		level := slog.LevelInfo
		if statusCode >= 500 {
			level = slog.LevelError
		} else if statusCode >= 400 {
			level = slog.LevelWarn
		}
		slog.LogAttrs(r.Context(), level, "request completed", attrs...)
	})
}

// RateLimit returns middleware that rate-limits requests per key.
// The key is derived from the client IP and the request endpoint.
// Returns 429 Too Many Requests with Retry-After header when rate limited.
func RateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Build a rate limit key from IP + method + path.
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = forwarded
			}
			key := ip + ":" + r.Method + ":" + r.URL.Path

			ok, retryAfter := rl.Allow(key)
			if !ok {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{
						"code":    "RATE_LIMITED",
						"message": "Terlalu banyak permintaan, silakan coba lagi",
					},
				}); encodeErr != nil {
					slog.Error("failed to encode rate-limit response", "error", encodeErr)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// UserStore defines the minimal interface needed for BI limit enforcement.
type UserStore interface {
	FindByID(ctx context.Context, id uuid.UUID) (types.User, error)
}

// BILimit returns middleware that enforces BI transaction limits based on
// the user's KYC level. Requires auth middleware to have injected the user ID
// into the request context.
//
// Basic KYC: max Rp 2.000.000 (200,000,000 sen) per transaction.
// Verified KYC: max Rp 10.000.000 (1,000,000,000 sen) per transaction.
//
// Only applies to POST and PUT requests containing an amount_sen field.
func BILimit(store UserStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only check money-related requests.
			if r.Method != http.MethodPost && r.Method != http.MethodPut {
				next.ServeHTTP(w, r)
				return
			}

			// Read and buffer the body so we can re-use it.
			body, err := io.ReadAll(r.Body)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = io.NopCloser(bytes.NewBuffer(body))

			// Parse amount_sen from body.
			var req struct {
				AmountSen int64 `json:"amount_sen"`
			}
			if err := json.Unmarshal(body, &req); err != nil || req.AmountSen <= 0 {
				// No amount field present or invalid — let the handler validate.
				next.ServeHTTP(w, r)
				return
			}

			amountSen := types.Money(req.AmountSen)

			// Get user ID from context (set by auth middleware).
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				// Not authenticated — let auth middleware handle it.
				next.ServeHTTP(w, r)
				return
			}

			// Look up user's KYC level.
			user, err := store.FindByID(r.Context(), userID)
			if err != nil {
				// User not found — let downstream handle.
				next.ServeHTTP(w, r)
				return
			}

			// Determine limit based on KYC level.
			var limit types.Money
			switch user.KYCLevel {
			case types.KYCLevelVerified:
				limit = BILimitVerifiedSen
			default: // basic or unknown
				limit = BILimitBasicSen
			}

			// Enforce limit.
			if amountSen > limit {
				writeJSONError(w, types.ErrExceedsTransactionLimit)
				return
			}

			next.ServeHTTP(w, r)
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
