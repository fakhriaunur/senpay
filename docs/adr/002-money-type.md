# ADR 002: Use int64 Money Type with Sen Precision

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

Financial systems must represent monetary amounts without floating-point precision errors. Indonesian Rupiah (IDR) is a fiat currency where the smallest practical unit is 1 sen (1/100 IDR). Common pitfalls include:

- `float64` rounding errors in financial calculations (e.g., `0.1 + 0.2 ≠ 0.3`)
- Integer overflow when amounts grow large
- Mixing display units (IDR) with storage units (sen)

## Decision

Represent all monetary amounts as `int64` denominated in **sen** (1 IDR = 100 sen).

```go
type Money int64
```

### Rules

1. **Never use `float64`** for monetary amounts — every financial operation uses `Money`
2. **Overflow-checked arithmetic** via `SafeAdd(a, b Money) (Money, bool)` and `SafeSub(a, b Money) (Money, bool)` — returns `ok=false` on overflow/underflow
3. **Display formatting** divides by 100 for IDR display (e.g., `Money(100000).IDR() = 1000` for IDR 1.000)
4. **JSON serialization** uses raw integer (`amount_sen` field)
5. **BI limits** expressed in sen: basic KYC = 200,000 sen (Rp 2M), verified KYC = 1,000,000 sen (Rp 10M)

### Range

`int64` max = ~9.22 × 10¹⁸ sen (~92 quadrillion IDR). This is sufficient for Indonesian consumer e-wallet volumes and provides headroom for growth.

## Consequences

**Positive:**

- Exact arithmetic: no rounding errors from floating-point
- Safe addition/subtraction with explicit overflow handling
- Simple storage: `BIGINT` in PostgreSQL
- Clear separation between display and storage units

**Negative:**

- Requires conversion for display (IDR formatting with thousand separators)
- Must remember: 1 IDR = 100 sen, not 1 = 1

## Compliance

All financial fields use `types.Money`. Code review rejects `float64` for monetary values. Overflow checks (`SafeAdd`/`SafeSub`) are required for all arithmetic.
