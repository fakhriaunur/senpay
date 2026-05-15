package types

import (
	"time"

	"github.com/google/uuid"
)

// BalanceSnapshot represents a user's balance at a point in time.
// Balance is projected from tx_log (sum of committed entries) and stored
// with an optimistic lock via the Version field.
type BalanceSnapshot struct {
	UserID    uuid.UUID `json:"user_id"`
	BalanceSen int64    `json:"balance_sen"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewBalanceSnapshot creates a new BalanceSnapshot with zero balance.
func NewBalanceSnapshot(userID uuid.UUID) BalanceSnapshot {
	return BalanceSnapshot{
		UserID:     userID,
		BalanceSen: 0,
		Version:    1,
		UpdatedAt:  time.Now().UTC(),
	}
}
