package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetrics_RecordRequest(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("GET", "/health", 200, 100*time.Millisecond)
	m.RecordRequest("POST", "/v1/transfer", 500, 200*time.Millisecond)
	m.RecordRequest("GET", "/metrics", 200, 50*time.Millisecond)

	if m.requestCount.Load() != 3 {
		t.Errorf("expected requestCount=3, got %d", m.requestCount.Load())
	}
	if m.errorCount.Load() != 1 {
		t.Errorf("expected errorCount=1, got %d", m.errorCount.Load())
	}
}

func TestMetrics_MetricsEndpoint(t *testing.T) {
	m := NewMetrics()

	m.RecordRequest("GET", "/health", 200, 100*time.Millisecond)

	handler := m.MetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "senpay_request_count") {
		t.Errorf("expected senpay_request_count in body, got:\n%s", body)
	}
	if !strings.Contains(body, "senpay_request_duration_seconds") {
		t.Errorf("expected senpay_request_duration_seconds in body, got:\n%s", body)
	}
}

func TestMetrics_Middleware(t *testing.T) {
	m := NewMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify context values
		rid := GetRequestID(r.Context())
		if rid == "" {
			t.Error("expected non-empty request ID")
		}

		// Verify trace context
		tc := ExtractTraceContext(r)
		if tc.TraceID != "" {
			// If traceparent was sent, it should be extracted
			if tc.TraceID == "" || tc.SpanID == "" {
				t.Error("expected trace_id and span_id from traceparent")
			}
		}

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "test-request-id")
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// Verify response header
	if rid := rec.Header().Get("X-Request-ID"); rid != "test-request-id" {
		t.Errorf("expected X-Request-ID=test-request-id, got %s", rid)
	}
}
