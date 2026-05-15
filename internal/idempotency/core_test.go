package idempotency

import (
	"testing"

	"pgregory.net/rapid"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		key    string
		status string
		want   Decision
	}{
		// ── No prior request ───────────────────────────────────
		{name: "empty_key_empty_status", key: "", status: "", want: Proceed},
		{name: "valid_key_no_prior", key: "test-key-001", status: "", want: Proceed},
		{name: "spaceship_key_no_prior", key: "test-key-002", status: "", want: Proceed},

		// ── Duplicate (completed) ──────────────────────────────
		{name: "completed_status", key: "key-1", status: "completed", want: Duplicate},
		{name: "completed_with_uuid_key", key: "test-key-003", status: "completed", want: Duplicate},

		// ── In-flight ──────────────────────────────────────────
		{name: "in_flight_status", key: "key-1", status: "in_flight", want: InFlight},
		{name: "in_flight_with_uuid_key", key: "test-key-004", status: "in_flight", want: InFlight},

		// ── Edge cases ─────────────────────────────────────────
		{name: "unknown_status_proceed", key: "key-1", status: "unknown", want: Proceed},
		{name: "pending_status_proceed", key: "key-1", status: "pending", want: Proceed},
		{name: "failed_status_proceed", key: "key-1", status: "failed", want: Proceed},
		{name: "empty_key_completed", key: "", status: "completed", want: Duplicate},
		{name: "empty_key_in_flight", key: "", status: "in_flight", want: InFlight},
		{name: "whitespace_status", key: "key-1", status: "  ", want: Proceed},
		{name: "case_sensitive", key: "key-1", status: "COMPLETED", want: Proceed},
		{name: "completed_with_extra", key: "key-1", status: "completed ", want: Proceed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.key, tt.status)
			if got != tt.want {
				t.Errorf("Check(%q, %q) = %v (%s), want %v (%s)",
					tt.key, tt.status, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestDecision_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		decision Decision
		want     string
	}{
		{Proceed, "proceed"},
		{Duplicate, "duplicate"},
		{InFlight, "in_flight"},
		{Decision(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.decision.String(); got != tt.want {
				t.Errorf("Decision(%d).String() = %q, want %q", tt.decision, got, tt.want)
			}
		})
	}
}

// ── Rapid property-based tests ──────────────────────────────────

func TestProperty_Check_Proceed(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		key := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "key")

		// Empty status should always return Proceed.
		result := Check(key, "")
		if result != Proceed {
			t.Fatalf("Check(%q, '') = %v, want Proceed", key, result)
		}
	})
}

func TestProperty_Check_Duplicate(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		key := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "key")

		// "completed" status should always return Duplicate.
		result := Check(key, "completed")
		if result != Duplicate {
			t.Fatalf("Check(%q, 'completed') = %v, want Duplicate", key, result)
		}
	})
}

func TestProperty_Check_InFlight(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		key := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "key")

		// "in_flight" status should always return InFlight.
		result := Check(key, "in_flight")
		if result != InFlight {
			t.Fatalf("Check(%q, 'in_flight') = %v, want InFlight", key, result)
		}
	})
}

func TestProperty_Check_KeyIrrelevant(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		key1 := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "key1")
		key2 := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "key2")
		statuses := []string{"", "completed", "in_flight", "unknown", "pending", "failed"}
		status := rapid.SampledFrom(statuses).Draw(t, "status")

		// The key should not affect the decision; only status matters.
		result1 := Check(key1, status)
		result2 := Check(key2, status)
		if result1 != result2 {
			t.Fatalf("Check with different keys but same status (%q) returned different results: %v vs %v",
				status, result1, result2)
		}
	})
}
