# E-Wallet MVP — Specification

Audience: hiring engineers at GoPay, OVO, DANA (Indonesian fintech).
Goal: production-grade demo proving architecture, code quality, and fintech domain competence.

---

## Product Scope

### 6 Core Flows

| Flow | Endpoint | Priority |
|------|----------|----------|
| Register + KYC | `POST /v1/auth/register`, `/login`, `/verify-kyc` | Must |
| Top-up via VA | `POST /v1/topup` → simulated VA number | Must |
| P2P Transfer | `POST /v1/transfer` | Must |
| Balance Inquiry | `GET /v1/balance` | Must |
| Withdraw to Bank | `POST /v1/withdraw` | Should |
| Tx History | `GET /v1/transactions` paginated | Should |

### Non-MVP (deferred)

Multi-currency, promo/cashback, QR scan, biller, merchant onboarding, invest/savings, split payment.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  cmd/                                                         │
│  ├── server/    → HTTP server (API gateway + handlers)        │
│  └── tui/       → Bubble Tea TUI (proves API before Flutter)  │
│  ├── ledger/        core.go postgres.go      Transfer engine  │
│  ├── fee/           core.go                   Fee calculation │
│  ├── auth/          core.go handler.go postgres.go  Auth+KYC   │
│  ├── gateway/       middleware.go router.go    HTTP layer      │
│  ├── bank/          interface.go provider_stub.go Payment rail │
│  │                  snap.go snap_mock_server.go provider_snap.go
│  ├── notif/         interface.go provider_stub.go Notification│
│  ├── idempotency/   core.go redis.go          Dedup guard     │
│  ├── projection/    core.go                   Balance project │
│  ├── saga/          coordinator.go           Retry+compensate │
│  ├── featureflag/   core.go                  Feature toggle   │
│  ├── telemetry/     log.go metrics.go trace.go middleware.go  │
│  └── config/        config.go                Env-based config │
├── migrations/       001_*.up.sql 002_*.down.sql               │
├── docs/adr/         001-money-type.md 002-append-only-ledger.md│
├── Dockerfile        (multi-stage: build → distroless)         │
├── docker-compose.yml  (server + PG + Redis + NATS)            │
├── justfile          lint test build run docker-all            │
├── .github/workflows/  ci.yml (lint, test, vulncheck)          │
└── README.md         Architecture diagram + how-to-run         │
```

### Dependency Rule

- `core.go` files import nothing beyond `stdlib` + `internal/types`
- `postgres.go` (shell) implements interface, calls core types
- `handler.go` (shell) calls core functions, never reverse
- Enforced by code review, not compiler

---

## Domain Model

```go
package types

type Money int64            // sen (1/100 IDR). Never float64.

type UserID string          // UUID
type TxnID string           // UUID
type IdempotencyKey string  // client-generated UUID

type TxType string
const (
    TxTopup    TxType = "topup"
    TxTransfer TxType = "transfer"
    TxWithdraw TxType = "withdraw"
    TxFee      TxType = "fee"
)

type TxStatus string
const (
    TxPending     TxStatus = "pending"
    TxCommitted   TxStatus = "committed"
    TxFailed      TxStatus = "failed"
    TxCompensated TxStatus = "compensated"
)

type KYCLevel string
const (
    KYCBasic    KYCLevel = "basic"
    KYCVerified KYCLevel = "verified"
)
```

### Structs

```go
type User struct {
    ID        UserID    `json:"id"`
    Phone     string    `json:"phone"`
    Email     string    `json:"email,omitempty"`
    PINHash   string    `json:"-"` // bcrypt hash
    KYCLevel  KYCLevel  `json:"kyc_level"`
    CreatedAt time.Time `json:"created_at"`
}

type Transaction struct {
    ID             TxnID          `json:"id"`
    IdempotencyKey IdempotencyKey `json:"idempotency_key"`
    TxType         TxType         `json:"tx_type"`
    SenderID       UserID         `json:"sender_id,omitempty"`
    ReceiverID     UserID         `json:"receiver_id,omitempty"`
    AmountSen      Money          `json:"amount_sen"`
    Currency       string         `json:"currency"`
    Status         TxStatus       `json:"status"`
    FailureReason  string         `json:"failure_reason,omitempty"`
    CreatedAt      time.Time      `json:"created_at"`
    CommittedAt    *time.Time     `json:"committed_at,omitempty"`
}

type BalanceSnapshot struct {
    UserID     UserID    `json:"user_id"`
    BalanceSen Money     `json:"balance_sen"`
    Version    int       `json:"version"`
    UpdatedAt  time.Time `json:"updated_at"`
}

type TransferCmd struct {
    SenderID      UserID
    ReceiverID    UserID
    AmountSen     Money
    IdempotencyKey string
    Now           time.Time
}
```

---

## API Surface

Auth: Bearer JWT from login response. Access token expires 30 min. Refresh token expires 7d (single-use, rotated).

| Method | Path | Request | Response |
|--------|------|---------|----------|
| POST | `/v1/auth/register` | `{ phone, pin }` | `{ user_id }` |
| POST | `/v1/auth/login` | `{ phone, pin }` | `{ token, expires_in }` |
| POST | `/v1/auth/verify-kyc` | `{ selfie_b64, id_card_b64 }` | `{ kyc_level }` |
| GET | `/v1/balance` | — | `{ balance_sen, version }` |
| POST | `/v1/topup` | `{ idempotency_key, amount_sen }` | `{ va_number, amount, expires_at }` |
| POST | `/v1/transfer` | `{ idempotency_key, to_phone, amount_sen }` | `{ tx_id, status }` |
| POST | `/v1/withdraw` | `{ idempotency_key, amount_sen, bank_account }` | `{ tx_id, status }` |
| GET | `/v1/transactions` | `?cursor=&limit=20` | `{ items[], next_cursor }` |
| GET | `/v1/transactions/:tx_id` | — | `{ transaction }` |
| GET | `/health` | — | `{ status: "ok" }` |
| GET | `/ready` | — | `{ pg: "ok", redis: "ok" }` |
| GET | `/metrics` | — | `{ prometheus text format }` |

### Error Response Shape

```json
{
  "error": {
    "code": "INSUFFICIENT_BALANCE",
    "message": "Saldo tidak cukup",
    "request_id": "req_abc123"
  }
}
```

---

## Error Taxonomy

```go
type DomainError struct {
    Code       string
    Message    string   // user-facing, Indonesian
    Internal   string   // debug context, never exposed to user
    HTTPStatus int
}

var (
    ErrInsufficientBalance   = &DomainError{Code: "INSUFFICIENT_BALANCE",   Message: "Saldo tidak cukup",             HTTPStatus: 400}
    ErrInvalidAmount         = &DomainError{Code: "INVALID_AMOUNT",         Message: "Jumlah tidak valid",            HTTPStatus: 400}
    ErrSelfTransfer          = &DomainError{Code: "SELF_TRANSFER",          Message: "Tidak bisa transfer ke diri sendiri", HTTPStatus: 400}
    ErrDuplicateTransaction  = &DomainError{Code: "DUPLICATE_TRANSACTION",  Message: "Transaksi sudah diproses",      HTTPStatus: 409}
    ErrUserNotFound          = &DomainError{Code: "USER_NOT_FOUND",         Message: "Pengguna tidak ditemukan",       HTTPStatus: 404}
    ErrInvalidPIN            = &DomainError{Code: "INVALID_PIN",            Message: "PIN salah",                      HTTPStatus: 401}
    ErrSerializationConflict = &DomainError{Code: "SERIALIZATION_CONFLICT", Message: "Silakan coba lagi",             HTTPStatus: 409}
    ErrBankDown              = &DomainError{Code: "BANK_UNAVAILABLE",       Message: "Layanan bank sedang sibuk",    HTTPStatus: 502}
    ErrKYCRequired           = &DomainError{Code: "KYC_REQUIRED",           Message: "Verifikasi KYC diperlukan",     HTTPStatus: 403}
    ErrUnauthorized          = &DomainError{Code: "UNAUTHORIZED",           Message: "Sesi habis, silakan login ulang", HTTPStatus: 401}
    ErrExceedsLimit          = &DomainError{Code: "EXCEEDS_TRANSACTION_LIMIT", Message: "Melebihi batas transaksi",       HTTPStatus: 400}
    ErrIdempotencyInFlight   = &DomainError{Code: "REQUEST_IN_FLIGHT",        Message: "Permintaan sedang diproses",     HTTPStatus: 202}
)
```

---

## Data Pipelines (DoD Flow Design)

### Transfer Pipeline

```
HTTP POST /transfer
  → Validate JWT, rate limit
  → Check idempotency key
     - Redis GET → hit → return cached response (HTTP 200)
     - Redis GET → "in-flight" marker (TTL 30s) → return 202 Accepted
     - Neither → SET "in-flight" marker (TTL 30s), continue
  → Parse + validate: amount>0, sender≠receiver
  → Read balance_snapshot (PG single row)
  → CORE: ExecuteTransfer(senderBal, receiverBal, amount) → TxEvent or error
  → PG serializable transaction:
      INSERT tx_log(debit, pending)
      INSERT tx_log(credit, pending)
      UPDATE balance_snapshot WHERE version=v → v+1
      → retry(3) on serialization failure, else fail+compensate
  → UPDATE tx_log SET status=committed
  → SET idempotency_key TTL 24h (Redis)
  → CLEAR "in-flight" marker (Redis)
  → PUB tx.completed (NATS)
  → Return { tx_id, status }
```

### Topup Pipeline

```
POST /v1/topup
  → Idempotency check
  → CORE: GenerateTopup(amount) → { VA_number, expires_at }
  → INSERT tx_log(pending)
  → (Simulated) bank VA created
  → Return VA_number to user
  → (Background) Webhook callback marks tx_log committed
```

### Withdraw Pipeline

```
POST /v1/withdraw
  → Idempotency check (with in-flight marker)
  → Balance check (same as transfer)
  → INSERT tx_log(debit, pending)
  → UPDATE balance_snapshot
  → POST to bank adapter with SNAP headers
     (HMAC_SHA512, X-TIMESTAMP, X-EXTERNAL-ID, X-PARTNER-ID, CHANNEL-ID)
  → On success: tx_log committed, clear in-flight marker
  → On timeout (60s): call reversal endpoint, max 3 retries × 15s
     - reversal success: tx_log compensated, balance restored
     - reversal failure after 3 retries: tx_log stays pending, ops alert
  → On explicit failure: tx_log compensated, balance restored
```

### Batch Fee Processing (DoD style)

```go
// Process as arrays, not row-by-row
func BatchProcessFees(txs []Transaction, rules FeeRules) []FeeEvent {
    amounts := make([]int64, len(txs))
    for i, tx := range txs {
        amounts[i] = int64(tx.AmountSen)
    }
    fees := calculateFeesBatch(amounts, rules)
    events := make([]FeeEvent, len(txs))
    for i := range txs {
        events[i] = FeeEvent{TxID: txs[i].ID, FeeSen: Money(fees[i])}
    }
    return events
}
```

### Balance Projection (DoD style)

```go
// Scan + reduce, not row-by-row UPDATE
func ProjectBalances(txLogs []Transaction) map[UserID]int64 {
    bal := make(map[UserID]int64)
    for _, tx := range txLogs {
        if tx.Status != TxCommitted { continue }
        bal[tx.SenderID] -= int64(tx.AmountSen)
        bal[tx.ReceiverID] += int64(tx.AmountSen)
    }
    return bal
}
```

---

## File-by-File Structure

```
cmd/
├── server/main.go             # Init config, wire deps, start HTTP
└── tui/main.go                # tea.NewProgram, instantiate client

internal/
├── ledger/
│   ├── types.go               # TransferCmd, TxEvent, BalanceDelta
│   ├── core.go                # ExecuteTransfer — pure function
│   ├── core_test.go           # table-driven + property tests
│   ├── store.go               # LedgerStore interface
│   ├── postgres.go            # PostgresLedgerStore impl
│   └── postgres_test.go       # testcontainers contract tests
├── fee/
│   ├── core.go                # CalcFee, CalcFees(batch) — pure
│   ├── core_test.go
│   ├── store.go               # FeeStore interface
│   └── postgres.go
├── auth/
│   ├── core.go                # HashPIN, VerifyPIN, ValidatePhone — pure
│   ├── core_test.go
│   ├── handler.go             # register/login/verifyKYC handlers
│   ├── handler_test.go        # httptest.Server tests
│   ├── jwt.go                 # JWT issue + validate
│   ├── jwt_test.go
│   ├── store.go               # UserStore interface
│   └── postgres.go
├── gateway/
│   ├── router.go              # mux setup, route registration
│   ├── middleware.go          # auth, rate-limit, panic recovery, req_id, logging
│   └── middleware_test.go
├── bank/
│   ├── interface.go           # PaymentRail { CreateVA, Transfer, Withdraw, Status }
│   ├── provider_stub.go       # Simulated provider (no real bank)
│   └── provider_stub_test.go
├── notif/
│   ├── interface.go           # Notifier { SendPush, SendSMS }
│   ├── provider_stub.go
│   └── consumer.go            # NATS subscriber → dispatch
├── idempotency/
│   ├── core.go                # Check(key, seen) → { Allow, Duplicate, Expired }
│   ├── core_test.go
│   ├── store.go               # IdempotencyStore interface
│   └── redis.go               # RedisIdempotencyStore
├── projection/
│   ├── core.go                # ProjectBalances — pure function
│   ├── core_test.go
│   ├── store.go               # ProjectionStore interface
│   └── postgres.go
├── saga/
│   ├── coordinator.go         # Step[T](), Compensate(), Retry(3, backoff)
│   └── coordinator_test.go
├── featureflag/
│   ├── core.go                # Check(flag) → bool — pure
│   └── core_test.go
├── telemetry/
│   ├── log.go                 # Init structured JSON logger, RequestID middleware
│   ├── metrics.go             # Prometheus counters/gauges/histograms
│   ├── trace.go               # OpenTelemetry span context propagation
│   ├── middleware.go          # HTTP metrics middleware
│   └── telemetry_test.go
├── config/
│   └── config.go              # Env vars → Config struct
└── types/
    ├── money.go               # Money(int64), Validate(), SafeAdd()
    ├── user.go                # User, UserID
    ├── tx.go                  # Transaction, TxType, TxStatus
    └── errors.go              # DomainError, all error vars

migrations/
├── 001_create_users.up.sql
├── 001_create_users.down.sql
├── 002_create_tx_log.up.sql
├── 002_create_tx_log.down.sql
├── 003_create_balance_snapshot.up.sql
├── 003_create_balance_snapshot.down.sql
├── 004_create_idempotency_keys.up.sql
└── 004_create_idempotency_keys.down.sql

docs/adr/
├── 001-money-type.md
├── 002-append-only-ledger.md
├── 003-idempotency-strategy.md
├── 004-fcis-package-structure.md
├── 005-serializable-isolation.md
└── 006-crash-early-philosophy.md
```

---

## Database Schema

### users

```sql
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone       TEXT NOT NULL UNIQUE,
    email       TEXT,
    pin_hash    TEXT NOT NULL,
    kyc_level   TEXT NOT NULL DEFAULT 'basic' CHECK (kyc_level IN ('basic','verified')),
    full_name   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_phone ON users(phone);
```

### tx_log (append-only)

```sql
CREATE TABLE tx_log (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key  TEXT NOT NULL UNIQUE,
    tx_type          TEXT NOT NULL CHECK (tx_type IN ('topup','transfer','withdraw','fee')),
    sender_id        UUID REFERENCES users(id),
    receiver_id      UUID REFERENCES users(id),
    amount_sen       BIGINT NOT NULL CHECK (amount_sen > 0),
    currency         TEXT NOT NULL DEFAULT 'IDR',
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','committed','failed','compensated')),
    failure_reason   TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    committed_at     TIMESTAMPTZ
);
CREATE INDEX idx_tx_log_sender    ON tx_log(sender_id, created_at DESC);
CREATE INDEX idx_tx_log_receiver  ON tx_log(receiver_id, created_at DESC);
CREATE INDEX idx_tx_log_created   ON tx_log(created_at);
```

### balance_snapshot

```sql
CREATE TABLE balance_snapshot (
    user_id     UUID PRIMARY KEY REFERENCES users(id),
    balance_sen BIGINT NOT NULL DEFAULT 0 CHECK (balance_sen >= 0),
    version     INT NOT NULL DEFAULT 1,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed: new user gets zero balance on register
CREATE OR REPLACE FUNCTION create_balance_snapshot()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO balance_snapshot (user_id, balance_sen, version)
    VALUES (NEW.id, 0, 1);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_create_balance AFTER INSERT ON users
    FOR EACH ROW EXECUTE FUNCTION create_balance_snapshot();
```

### idempotency_keys

```sql
CREATE TABLE idempotency_keys (
    key        TEXT PRIMARY KEY,
    response   JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_idempotency_expires ON idempotency_keys(expires_at);
```

---

## ADR Index

| # | Title | Decision |
|---|-------|----------|
| 001 | Money type as int64 sen | Never float. Overflow checked at boundary. |
| 002 | Append-only ledger | tx_log never mutated. Balance is projected snapshot. |
| 003 | Idempotency via client key + Redis TTL | Double-submit protection. TTL matches SLA. |
| 004 | FCIS core.go convention | Pure code separated from I/O at package boundary. Dependency direction by convention. |
| 005 | PostgreSQL serializable isolation | No lost updates. Retry(3) + compensate if exhausted. |
| 006 | Crash-early error philosophy | Detect impossible state → abort with diagnostic. |
| 007 | JWT token lifetime | Access token 30 min, refresh token 7d. Industry standard per GoPay/DANA. |
| 008 | SNAP protocol simulation | HMAC_SHA512, X-TIMESTAMP, X-EXTERNAL-ID headers. Mock bank server. Zero cost. |

---

## Testing Strategy

### Test Split

| Layer | Tooling | Approx Count | CI Gate |
|-------|---------|--------------|---------|
| Core unit | `testing` + table-driven | 200-400 | Every commit |
| Core property | `rapid` (Go property-based) | 20-40 | Every commit |
| Shell contract | `testcontainers-go` | 30-50 | PR |
| Shell integration | `testcontainers-go` + `httptest` | 10-20 | PR merge |
| E2E | Docker compose + tuistory | 3-5 | Nightly |

### Core Test Example

```go
func TestExecuteTransfer(t *testing.T) {
    tests := []struct{
        name     string
        sender   types.Money
        receiver types.Money
        amount   types.Money
        want     *ledger.TxEvent
        wantErr  bool
        errCode  string
    }{
        {"success", 100_000, 50_000, 10_000,
            &ledger.TxEvent{SenderNewBal: 90_000, ReceiverNewBal: 60_000}, false, ""},
        {"insufficient", 5_000, 50_000, 10_000, nil, true, "INSUFFICIENT_BALANCE"},
        {"zero amount", 100_000, 50_000, 0, nil, true, "INVALID_AMOUNT"},
        {"self transfer", 100_000, 100_000, 10_000, nil, true, "SELF_TRANSFER"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ledger.ExecuteTransfer(tt.sender, tt.receiver, tt.amount)
            if tt.wantErr {
                var de *types.DomainError
                assert.ErrorAs(t, err, &de)
                assert.Equal(t, tt.errCode, de.Code)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}
```

### Property Test Example

```go
func TestBalanceInvariant(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        sBal := types.Money(rapid.Int64Range(0, 1_000_000_00).Draw(t, "sender"))
        rBal := types.Money(rapid.Int64Range(0, 1_000_000_00).Draw(t, "receiver"))
        amt  := types.Money(rapid.Int64Range(1, int64(min(sBal, 1_000_000_00))).Draw(t, "amount"))

        event, err := ledger.ExecuteTransfer(sBal, rBal, amt)
        if err != nil { return }

        // invariant: total money conserved
        assert.Equal(t, int64(sBal)+int64(rBal),
            int64(event.SenderNewBal)+int64(event.ReceiverNewBal))
        // invariant: sender decreased
        assert.Less(t, event.SenderNewBal, int64(sBal))
        // invariant: receiver increased
        assert.Greater(t, event.ReceiverNewBal, int64(rBal))
    })
}
```

---

## CI Pipeline (.github/workflows/ci.yml)

```yaml
on: [push, pull_request]
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
      - run: golangci-lint run ./...
      - run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - run: govulncheck ./...

  test-core:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./internal/... -run 'Test.*Core|Property' -short -count=1 -v

  test-shell:
    needs: [lint, test-core]
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env: { POSTGRES_PASSWORD: test, POSTGRES_DB: ewallet_test }
      redis:
        image: redis:7-alpine
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go run ./migrations/... up
      - run: go test ./internal/... -run 'Test.*Contract|Integration' -count=1 -v
```

---

## Implementation Phases

```
Phase 1: Foundation
  types/ — Money, User, Transaction, DomainError
  config/ — env parsing
  telemetry/ — logger, metrics, panic middleware
  migrations/ — 4 SQL files (up + down)
  justfile, Dockerfile, docker-compose.yml, .github/workflows/ci.yml
  README skeleton

Phase 2: Core Engine (pure, no I/O)
  ledger/core.go + core_test.go
  fee/core.go + core_test.go
  idempotency/core.go + core_test.go
  projection/core.go + core_test.go
  auth/core.go + core_test.go

Phase 3: Persistence (shell adapters)
  auth/postgres.go
  ledger/postgres.go + postgres_test.go
  idempotency/redis.go
  projection/postgres.go

Phase 4: HTTP + Auth
  gateway/router.go + middleware.go
  auth/handler.go + jwt.go
  gateway/middleware_test.go
  Smoke test: register → login → balance

Phase 5: Transfer Orchestrator
  saga/coordinator.go + coordinator_test.go
  Wire: handler → saga → ledger (core) → postgres (shell)
  HTTP /v1/transfer handler
  Smoke test: topup → transfer → balance check

Phase 6: Adapters
  bank/interface.go + provider_stub.go + test
  bank/snap.go + snap_test.go (SNAP protocol: HMAC_SHA512, headers, verification)
  bank/snap_mock_server.go (in-process mock bank)
  bank/provider_snap.go (adapter using SNAP to call mock)
  notif/interface.go + provider_stub.go + consumer.go
  HTTP /v1/topup, /v1/withdraw handlers
  Smoke test: full 6 flows

Phase 7: TUI
  cmd/tui/model/*.go (login, dashboard, transfer, history, topup)
  cmd/tui/client/api.go HTTP client
  cmd/tui/tui_test.go tuistory tests
  E2E: TUI → API → PG → Redis

Phase 8: Polish
  docs/adr/ 001-006
  README architecture section + diagrams
  CI fix (any flaky)
  Final review pass
```

---

## justfile

```just
.PHONY: test build run lint docker-all

test:
    go test ./internal/... -short -count=1

build:
    go build ./cmd/server

run:
    go run ./cmd/server

lint:
    golangci-lint run ./...

docker-all:
    docker compose up --build
```

---

## What Success Looks Like

1. **Clone → `just docker-all` → `go run ./cmd/tui`** — 6 working screens
2. **README** — architecture explained in 60 seconds
3. **`internal/ledger/core.go`** — clean pure function, overflow check, crash-early
4. **ADRs** — deliberate tradeoff thinking documented
5. **CI badge** — green, lint clean, no critical vulns
6. **Error messages** — Indonesian, typed, consistent across all endpoints
7. **Tests** — 200+ core tests run in <1s, contract tests run against real infrastructure
