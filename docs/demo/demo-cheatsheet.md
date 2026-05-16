# Demo Cheatsheet — Senpay Interview

One-page quick reference. All commands, endpoints, ports, and file paths.

---

## Justfile / Make Commands

```bash
just build          # Compile all Go binaries
just test           # Run all tests (short mode)
just test-all       # Full test suite (short + integration)
just run-server     # Start backend API
just run-tui        # Start TUI
just db-up          # Docker compose up -d
just db-down        # Docker compose down -v
just db-migrate     # Run pending migrations
```

---

## Docker Services

| Container | Image | Port | Description |
|-----------|-------|------|-------------|
| senpay-postgres | postgres:18-bookworm | 5432 | Append-only ledger |
| senpay-redis | redis:8.6 | 6379 | Idempotency cache + rate limiting |
| senpay-nats | nats:2.14-alpine | 4222 | Event publishing (tx.completed) |

```bash
docker compose ps
docker compose logs senpay-postgres --tail 20
```

---

## API Endpoints (port 8384)

```bash
# Health
curl http://localhost:8384/health

# Auth
curl -X POST http://localhost:8384/v1/auth/register       -H 'Content-Type: application/json' -d '{"phone":"08xx","pin":"123456"}'
curl -X POST http://localhost:8384/v1/auth/login           -H 'Content-Type: application/json' -d '{"phone":"08xx","pin":"123456"}'
curl -X POST http://localhost:8384/v1/auth/refresh         -H 'Content-Type: application/json' -d '{"refresh_token":"..."}'
curl -X POST http://localhost:8384/v1/auth/kyc             -H 'Content-Type: application/json' -H 'Authorization: Bearer TOKEN' -d '{"kyc_level":"verified"}'
curl -X GET  http://localhost:8384/v1/auth/me              -H 'Authorization: Bearer TOKEN'
curl -X GET  http://localhost:8384/v1/wallet/balance       -H 'Authorization: Bearer TOKEN'

# Transfer
curl -X POST http://localhost:8384/v1/transfer             -H 'Content-Type: application/json' -H 'Authorization: Bearer TOKEN' -d '{"to_phone":"08xx","amount_sen":50000,"promo_code":"BEBASFEE"}'

# Transaction History
curl -X GET  "http://localhost:8384/v1/transactions?limit=10"  -H 'Authorization: Bearer TOKEN'
curl -X GET  "http://localhost:8384/v1/transactions/UUID"      -H 'Authorization: Bearer TOKEN'

# Top-up (with SNAP bank adapter)
curl -X POST http://localhost:8384/v1/topup               -H 'Content-Type: application/json' -H 'Authorization: Bearer TOKEN' -d '{"amount_sen":100000,"idempotency_key":"..."}'

# Withdraw
curl -X POST http://localhost:8384/v1/withdraw             -H 'Content-Type: application/json' -H 'Authorization: Bearer TOKEN' -d '{"amount_sen":50000,"bank_account":"1234567890","bank_name":"BRI"}'

# Senpai (Nudge)
curl -X GET  http://localhost:8384/v1/senpai/summary       -H 'Authorization: Bearer TOKEN'
curl -X POST http://localhost:8384/v1/senpai/budgets       -H 'Content-Type: application/json' -H 'Authorization: Bearer TOKEN' -d '{"category":"Belanja","limit_sen":500000}'
curl -X GET  http://localhost:8384/v1/senpai/budgets       -H 'Authorization: Bearer TOKEN'
curl -X GET  http://localhost:8384/v1/senpai/nudge         -H 'Authorization: Bearer TOKEN'
```

---

## i18n (Bilingual)

| Header | Language | Example |
|--------|----------|---------|
| (none) | Indonesian | `"PIN salah"` |
| `Accept-Language: en` | English | `"Invalid PIN"` |
| `Accept-Language: fr` | Fallback | Indonesian |

TUI: press `5` or `s` on dashboard → settings → toggle language.

---

## Key Architectural Files

| File | What to Say |
|------|-------------|
| `internal/ledger/core.go` | Pure function — `ExecuteTransfer` |
| `internal/fee/core.go` | Pure function — `CalcFee` with FeeConfig |
| `internal/fee/config.go` | FeeConfig YAML parser — Parse-Don't-Validate |
| `internal/types/money.go` | SafeAdd/SafeSub — overflow-proof Money |
| `internal/types/enums.go` | PDV types — KYCLevel, TxType, TxStatus |
| `internal/idempotency/core.go` | Idempotency decision — Proceed/Duplicate/InFlight |
| `internal/saga/coordinator.go` | Saga retry (3) + compensation |
| `internal/senpai/nudge.go` | Rule-based nudge detectors |
| `internal/senpai/llm/interface.go` | Provider-agnostic LLM adapter |
| `internal/gateway/middleware.go` | BI limits + rate limiter + recovery |
| `internal/transfer/service.go` | Transfer orchestrator — idempotency → saga → event |
| `internal/tui/dashboard.go` | Dashboard with balance + nudge card |
| `internal/i18n/i18n.go` | Bilingual T(key, lang) lookup |
| `fees.yaml` | Config-driven fee parameters |
| `locales/id.json` | Indonesian locale (~177 keys) |
| `locales/en.json` | English locale (~177 keys) |

---

## 10-Minute Demo Flow

| Time | Topic | Commands |
|------|-------|----------|
| 0-2 | Architecture | `ls internal/`, `head -5 core.go`, `head -30 store.go`, `git log --oneline -15` |
| 2-4 | Live pipeline | `docker compose ps`, `go test ./internal/... -short` |
| 4-6 | Transfer flow | Register → Login → Transfer → Balance → History |
| 6-8 | Business engines | Nudge API, fees.yaml, i18n Accept-Language |
| 8-10 | TUI | `go run ./cmd/tui` + keyboard navigate 6 screens |

---

## Error Code Taxonomy

| Code | HTTP | Meaning |
|------|------|---------|
| `INVALID_AMOUNT` | 400 | Amount validation failure |
| `INSUFFICIENT_BALANCE` | 400 | Not enough balance for transfer |
| `SELF_TRANSFER` | 400 | Can't transfer to yourself |
| `EXCEEDS_TRANSACTION_LIMIT` | 400 | BI limit (Rp 2M basic / Rp 10M verified) |
| `AMOUNT_BELOW_MINIMUM` | 400 | Below Rp 10 minimum transfer |
| `PHONE_ALREADY_REGISTERED` | 409 | Duplicate registration |
| `INVALID_PIN` | 401 | Wrong credentials |
| `UNAUTHORIZED` | 401 | Session expired or invalid token |
| `REQUEST_IN_FLIGHT` | 202 | Duplicate request in progress |
| `SERIALIZATION_CONFLICT` | 409 | Retry after saga rollback |
| `FEATURE_NOT_AVAILABLE` | 501 | Feature-gated (LLM nudges) |
| `INTERNAL_ERROR` | 500 | Unexpected failure |
