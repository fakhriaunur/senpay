package types

import (
	"time"

	"github.com/google/uuid"
)

// Transaction represents a financial transaction in the system.
type Transaction struct {
	ID             uuid.UUID  `json:"id"`
	IdempotencyKey string     `json:"idempotency_key"`
	TxType         string     `json:"tx_type"`
	SenderID       *uuid.UUID `json:"sender_id,omitempty"`
	ReceiverID     *uuid.UUID `json:"receiver_id,omitempty"`
	AmountSen      int64      `json:"amount_sen"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	FailureReason  *string    `json:"failure_reason,omitempty"`
	Category       string     `json:"category,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CommittedAt    *time.Time `json:"committed_at,omitempty"`
}

// Default category for transactions without an explicit category.
const CategoryDefault = "Lainnya"

// Transaction types.
const (
	TxTypeTopup    = "topup"
	TxTypeTransfer = "transfer"
	TxTypeWithdraw = "withdraw"
	TxTypeFee      = "fee"
)

// Transaction statuses.
const (
	TxStatusPending     = "pending"
	TxStatusCommitted   = "committed"
	TxStatusFailed      = "failed"
	TxStatusCompensated = "compensated"
	TxStatusTimeout     = "timeout"
)

// Currency.
const (
	CurrencyIDR = "IDR"
)

// NewTransaction creates a new Transaction with a UUID v7 ID.
func NewTransaction(txType, idempotencyKey string, amountSen int64, senderID, receiverID *uuid.UUID) Transaction {
	return Transaction{
		ID:             uuid.Must(uuid.NewV7()),
		IdempotencyKey: idempotencyKey,
		TxType:         txType,
		SenderID:       senderID,
		ReceiverID:     receiverID,
		AmountSen:      amountSen,
		Currency:       CurrencyIDR,
		Status:         TxStatusPending,
		CreatedAt:      time.Now().UTC(),
	}
}
