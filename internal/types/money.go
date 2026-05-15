package types

import (
	"math"
)

// Money represents a monetary amount in sen (1 IDR = 100 sen).
// Stored as int64 to avoid floating-point precision issues.
// All financial operations use this type exclusively.
type Money int64

// Sen returns the amount in sen.
func (m Money) Sen() int64 {
	return int64(m)
}

// IDR returns the amount in IDR (rounded down, since sen is the base unit).
func (m Money) IDR() int64 {
	return int64(m) / 100
}

const (
	// Zero is the zero value for Money.
	Zero Money = 0
)

// SafeAdd adds two Money values with overflow protection.
// Returns the result and a bool indicating whether the operation overflowed.
func SafeAdd(a, b Money) (Money, bool) {
	a64 := int64(a)
	b64 := int64(b)
	// Overflow: both positive, sum wraps negative (two's complement wraparound)
	if a64 > 0 && b64 > 0 && a64 > math.MaxInt64-b64 {
		return Zero, true
	}
	// Underflow: both negative, sum wraps positive
	if a64 < 0 && b64 < 0 && a64 < math.MinInt64-b64 {
		return Zero, true
	}
	return Money(a64 + b64), false
}

// SafeSub subtracts two Money values with underflow protection.
// Returns the result and a bool indicating whether the operation underflowed (result < 0).
func SafeSub(a, b Money) (Money, bool) {
	if int64(b) > int64(a) {
		return Zero, true
	}
	result := int64(a) - int64(b)
	if result < 0 {
		return Zero, true
	}
	return Money(result), false
}

// IsZero returns true if the amount is zero.
func (m Money) IsZero() bool {
	return m == Zero
}

// IsNegative returns true if the amount is negative.
func (m Money) IsNegative() bool {
	return int64(m) < 0
}

// IsPositive returns true if the amount is positive.
func (m Money) IsPositive() bool {
	return int64(m) > 0
}
