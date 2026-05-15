package idempotency

// Decision represents the outcome of an idempotency check.
type Decision int

const (
	// Proceed means no prior request found — caller should execute the operation.
	Proceed Decision = iota
	// Duplicate means a completed request with this key already exists.
	Duplicate
	// InFlight means a request with this key is currently being processed.
	InFlight
)

// String returns a human-readable representation of the Decision.
func (d Decision) String() string {
	switch d {
	case Proceed:
		return "proceed"
	case Duplicate:
		return "duplicate"
	case InFlight:
		return "in_flight"
	default:
		return "unknown"
	}
}

// Check determines the idempotency decision based on the stored status.
//
// Status mapping:
//   - "" (empty)     → Proceed   — no prior request, caller should execute
//   - "completed"    → Duplicate — a completed result exists, return cached
//   - "in_flight"    → InFlight  — request in progress, return HTTP 202
//   - any other      → Proceed   — unknown status, treat as no prior
//
// The key parameter is informational for logging/debugging; the decision
// is based solely on the status parameter.
func Check(key string, status string) Decision {
	switch status {
	case "":
		return Proceed
	case "completed":
		return Duplicate
	case "in_flight":
		return InFlight
	default:
		return Proceed
	}
}
