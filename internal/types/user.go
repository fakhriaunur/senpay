package types

import (
	"time"

	"github.com/google/uuid"
)

// User represents a registered user in the system.
type User struct {
	ID        uuid.UUID `json:"id"`
	Phone     string    `json:"phone"`
	PINHash   string    `json:"-"` // never serialized
	KYCLevel  string    `json:"kyc_level"`
	CreatedAt time.Time `json:"created_at"`
}

// KYC levels.
const (
	KYCLevelBasic    = "basic"
	KYCLevelVerified = "verified"
)

// NewUser creates a new User with a UUID v7 ID.
func NewUser(phone, pinHash string) User {
	return User{
		ID:        uuid.Must(uuid.NewV7()),
		Phone:     phone,
		PINHash:   pinHash,
		KYCLevel:  KYCLevelBasic,
		CreatedAt: time.Now().UTC(),
	}
}
