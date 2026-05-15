package telemetry

import (
	"context"
	"net/http"
	"time"
)

type contextKey string

const (
	ctxKeyTraceID contextKey = "trace_id"
)

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
// - W3C trace context extraction from headers
// - Metrics recording (request count, duration, errors)
//
// Request ID injection and structured logging are handled by the gateway middleware.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract trace context
		tc := ExtractTraceContext(r)
		traceID := tc.TraceID
		if traceID != "" {
			ctx := context.WithValue(r.Context(), ctxKeyTraceID, traceID)
			r = r.WithContext(ctx)
		}

		// Create response writer wrapper to capture status code
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next.ServeHTTP(ww, r)

		// Calculate duration
		duration := time.Since(start)
		statusCode := ww.statusCode

		// Record metrics
		m.RecordRequest(r.Method, r.URL.Path, statusCode, duration)
	})
}
