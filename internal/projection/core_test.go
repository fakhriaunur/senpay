package projection

import (
	"math"
	"testing"

	"senpay/internal/types"

	"pgregory.net/rapid"
)

func TestProjectBalances(t *testing.T) {
	t.Parallel()

	maxMoney := types.Money(math.MaxInt64)

	tests := []struct {
		name   string
		txLog  []TxEntry
		userID string
		want   types.Money
	}{
		// ── Empty txLog ────────────────────────────────────────
		{name: "empty_log", txLog: []TxEntry{}, userID: "user-1", want: 0},

		// ── Single committed credit ────────────────────────────
		{name: "single_credit", txLog: []TxEntry{
			{Amount: 50000, SenderID: "other", ReceiverID: "user-1", Status: "committed"},
		}, userID: "user-1", want: 50000},

		// ── Single committed debit ─────────────────────────────
		{name: "single_debit", txLog: []TxEntry{
			{Amount: 25000, SenderID: "user-1", ReceiverID: "other", Status: "committed"},
		}, userID: "user-1", want: -25000}, // negative because debit reduces balance

		// ── Credit and debit ──────────────────────────────────
		{name: "credit_and_debit", txLog: []TxEntry{
			{Amount: 100000, SenderID: "other", ReceiverID: "user-1", Status: "committed"},
			{Amount: 30000, SenderID: "user-1", ReceiverID: "other", Status: "committed"},
		}, userID: "user-1", want: 70000},

		// ── Multiple credits ───────────────────────────────────
		{name: "multiple_credits", txLog: []TxEntry{
			{Amount: 50000, SenderID: "a", ReceiverID: "user-1", Status: "committed"},
			{Amount: 25000, SenderID: "b", ReceiverID: "user-1", Status: "committed"},
			{Amount: 10000, SenderID: "c", ReceiverID: "user-1", Status: "committed"},
		}, userID: "user-1", want: 85000},

		// ── Multiple debits ────────────────────────────────────
		{name: "multiple_debits", txLog: []TxEntry{
			{Amount: 10000, SenderID: "user-1", ReceiverID: "a", Status: "committed"},
			{Amount: 20000, SenderID: "user-1", ReceiverID: "b", Status: "committed"},
		}, userID: "user-1", want: -30000},

		// ── Mixed: only committed counted ──────────────────────
		{name: "committed_only_pending_excluded", txLog: []TxEntry{
			{Amount: 50000, SenderID: "a", ReceiverID: "user-1", Status: "committed"},
			{Amount: 30000, SenderID: "b", ReceiverID: "user-1", Status: "pending"},
			{Amount: 100000, SenderID: "c", ReceiverID: "user-1", Status: "failed"},
		}, userID: "user-1", want: 50000},

		{name: "committed_only_mixed_status", txLog: []TxEntry{
			{Amount: 100000, SenderID: "a", ReceiverID: "user-1", Status: "committed"},
			{Amount: 50000, SenderID: "user-1", ReceiverID: "b", Status: "committed"},
			{Amount: 20000, SenderID: "c", ReceiverID: "user-1", Status: "pending"},
			{Amount: 5000, SenderID: "user-1", ReceiverID: "d", Status: "failed"},
			{Amount: 10000, SenderID: "e", ReceiverID: "user-1", Status: "compensated"},
		}, userID: "user-1", want: 50000},

		// ── Unrelated entries (not involving user) ─────────────
		{name: "unrelated_entries_ignored", txLog: []TxEntry{
			{Amount: 50000, SenderID: "a", ReceiverID: "b", Status: "committed"},
			{Amount: 25000, SenderID: "c", ReceiverID: "d", Status: "committed"},
		}, userID: "user-1", want: 0},

		// ── User is both sender and receiver in same entry? Should not happen
		//    in practice, but if it does, debit takes precedence (processing order).
		{name: "user_as_both_sender_and_receiver", txLog: []TxEntry{
			{Amount: 50000, SenderID: "user-1", ReceiverID: "user-1", Status: "committed"},
		}, userID: "user-1", want: 0}, // credit +50000, debit -50000 = 0

		// ── Multiple entries with various statuses ─────────────
		{name: "complex_committed_only", txLog: []TxEntry{
			{Amount: 1000000, SenderID: "topup-bank", ReceiverID: "user-1", Status: "committed"},
			{Amount: 250000, SenderID: "user-1", ReceiverID: "merchant-a", Status: "committed"},
			{Amount: 150000, SenderID: "user-1", ReceiverID: "merchant-b", Status: "committed"},
			{Amount: 500000, SenderID: "topup-bank", ReceiverID: "user-1", Status: "pending"},
			{Amount: 100000, SenderID: "user-1", ReceiverID: "merchant-c", Status: "pending"},
		}, userID: "user-1", want: 600000}, // 1000000 - 250000 - 150000

		// ── Single entry, user is sender ───────────────────────
		{name: "user_is_sender_debit", txLog: []TxEntry{
			{Amount: 75000, SenderID: "user-1", ReceiverID: "other", Status: "committed"},
		}, userID: "user-1", want: -75000},

		// ── Max int64 values ──────────────────────────────────
		{name: "max_int64_credit", txLog: []TxEntry{
			{Amount: maxMoney, SenderID: "other", ReceiverID: "user-1", Status: "committed"},
		}, userID: "user-1", want: maxMoney},

		// ── Zero amount entries ────────────────────────────────
		{name: "zero_amount_entries", txLog: []TxEntry{
			{Amount: 0, SenderID: "other", ReceiverID: "user-1", Status: "committed"},
			{Amount: 0, SenderID: "user-1", ReceiverID: "other", Status: "committed"},
		}, userID: "user-1", want: 0},

		// ── Net zero after multiple entries ────────────────────
		{name: "net_zero", txLog: []TxEntry{
			{Amount: 100000, SenderID: "a", ReceiverID: "user-1", Status: "committed"},
			{Amount: 100000, SenderID: "user-1", ReceiverID: "b", Status: "committed"},
		}, userID: "user-1", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProjectBalances(tt.txLog, tt.userID)
			if got != tt.want {
				t.Errorf("ProjectBalances(%v, %q) = %d, want %d", tt.txLog, tt.userID, got, tt.want)
			}
		})
	}
}

func TestProjectBalances_UserNotInLog(t *testing.T) {
	t.Parallel()

	// Verify that a user not appearing in any entry gets balance 0.
	txLog := []TxEntry{
		{Amount: 50000, SenderID: "alice", ReceiverID: "bob", Status: "committed"},
		{Amount: 30000, SenderID: "bob", ReceiverID: "charlie", Status: "committed"},
	}
	got := ProjectBalances(txLog, "dave")
	if got != 0 {
		t.Errorf("expected 0 for user not in log, got %d", got)
	}
}

func TestProjectBalances_AllStatuses(t *testing.T) {
	t.Parallel()

	// Test all possible status values for credit entries.
	statuses := []struct {
		status string
		counts bool
	}{
		{"committed", true},
		{"pending", false},
		{"failed", false},
		{"compensated", false},
		{"", false},
	}

	for _, s := range statuses {
		t.Run("status_"+s.status, func(t *testing.T) {
			txLog := []TxEntry{
				{Amount: 50000, SenderID: "other", ReceiverID: "user-1", Status: s.status},
			}
			got := ProjectBalances(txLog, "user-1")
			if s.counts && got != 50000 {
				t.Errorf("expected 50000 for committed, got %d", got)
			}
			if !s.counts && got != 0 {
				t.Errorf("expected 0 for %s, got %d", s.status, got)
			}
		})
	}
}

// ── Rapid property-based tests ──────────────────────────────────

func TestProperty_ProjectBalances_EmptyLog(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "userID")

		balance := ProjectBalances([]TxEntry{}, userID)
		if balance != 0 {
			t.Fatalf("empty log should yield 0, got %d", balance)
		}
	})
}

func TestProperty_ProjectBalances_PendingAndFailedExcluded(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "userID")
		n := rapid.IntRange(0, 20).Draw(t, "numEntries")

		entries := make([]TxEntry, n)
		var expectedBalance types.Money
		otherID := "counterparty-" + userID

		for i := 0; i < n; i++ {
			amount := types.Money(rapid.Int64Range(1, 10000000).Draw(t, "amount"))
			// Alternate between committed and non-committed.
			isCommitted := rapid.IntRange(0, 1).Draw(t, "isCommitted") == 0
			isCredit := rapid.IntRange(0, 1).Draw(t, "isCredit") == 0
			status := "pending"
			if isCommitted {
				status = "committed"
			}

			var sender, receiver string
			if isCredit {
				sender = otherID
				receiver = userID
			} else {
				sender = userID
				receiver = otherID
			}

			entries[i] = TxEntry{
				Amount:     amount,
				SenderID:   sender,
				ReceiverID: receiver,
				Status:     status,
			}

			if isCommitted {
				if isCredit {
					expectedBalance += amount
				} else {
					expectedBalance -= amount
				}
			}
		}

		balance := ProjectBalances(entries, userID)
		if balance != expectedBalance {
			t.Fatalf("ProjectBalances = %d, want %d (based on committed entries only)", balance, expectedBalance)
		}
	})
}

func TestProperty_ProjectBalances_PendingFailedNeverAffect(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "userID")
		otherID := "other-" + userID

		n := rapid.IntRange(1, 10).Draw(t, "numEntries")
		entries := make([]TxEntry, n)

		for i := 0; i < n; i++ {
			amount := types.Money(rapid.Int64Range(1, 1000000).Draw(t, "amount"))
			status := rapid.SampledFrom([]string{
				"pending", "failed", "compensated", "",
			}).Draw(t, "status")
			isCredit := rapid.IntRange(0, 1).Draw(t, "isCredit") == 0

			if isCredit {
				entries[i] = TxEntry{Amount: amount, SenderID: otherID, ReceiverID: userID, Status: status}
			} else {
				entries[i] = TxEntry{Amount: amount, SenderID: userID, ReceiverID: otherID, Status: status}
			}
		}

		// None of these entries are committed, so balance must be 0.
		balance := ProjectBalances(entries, userID)
		if balance != 0 {
			t.Fatalf("expected 0 when no committed entries, got %d", balance)
		}
	})
}

func TestProperty_ProjectBalances_UnrelatedEntriesIgnored(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "userID")
		n := rapid.IntRange(1, 20).Draw(t, "numEntries")

		entries := make([]TxEntry, n)
		for i := 0; i < n; i++ {
			amount := types.Money(rapid.Int64Range(1, 1000000).Draw(t, "amount"))
			// Generate sender and receiver that are BOTH different from userID.
			sender := "user-" + rapid.StringMatching(`[a-z0-9]{4}`).Draw(t, "sender")
			receiver := "user-" + rapid.StringMatching(`[a-z0-9]{4}`).Draw(t, "receiver")
			status := rapid.SampledFrom([]string{"committed", "pending", "failed"}).Draw(t, "status")

			entries[i] = TxEntry{
				Amount:     amount,
				SenderID:   sender,
				ReceiverID: receiver,
				Status:     status,
			}
		}

		// None of the entries involve userID, so balance must be 0.
		balance := ProjectBalances(entries, userID)
		if balance != 0 {
			t.Fatalf("expected 0 when no entries involve user, got %d", balance)
		}
	})
}

func TestProperty_ProjectBalances_OnlyCommittedCounted(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`[a-zA-Z0-9\-]{8,36}`).Draw(t, "userID")
		otherID := "other-" + userID

		// Create a mix of committed and non-committed entries.
		entries := []TxEntry{
			{Amount: 100000, SenderID: otherID, ReceiverID: userID, Status: "committed"},
			{Amount: 50000, SenderID: otherID, ReceiverID: userID, Status: "pending"},
			{Amount: 25000, SenderID: otherID, ReceiverID: userID, Status: "failed"},
			{Amount: 10000, SenderID: otherID, ReceiverID: userID, Status: "compensated"},
		}

		balance := ProjectBalances(entries, userID)
		if balance != 100000 {
			t.Fatalf("expected 100000 (only committed), got %d", balance)
		}
	})
}
