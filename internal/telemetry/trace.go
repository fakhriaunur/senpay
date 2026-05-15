package telemetry

import (
	"net/http"
)

// TraceContext holds W3C trace context fields.
type TraceContext struct {
	TraceID    string
	SpanID     string
	TraceFlags byte
}

const (
	// TraceParentHeader is the W3C trace context header.
	TraceParentHeader = "traceparent"
	// TraceStateHeader is the W3C trace state header.
	TraceStateHeader = "tracestate"

	// TraceParentFormat: "00-{trace_id}-{span_id}-{trace_flags}"
	traceParentLength = 55
	traceIDStart      = 3
	traceIDEnd        = 35
	spanIDStart       = 36
	spanIDEnd         = 52
)

// ExtractTraceContext extracts W3C trace context from HTTP headers.
// Returns parsed TraceContext if traceparent header is present and valid,
// or empty TraceContext if absent or malformed.
func ExtractTraceContext(r *http.Request) TraceContext {
	tp := r.Header.Get(TraceParentHeader)
	if tp == "" {
		return TraceContext{}
	}

	return ParseTraceParent(tp)
}

// ParseTraceParent parses a W3C traceparent header value.
// Format: "00-{trace_id}-{span_id}-{trace_flags}"
func ParseTraceParent(tp string) TraceContext {
	if len(tp) < traceParentLength {
		return TraceContext{}
	}

	tc := TraceContext{
		TraceID:    tp[traceIDStart:traceIDEnd],
		SpanID:     tp[spanIDStart:spanIDEnd],
		TraceFlags: 0,
	}

	if len(tp) >= 56 {
		tc.TraceFlags = tp[53] - '0'
	}

	return tc
}

// NewTraceParent creates a W3C traceparent header value from trace and span IDs.
func NewTraceParent(traceID, spanID string) string {
	if len(traceID) != 32 {
		return ""
	}
	if len(spanID) != 16 {
		return ""
	}
	return "00-" + traceID + "-" + spanID + "-01"
}

// PropagateTraceContext returns an HTTP header set with the given trace context
// for outbound requests.
func PropagateTraceContext(tc TraceContext) http.Header {
	h := http.Header{}
	if tc.TraceID != "" && tc.SpanID != "" {
		h.Set(TraceParentHeader, NewTraceParent(tc.TraceID, tc.SpanID))
	}
	return h
}
