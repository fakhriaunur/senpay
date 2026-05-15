package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const (
	ctxKeyRequestID contextKey = "request_id"
	ctxKeyTraceID   contextKey = "trace_id"
)

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// GetTraceID extracts the trace ID from context.
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKeyTraceID).(string); ok {
		return id
	}
	return ""
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before delegating.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Middleware returns an HTTP handler that wraps the next handler with:
// - Request ID injection (from X-Request-ID header or generated UUID v7)
// - W3C trace context extraction from headers
// - Structured request/response logging via slog
// - Metrics recording (request count, duration, errors)
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract or generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.Must(uuid.NewV7()).String()
		}

		// Extract trace context
		tc := ExtractTraceContext(r)
		traceID := tc.TraceID
		if traceID == "" {
			traceID = "" // no trace context from upstream
		}

		// Store in context
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, requestID)
		ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)
		r = r.WithContext(ctx)

		// Set response header
		w.Header().Set("X-Request-ID", requestID)

		// Create response writer wrapper
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Log request start
		attrs := []slog.Attr{
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("query", r.URL.RawQuery),
		}
		if traceID != "" {
			attrs = append(attrs, slog.String("trace_id", traceID))
		}
		slog.LogAttrs(ctx, slog.LevelInfo, "request started", attrs...)

		// Process request
		next.ServeHTTP(ww, r)

		// Calculate duration
		duration := time.Since(start)
		statusCode := ww.statusCode

		// Record metrics
		m.RecordRequest(r.Method, r.URL.Path, statusCode, duration)

		// Log request completion
		respAttrs := []slog.Attr{
			slog.String("request_id", requestID),
			slog.Int("status", statusCode),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
		}
		if traceID != "" {
			respAttrs = append(respAttrs, slog.String("trace_id", traceID))
		}

		level := slog.LevelInfo
		if statusCode >= 500 {
			level = slog.LevelError
		} else if statusCode >= 400 {
			level = slog.LevelWarn
		}
		slog.LogAttrs(ctx, level, "request completed", respAttrs...)
	})
}
