# ADR 009: PDV Type System — Parse-Don't-Validate with Typed Newtypes

**Status:** Accepted  
**Date:** 2026-05-16  
**Deciders:** Senpay Engineering Team

## Context

A [PDV gap scan](../pdv-gap-scan-report.md) identified 28 places where raw `string` discriminators are used for semantically distinct concepts: KYC levels, transaction types, transaction statuses, VA statuses, bank provider switches, ledger entry types, idempotency statuses, callback statuses, and withdraw statuses. Each raw string introduces risk:

- A typo like `"verifed"` instead of `"verified"` compiles and silently stores invalid data in the database.
- String switches have no exhaustiveness checking — a new value silently falls through to `default`.
- Callers can pass any arbitrary string, bypassing intentional design constraints.
- Status domains that are semantically different (e.g., `TxStatus` vs `VAStatus`) are conflated as the same `string` type.

ADR 002 established the `Money` newtype for monetary values, and `idempotency.Decision` and `bank.StubBehavior` already demonstrate the pattern. This ADR extends the approach to **all string discriminators** in the codebase.

## Decision

Replace all raw string discriminators with **typed newtypes** following the Parse-Don't-Validate (PDV) pattern. Each discriminator gets:

1. A named type (e.g., `type KYCLevel string`)
2. Exported constants for all valid values
3. A `ParseXxx(s string) (Xxx, error)` constructor — the **only** way to create a value from a raw string
4. Business logic uses typed constants only — **zero raw string comparisons**

### Newtypes Defined

| Type | Package | Values | Replaces |
|------|---------|--------|----------|
| `KYCLevel` | `types` | `KYCLevelBasic`, `KYCLevelVerified` | Raw `"basic"`, `"verified"` strings in ~10 files |
| `TxType` | `types` | `TxTypeTransfer`, `TxTypeTopup`, `TxTypeWithdraw`, `TxTypeFee` | Raw `"transfer"`, `"topup"`, `"withdraw"`, `"fee"` strings in ~20 files |
| `TxStatus` | `types` | `TxStatusPending`, `TxStatusCommitted`, `TxStatusFailed`, `TxStatusRejected`, `TxStatusExpired` | Raw `"pending"`, `"committed"`, `"failed"` strings in ~20 files |
| `VAStatus` | `bank` | `VAStatusActive`, `VAStatusPaid`, `VAStatusExpired` | Raw `"active"`, `"paid"`, `"expired"` strings in bank store/service |
| `BankProviderType` | `config` | `ProviderStub`, `ProviderSnap` | Raw `"stub"`, `"snap"` strings in config and entry points |
| `EntryType` | `ledger` | `EntryTypeDebit`, `EntryTypeCredit` | Raw `"debit"`, `"credit"` strings in ledger core |
| `RedisStatus` | `idempotency` | `StatusCompleted`, `StatusInFlight`, `StatusEmpty` | Raw `"completed"`, `"in_flight"`, `""` strings at Redis call sites |
| `CallbackStatus` | `bank` | `CallbackStatusSuccess`, `CallbackStatusFailed` | Raw `"success"`, `"failed"` strings in bank webhook handling |
| `WithdrawStatus` | `bank` | Reuses `TxStatusPending`, `TxStatusCommitted`, `TxStatusFailed`, plus `TxStatusTimeout` | Raw status strings in `WithdrawRecord` |

### Pattern

```go
// Pattern A: String-backed newtype (used for all discriminators)
type KYCLevel string

const (
    KYCLevelBasic    KYCLevel = "basic"
    KYCLevelVerified KYCLevel = "verified"
)

// ParseKYCLevel is the single entry point for creating a KYCLevel from raw input.
// All callers at the shell boundary (HTTP, DB scan, JSON deserialization) must use this.
func ParseKYCLevel(s string) (KYCLevel, error) {
    switch s {
    case string(KYCLevelBasic), string(KYCLevelVerified):
        return KYCLevel(s), nil
    default:
        return "", fmt.Errorf("invalid KYC level: %q", s)
    }
}
```

### Boundary Contract

- **Shell boundary (HTTP handlers, DB scanners, JSON unmarshal)**: Call `ParseXxx(rawString)`. On error, reject with `400 Bad Request` or equivalent.
- **Core functions**: Accept the typed constant directly. Never accept or compare raw strings.
- **Constructors**: Accept typed constants. `NewTransaction(txType TxType, ...)` — no string parameter.
- **SQL queries**: Use typed constant values via `string(TxTypeTransfer)` or `%s` formatting. Future: consider a `String()` method.

## Alternatives Considered

**Status: string enum via `iota` (int-backed)** — Rejected. String-backed types preserve human-readable values in JSON, logs, and database rows. `iota` would require mapping tables for serialization.

**Validation functions without newtypes (keep `string`, add `IsValidKYCLevel(s string) bool`)** — Rejected. Validation functions are Parse-Don't-Validate anti-pattern. Callers can forget to call them. The type system should enforce correctness, not convention.

**Do nothing (accept raw strings)** — Rejected. The 28 identified gaps demonstrate that raw strings are error-prone. The existing bugs (`"verifed"`, silent fall-through) are real, not theoretical.

## Consequences

**Positive:**

- Compile-time safety: `== "verifed"` typos become compile errors
- Exhaustiveness: Adding a new value to a switch without handling it is a compile error (with `go vet` exhaustive check)
- Self-documenting: Function signatures like `CalcFee(amount Money, kyc KYCLevel)` are unambiguous
- Auditable: Grep for `ParseKYCLevel` finds all shell entry points for KYC validation
- Consistent: All discriminators follow the same pattern, reducing cognitive load

**Negative:**

- ~16 files refactored across `types`, `fee`, `bank`, `auth`, `ledger`, `idempotency`, `config`, `senpai`, `tui`, `gateway`, `projection`
- Boilerplate: Each newtype requires ~15 lines of constants + Parse function
- JSON (un)marshaling requires custom `MarshalJSON`/`UnmarshalJSON` methods that delegate to `Parse`
- Database scanning requires wrapper types or `sql.Scanner` implementation
- Migration risk: Changing struct fields from `string` to `KYCLevel` affects every serialization boundary

## Compliance

All PRs that introduce string discriminators must use typed newtypes. Code review enforces that no `core.go` function accepts or compares raw strings for semantically distinct values. CI lint checks for raw string comparisons against known discriminator values (`"basic"`, `"verified"`, `"transfer"`, etc.).
