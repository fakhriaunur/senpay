package ledger

import (
	"senpay/internal/types"
)

// TxEntry represents one side of a financial transaction entry.
type TxEntry struct {
	TxType string      // "debit" for sender loss, "credit" for receiver gain
	Amount types.Money
}

// TxEvent represents the result of executing a transfer.
type TxEvent struct {
	Debit  TxEntry // the debit side (sender loses money)
	Credit TxEntry // the credit side (receiver gains money)
}

// ExecuteTransfer executes a transfer between two parties.
// Returns a TxEvent with debit/credit entries, or a DomainError.
// Never mutates input parameters. Total money invariant preserved:
// senderBalance + receiverBalance == newSenderBalance + newReceiverBalance.
//
// Errors:
//   - ErrInvalidAmount when amount <= 0
//   - ErrInsufficientBalance when senderBalance < amount
func ExecuteTransfer(senderBalance, receiverBalance, amount types.Money) (TxEvent, *types.DomainError) {
	if !amount.IsPositive() {
		return TxEvent{}, &types.ErrInvalidAmount
	}
	if senderBalance < amount {
		return TxEvent{}, &types.ErrInsufficientBalance
	}
	return TxEvent{
		Debit:  TxEntry{TxType: "debit", Amount: amount},
		Credit: TxEntry{TxType: "credit", Amount: amount},
	}, nil
}
