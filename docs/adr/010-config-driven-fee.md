# ADR 010: Config-Driven Fee Calculation with Cold Reload

**Status:** Accepted  
**Date:** 2026-05-16  
**Deciders:** Senpay Engineering Team

## Context

Senpay charges fees on transfers. The fee structure varies by KYC level and is subject to change based on business needs and promotional campaigns. Requirements:

- Fee rules must be configurable without code changes
- Fee calculation must be deterministic and testable (pure function per ADR 001)
- Promotional discounts (e.g., "BEBASFEE" campaign) must be supported
- Fee config must be tamper-proof at runtime — no accidental corruption from concurrent requests
- Invalid config must crash at startup, not silently miscompute fees in production

## Decision

Fee rules are defined in `fees.yaml`, validated at startup via crash-early, and loaded once into an immutable `FeeConfig` struct. No hot-reload. Fee computation is a pure function in the core layer, receiving config as a parameter.

### Config Schema (`fees.yaml`)

```yaml
flat_fee_basic_sen: 2500       # Flat fee in sen for basic KYC transfers (Rp 2,500)
rate_verified_pct: 0.7         # Percentage fee for verified KYC transfers (0.7%)
min_fee_sen: 1000              # Minimum fee in sen for verified KYC (Rp 1,000)

promo:
  discount_pct: 100.0           # Discount % when valid promo code used (100 = free)
  free_transfer_window:
    start_time: "2026-01-01T00:00:00Z"
    end_time: "2026-12-31T23:59:59Z"
  campaign_codes:
    - "BEBASFEE"
    - "GRATIS-ONGKIR"
```

### Immutable Config Struct

```go
type FeeConfig struct {
    FlatFeeBasicSen  int64         // flat_fee_basic_sen
    RateVerifiedPct  float64       // rate_verified_pct
    MinFeeSen        int64         // min_fee_sen
    Promo            PromoConfig
}

type PromoConfig struct {
    DiscountPct        float64
    FreeTransferWindow TimeWindow
    CampaignCodes      []string   // validated as PromoCode newtype at load time
}
```

Loaded at startup by `config.LoadFeeConfig("fees.yaml")` into a single immutable value. No mutex, no setters — the config is a value, not a shared mutable reference.

### Pure Fee Calculation

Fee computation lives in `internal/fee/core.go` as a pure function:

```go
func CalcFee(amount types.Money, kyc types.KYCLevel, promo *types.PromoCode, cfg FeeConfig) (types.Money, error)
```

- `amount`: Transfer amount in sen
- `kyc`: Typed KYC level (from ADR 009)
- `promo`: Optional promo code (nil if none). Typed `PromoCode` newtype prevents injection.
- `cfg`: Immutable fee configuration passed as parameter
- Returns fee in sen, or error on invalid promo code

### Promo Code Validation

Promo codes are validated at the API boundary via `ParsePromoCode(s string) (PromoCode, error)`. The newtype prevents raw string injection. Campaign codes in `fees.yaml` are validated at startup — any invalid code format crashes the server.

### Crash-Early Validation

At startup, `config.LoadFeeConfig()` validates:

- `flat_fee_basic_sen` ≥ 0
- `rate_verified_pct` in range [0, 100]
- `min_fee_sen` ≥ 0
- `promo.discount_pct` in range [0, 100]
- `promo.free_transfer_window.start_time` before `end_time`
- All `campaign_codes` match `[a-zA-Z0-9-]+`
- No duplicate campaign codes

Any violation → `log.Fatal`. Server does not start with invalid fee config.

## Alternatives Considered

**Hot-reload (watch fees.yaml and apply changes without restart)** — Rejected. Introduces complexity: race conditions if a transfer is in-flight during reload, need for `sync/atomic` or `sync.RWMutex` around config, harder to reason about which config version a given transaction used. Cold reload is simpler and adequate for fee rules (which change infrequently).

**JSON format (fees.json)** — Rejected. YAML is more readable for nested configuration with comments. The existing project already uses YAML for this purpose. JSON lacks comment support, which is valuable for documenting fee rationale.

**Database-backed fee rules** — Rejected. Fee rules are deployment config, not application data. Storing them in the database adds latency to every fee calculation and creates a bootstrapping problem (what fee to use before the DB is available?). File-based config is simpler and sufficient.

**Hardcoded fee rules in Go source** — Rejected. Requires code changes and recompilation for every fee adjustment. Config file enables operations to adjust fees without development involvement.

## Consequences

**Positive:**

- Config changes require restart — no runtime corruption, no race conditions
- Immutable config passed as parameter to pure functions — trivially testable
- Crash-early validation prevents invalid config from reaching production
- `PromoCode` newtype prevents injection of arbitrary strings into fee calculation
- YAML format is human-readable and supports comments for documentation

**Negative:**

- Fee changes require server restart (acceptable: fee rules change infrequently, typically weekly/monthly)
- No per-request config version tracking (acceptable: cold reload means all requests use the same config version)
- `float64` for `rate_verified_pct` — acceptable because it's multiplied against `Money` (int64 sen) and the result is rounded. The percentage itself (0.7) is a configuration constant, not a financial value subject to accumulation errors
- `float64` for `discount_pct` — same rationale as rate

## Compliance

Fee configuration must be loaded via `config.LoadFeeConfig()`. No code may read `fees.yaml` directly outside the config package. Fee computation must be a pure function in `fee/core.go`, accepting `FeeConfig` as a parameter. Promo code values must pass through `ParsePromoCode` before reaching core logic.
