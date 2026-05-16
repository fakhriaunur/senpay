# PDV Gap Scan Report — Senpay Codebase

**Date:** 2026-05-16  
**Scope:** `internal/` directory, all `.go` files  
**Methodology:** Systematic grep + file inspection against PDV checklist categories

---

## Executive Summary

| Metric | Count |
|---|---|
| Total PDV gaps found | **28 distinct gaps** (across 6 categories) |
| Existing good patterns (already-PDV) | **4** (idempotency.Decision, bank.StubBehavior, types.Money, types.DomainError) |
| By effort: Small (1-5 files) | 11 |
| By effort: Medium (5-15 files) | 14 |
| By effort: Large (15+ files) | 3 |

---

## Existing Good Patterns (Positive Examples)

These are already properly typed — worth noting as patterns to follow:

1. **`idempotency.Decision`** (`internal/idempotency/core.go:9-26`) — `type Decision int` with iota constants (`Proceed`, `Duplicate`, `InFlight`) and a `String()` method. ✅ Proper newtype.

2. **`bank.StubBehavior`** (`internal/bank/provider_stub.go:13`) — `type StubBehavior string` with named constants (`StubBehaviorSuccess`, `StubBehaviorRejection`, `StubBehaviorTimeout`, `StubBehaviorSlow`). ✅ Proper newtype.

3. **`types.Money`** (`internal/types/money.go:10`) — `type Money int64` with methods (`IsPositive()`, `SafeAdd()`, `SafeSub()`). ✅ Proper newtype with behavioral methods.

4. **`types.DomainError`** (`internal/types/errors.go:9`) — `type DomainError struct{ Code string; ... }` with pre-built error variables. ✅ Proper error taxonomy.

---

## Category 1: KYC Level (string → should be KYCLevel type)

### Gap 1.1 — `types.User.KYCLevel` is raw `string`
- **File:** `internal/types/user.go:14`
- **Current:** `KYCLevel string \`json:"kyc_level"\``
- **Risk:** Any string can be assigned. A typo like `"verifed"` or `"basicc"` compiles and silently stores invalid data in DB.
- **Suggested fix:** Define `type KYCLevel string` with constants `KYCLevelBasic`, `KYCLevelVerified`, and a `ParseKYCLevel(s string) (KYCLevel, error)` function. Change struct field to `KYCLevel`.
- **Effort:** **Medium** (5-15 files — User struct used everywhere via DB scan, auth, bank, gateway, fee, tests)

### Gap 1.2 — `fee.CalcFee` takes `userKYC string`
- **File:** `internal/fee/core.go:34`
- **Current:** `func CalcFee(amount types.Money, userKYC string) (types.Money, *types.DomainError)`
- **Risk:** Callers pass any string. Default case silently treats unknown as basic — could mask bugs.
- **Suggested fix:** Change signature to `CalcFee(amount types.Money, kyc KYCLevel)`. The `default` case can still fall back to basic for backward compat.
- **Effort:** Covered by Gap 1.1

### Gap 1.3 — `fee.TransferInput.KYCLevel` is raw `string`
- **File:** `internal/fee/core.go:12`
- **Current:** `KYCLevel string`
- **Risk:** Same as 1.1 — batch fee calculation accepts arbitrary strings.
- **Suggested fix:** Change to `KYCLevel` type.
- **Effort:** Covered by Gap 1.1

### Gap 1.4 — `bank.checkBILimit` takes `kycLevel string`
- **File:** `internal/bank/service.go:1139`
- **Current:** `func checkBILimit(amount types.Money, kycLevel string) *types.DomainError`
- **Risk:** Any string passes BI limit check. Default silently caps at basic limit.
- **Suggested fix:** Change to `KYCLevel` type.
- **Effort:** Covered by Gap 1.1

### Gap 1.5 — `auth.Handler.KYC` validates with raw string comparisons
- **File:** `internal/auth/handler.go:324`
- **Current:** `if req.KYCLevel != types.KYCLevelVerified && req.KYCLevel != types.KYCLevelBasic`
- **Risk:** Fragile — adding new KYC levels requires updating every validation site.
- **Suggested fix:** Use `ParseKYCLevel(req.KYCLevel)` and check for error.
- **Effort:** Covered by Gap 1.1

### Gap 1.6 — `auth.UserStore.UpdateKYCLevel` takes `level string`
- **File:** `internal/auth/store.go:27`
- **Current:** `UpdateKYCLevel(ctx context.Context, id uuid.UUID, level string) error`
- **Risk:** Store accepts any string — could write invalid KYC level to DB.
- **Suggested fix:** Change to `KYCLevel` type.
- **Effort:** Covered by Gap 1.1

### Gap 1.7 — `auth.kycRequest.KYCLevel` and `auth.kycResponse.KYCLevel` are raw `string`
- **File:** `internal/auth/handler.go:61,79`
- **Current:** `KYCLevel string \`json:"kyc_level"\``
- **Risk:** JSON deserialization accepts any string without validation.
- **Suggested fix:** Use `KYCLevel` type with custom JSON unmarshal that validates.
- **Effort:** Covered by Gap 1.1

### Gap 1.8 — `gateway.mockUserStore.addUser` takes `kycLevel string`
- **File:** `internal/gateway/middleware_test.go:35`
- **Current:** `func (m *mockUserStore) addUser(id uuid.UUID, kycLevel string)`
- **Risk:** Test-only, but propagates the pattern.
- **Suggested fix:** Use `KYCLevel` type.
- **Effort:** Covered by Gap 1.1 (test files affected)

---

## Category 2: Transaction Type (string → should be TxType type)

### Gap 2.1 — `types.Transaction.TxType` is raw `string`
- **File:** `internal/types/transaction.go:13`
- **Current:** `TxType string \`json:"tx_type"\``
- **Risk:** Any string compiles. `"transer"`, `"top_up"`, `"withraw"` would silently store invalid data.
- **Suggested fix:** Define `type TxType string` with constants and `ParseTxType(s string) (TxType, error)`. Change struct field.
- **Effort:** **Large** (15+ files — Transaction struct used in DB, JSON APIs, TUI, handlers, tests everywhere)

### Gap 2.2 — `types.NewTransaction` takes `txType string`
- **File:** `internal/types/transaction.go:51`
- **Current:** `func NewTransaction(txType, idempotencyKey string, ...) Transaction`
- **Risk:** Constructor accepts any string without validation.
- **Suggested fix:** Change to `TxType` type.
- **Effort:** Covered by Gap 2.1

### Gap 2.3 — TUI uses raw string switches for TxType
- **File:** `internal/tui/history.go:164-168,179-184`
- **Current:** `switch tx.TxType { case "transfer", "fee", "withdraw": ... case "topup": ... }`
- **Risk:** A typo in any case label silently falls through to default. Not compile-time checked.
- **Suggested fix:** Use `TxType` constants.
- **Effort:** Covered by Gap 2.1

### Gap 2.4 — TUI detail uses raw string switches
- **File:** `internal/tui/detail.go:53-61,102-106`
- **Current:** `func txTypeDisplay(txType string)` — switch on `"transfer"`, `"topup"`, `"withdraw"`, `"fee"`
- **Risk:** Same as 2.3 — untyped string switches.
- **Suggested fix:** Use `TxType` constants.
- **Effort:** Covered by Gap 2.1

### Gap 2.5 — `tui.TransactionItem.TxType` is raw `string`
- **File:** `internal/tui/client.go:218`
- **Current:** `TxType string \`json:"tx_type"\``
- **Risk:** JSON deserialization silently accepts invalid values.
- **Suggested fix:** Use `TxType` type.
- **Effort:** Covered by Gap 2.1

### Gap 2.6 — SQL queries embed raw tx_type strings
- **File:** `internal/senpai/budgets.go:249,311`
- **Current:** `AND tx_type = 'transfer'` (hardcoded in SQL)
- **Risk:** If constants are renamed, SQL queries would silently filter wrong.
- **Suggested fix:** Use Go constant in query construction (e.g., `fmt.Sprintf("... tx_type = '%s'", types.TxTypeTransfer)`). Or at minimum use the constant value via `types.TxTypeTransfer`.
- **Effort:** **Small** (only 2 sites in budgets.go)

---

## Category 3: Transaction Status (string → should be TxStatus type)

### Gap 3.1 — `types.Transaction.Status` is raw `string`
- **File:** `internal/types/transaction.go:18`
- **Current:** `Status string \`json:"status"\``
- **Risk:** Any string compiles. `"commited"`, `"pendding"` would silently store invalid status.
- **Suggested fix:** Define `type TxStatus string` with constants and `ParseTxStatus(s string) (TxStatus, error)`.
- **Effort:** **Large** (15+ files — used everywhere Transaction is used: DB, JSON, handlers, TUI, bank, projection, tests)

### Gap 3.2 — TUI uses raw string switches for status
- **File:** `internal/tui/history.go:192-201`, `internal/tui/detail.go:69-79,87-94`
- **Current:** `func statusIcon(status string)`, `func statusDisplay(status string)`, `func statusColor(status string)` — all switch on raw strings.
- **Risk:** Typos in case labels silently fall through.
- **Suggested fix:** Use `TxStatus` constants.
- **Effort:** Covered by Gap 3.1

### Gap 3.3 — `projection.TxEntry.Status` is raw `string`
- **File:** `internal/projection/core.go:11`
- **Current:** `Status string` with comparison `entry.Status != "committed"`
- **Risk:** A typo in the string literal would silently skip committed entries.
- **Suggested fix:** Change to `TxStatus` type with `== TxStatusCommitted`.
- **Effort:** Covered by Gap 3.1 (projection affected)

### Gap 3.4 — `projection/core_test.go` uses raw strings extensively
- **File:** `internal/projection/core_test.go:28+`
- **Current:** `Status: "committed"`, `Status: "pending"`, `Status: "failed"` etc.
- **Risk:** Test-only, but propagates the anti-pattern.
- **Suggested fix:** Use `TxStatus` constants.
- **Effort:** Covered by Gap 3.1 (test files affected)

### Gap 3.5 — `bank.VATopupRecord.Status` is raw `string` (separate status domain!)
- **File:** `internal/bank/store.go:25`
- **Current:** `Status string \`json:"status"\` // active, paid, expired`
- **Risk:** VA status values are a *different* set from tx status. Using same `string` field means no type-level distinction.
- **Suggested fix:** Define `type VAStatus string` with constants `VAStatusActive`, `VAStatusPaid`, `VAStatusExpired`.
- **Effort:** **Small** (1-5 files — store.go, postgres.go, service.go, core.go)

### Gap 3.6 — `bank.WithdrawRecord.Status` is raw `string` (yet another status domain!)
- **File:** `internal/bank/store.go:62`
- **Current:** `Status string \`json:"status"\` // pending, committed, failed, timeout`
- **Risk:** Withdraw status uses same values as `TxStatus` but stored separately — type erosion.
- **Suggested fix:** Could reuse `TxStatus` type if semantics match, or define `WithdrawStatus`.
- **Effort:** **Small** (1-5 files — store.go, postgres.go, service.go)

### Gap 3.7 — `bank.BankCallback.Status` is raw `string` (webhook callback!)
- **File:** `internal/bank/provider.go:57`
- **Current:** `Status string \`json:"status"\` // "success" or "failed"`
- **Risk:** Webhook callback status is a *third* status domain with different values ("success"/"failed").
- **Suggested fix:** Define `type CallbackStatus string` with `CallbackStatusSuccess`, `CallbackStatusFailed`. Or just use a boolean if it's truly binary.
- **Effort:** **Small** (1-5 files — provider.go, mock_server.go, webhook_test files)

---

## Category 4: Idempotency Decision

### Gap 4.1 — `Check()` takes raw `status string` parameter (but returns typed `Decision`)
- **File:** `internal/idempotency/core.go:37`
- **Current:** `func Check(key string, status string) Decision` — switches on `""`, `"completed"`, `"in_flight"`
- **Risk:** Callers pass raw strings from Redis/DB. If a new status value is introduced in Redis but not handled, it's silently treated as `Proceed`.
- **Assessment:** **Mild gap.** The `Decision` return type is already properly typed. The input `status` is inherently external (from Redis/DB). A proper fix would define a `Status` type for the stored value with `Parse(s string) (Status, error)`.
- **Suggested fix:** Define `type RedisStatus string` with constants `StatusCompleted`, `StatusInFlight`, `StatusEmpty`. Change `Check` to accept `RedisStatus`.
- **Effort:** **Medium** (5-15 files — transfer/service.go, bank/service.go, all Redis call sites, tests)

### Gap 4.2 — Raw `"in_flight"` and `"completed"` strings used at call sites
- **Files:** `internal/transfer/service.go:133,451`, `internal/bank/service.go:131,323,381,1076`
- **Current:** `s.redisCache.SetIfNotExist(ctx, key, "in_flight", ...)` and `s.redisCache.Set(ctx, key, "completed", ...)`
- **Risk:** Typo in the string literal would make `Check()` interpret incorrectly.
- **Suggested fix:** Use typed constants.
- **Effort:** Covered by Gap 4.1

---

## Category 5: Budget Alert State

### No significant gap
- **File:** `internal/senpai/budgets.go:43-46`
- **Current:** `BudgetWithAlert.Alert bool, Exceeded bool`
- **Assessment:** Boolean flags are appropriate here. `Alert` (threshold >=80%) and `Exceeded` (>=100%) are independent dimensions, not exclusive states. A proper enum would be `type BudgetState int` with states `Normal`, `Warning`, `Exceeded` — but the current approach is functional and clear. **Not flagged as a gap.**

---

## Category 6: Other String Discriminators

### Gap 6.1 — Bank provider type is raw string
- **File:** `internal/config/config.go:31`
- **Current:** `BankProvider string` — values "stub" or "snap"
- **Risk:** A typo like `"stud"` or `"SNAP"` (wrong case) would silently create wrong behavior.
- **Suggested fix:** Define `type BankProviderType string` with `ProviderStub`, `ProviderSnap` constants and a `Parse(s string) (BankProviderType, error)`.
- **Effort:** **Small** (1-5 files — config.go, main.go/cmd entry point, any place that selects adapters)

### Gap 6.2 — `ledger.TxEntry.TxType` uses "debit"/"credit" raw strings
- **File:** `internal/ledger/core.go:9,35-36`
- **Current:** `TxType string // "debit" for sender loss, "credit" for receiver gain` — assigned as raw `"debit"` and `"credit"` literals
- **Risk:** A typo like `"debbit"` or `"creadit"` would compile and potentially break downstream ledger logic.
- **Suggested fix:** Define `type EntryType string` with constants `EntryTypeDebit`, `EntryTypeCredit`. Or reuse `TxType` if it's a subset.
- **Effort:** **Small** (1-5 files — ledger/core.go, ledger/core_test.go)

### Gap 6.3 — `types.Transaction.Currency` is raw `string`
- **File:** `internal/types/transaction.go:17`
- **Current:** `Currency string \`json:"currency"\`` with `CurrencyIDR = "IDR"`
- **Risk:** Low — only one currency used. But adding currencies later without a type would be error-prone.
- **Suggested fix:** Define `type Currency string` with `CurrencyIDR`. Low priority.
- **Effort:** **Medium** (5-15 files — used everywhere Transaction is used)

### Gap 6.4 — `types.Transaction.Category` is raw `string`
- **File:** `internal/types/transaction.go:20`
- **Current:** `Category string \`json:"category,omitempty"\`` with `CategoryDefault = "Lainnya"`
- **Risk:** Free-form string — any garbage accepted. Categories are user-defined in budgets; this is somewhat intentional. But a typo creates orphan categories.
- **Suggested fix:** Define `type Category string` with well-known constants for built-in categories. Not critical since categories are user-defined.
- **Effort:** Low priority, **Medium** if done.

### Gap 6.5 — `bank.BankCallback.Status` uses "success"/"failed" (covered in Gap 3.7)
- Already addressed in Category 3.

---

## Summary Table

| # | Category | File:Line | Issue | Current | Risk | Effort |
|---|---|---|---|---|---|---|
| 1.1 | KYC | types/user.go:14 | User.KYCLevel | `string` | ✅ Typo compiles | Medium |
| 1.2 | KYC | fee/core.go:34 | CalcFee param | `string` | ✅ Silent fallback | (peer to 1.1) |
| 1.3 | KYC | fee/core.go:12 | TransferInput.KYCLevel | `string` | ✅ Silent fallback | (peer to 1.1) |
| 1.4 | KYC | bank/service.go:1139 | checkBILimit param | `string` | ✅ Wrong limit | (peer to 1.1) |
| 1.5 | KYC | auth/handler.go:324 | KYC validation | `string` compare | ✅ Bypass check | (peer to 1.1) |
| 1.6 | KYC | auth/store.go:27 | UpdateKYCLevel param | `string` | ✅ DB corruption | (peer to 1.1) |
| 1.7 | KYC | auth/handler.go:61 | kycRequest.KYCLevel | `string` | ✅ JSON passthrough | (peer to 1.1) |
| 1.8 | KYC | gateway/middleware_test.go:35 | Test helper | `string` | ✅ Test propagation | (peer to 1.1) |
| 2.1 | TxType | types/transaction.go:13 | Transaction.TxType | `string` | ✅ Typo compiles | **Large** |
| 2.2 | TxType | types/transaction.go:51 | NewTransaction param | `string` | ✅ Invalid insert | (peer to 2.1) |
| 2.3 | TxType | tui/history.go:164 | Switch on TxType | `string` | ✅ Falls to default | (peer to 2.1) |
| 2.4 | TxType | tui/detail.go:53 | txTypeDisplay switch | `string` | ✅ Falls to default | (peer to 2.1) |
| 2.5 | TxType | tui/client.go:218 | TransactionItem.TxType | `string` | ✅ JSON passthrough | (peer to 2.1) |
| 2.6 | TxType | senpai/budgets.go:249 | SQL literal | `'transfer'` | ✅ Missed rows | **Small** |
| 3.1 | TxStatus | types/transaction.go:18 | Transaction.Status | `string` | ✅ Typo compiles | **Large** |
| 3.2 | TxStatus | tui/history.go:192 | Status switch | `string` | ✅ Falls to default | (peer to 3.1) |
| 3.3 | TxStatus | projection/core.go:11 | TxEntry.Status | `string` | ✅ Silent skip | (peer to 3.1) |
| 3.4 | TxStatus | projection/core_test.go | Test helpers | `string` | ✅ Test propagation | (peer to 3.1) |
| 3.5 | TxStatus | bank/store.go:25 | VAStatus separate domain | `string` | ✅ Wrong value set | **Small** |
| 3.6 | TxStatus | bank/store.go:62 | WithdrawStatus separate | `string` | ✅ Type confusion | **Small** |
| 3.7 | TxStatus | bank/provider.go:57 | CallbackStatus "success"/"failed" | `string` | ✅ Wrong webhook | **Small** |
| 4.1 | Idempotency | idempotency/core.go:37 | Check input param | `string` | ✅ Silent Proceed | **Medium** |
| 4.2 | Idempotency | transfer/service.go:133,451 | Raw "in_flight"/"completed" | `string` | ✅ Typo breaks | (peer to 4.1) |
| 6.1 | Provider | config/config.go:31 | BankProvider | `string` | ✅ Wrong adapter | **Small** |
| 6.2 | Ledger | ledger/core.go:9 | TxEntry.TxType "debit"/"credit" | `string` | ✅ Typo compiles | **Small** |
| 6.3 | Currency | types/transaction.go:17 | Transaction.Currency | `string` | ✅ Low (1 currency) | Medium |
| 6.4 | Category | types/transaction.go:20 | Transaction.Category | `string` | ✅ Orphan category | Medium |

---

## Priority Recommendations

### High Impact, Low Effort (do first):
1. **Gap 3.5** — `VAStatus` newtype (`internal/bank/store.go`) — 3 values, 3 files
2. **Gap 3.7** — `CallbackStatus` newtype (`internal/bank/provider.go`) — 2 values, 3 files
3. **Gap 6.1** — `BankProviderType` newtype (`internal/config/config.go`) — 2 values, 2 files
4. **Gap 6.2** — `EntryType` newtype for debit/credit (`internal/ledger/core.go`) — 2 values, 2 files
5. **Gap 2.6** — Use typed constants in SQL queries (`internal/senpai/budgets.go`) — 2 SQL sites

### High Impact, Medium Effort (do second):
6. **Gap 1.x** — `KYCLevel` type — 2 values, ~10 files affected. Wide impact but simple values.
7. **Gap 4.x** — `RedisStatus` type for idempotency — 3 values, ~8 call sites
8. **Gap 3.6** — `WithdrawStatus` type — could merge with `TxStatus` or stay separate

### High Impact, Large Effort (do last):
9. **Gap 2.x** — `TxType` type — 4 values, ~20 files. Most impactful due to wide usage.
10. **Gap 3.x** — `TxStatus` type — 5 values, ~20 files. Most impactful due to wide usage.

### Deprioritized:
11. **Gap 6.3** — `Currency` type — only 1 value today, low ROI
12. **Gap 6.4** — `Category` type — user-defined values, limited type safety benefit

---

## Suggested Newtype Patterns

For each gap, follow the pattern already established by `idempotency.Decision` and `bank.StubBehavior`:

```go
// Pattern A: For string-backed enums (most gaps)
type KYCLevel string

const (
    KYCLevelBasic    KYCLevel = "basic"
    KYCLevelVerified KYCLevel = "verified"
)

func ParseKYCLevel(s string) (KYCLevel, error) {
    switch s {
    case string(KYCLevelBasic), string(KYCLevelVerified):
        return KYCLevel(s), nil
    default:
        return "", fmt.Errorf("invalid KYC level: %q", s)
    }
}

// Pattern B: For int-backed enums (already used by idempotency.Decision)
type Decision int

const (
    Proceed Decision = iota
    Duplicate
    InFlight
)

func (d Decision) String() string { ... }
```
