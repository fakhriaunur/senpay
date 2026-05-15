package projection

import "senpay/internal/types"

// TxEntry represents a single transaction log entry for balance projection.
type TxEntry struct {
	Amount     types.Money
	SenderID   string
	ReceiverID string
	Status     string
}

// ProjectBalances computes a user's projected balance by summing
// committed credit entries minus committed debit entries.
//
// Rules:
//   - Only entries with Status "committed" are counted.
//   - Credit entries: user is the ReceiverID → amount added to balance.
//   - Debit entries: user is the SenderID → amount subtracted from balance.
//   - Empty txLog returns 0.
//
// This is a pure function: no I/O, no side effects, deterministic.
// The projection is a pure arithmetic sum — negative balances are allowed
// as this is a mathematical projection, not a balance constraint check.
func ProjectBalances(txLog []TxEntry, userID string) types.Money {
	var balance types.Money

	for _, entry := range txLog {
		if entry.Status != "committed" {
			continue
		}

		if entry.ReceiverID == userID {
			// Credit: add amount to balance.
			balance += entry.Amount
		}

		if entry.SenderID == userID {
			// Debit: subtract amount from balance.
			balance -= entry.Amount
		}
	}

	return balance
}
