# Senpay — Production-Grade Indonesian E-Wallet 

![Version](https://img.shields.io/badge/version-v0.2.0-blue)

**Senpay** is a production-grade Indonesian e-wallet backend + TUI demo, designed for fintech engineering interviews at GoPay/OVO/DANA/ShopeePay. Architecture mirrors real industry patterns: Go 1.26, PostgreSQL 18.3, Redis 8.6.3, NATS 2.14.0, FCIS (Functional Core / Imperative Shell), SNAP protocol simulation, append-only ledger, saga orchestration.

Guided by *The Pragmatic Programmer* principles: tracer bullets (end-to-end before business logic), orthogonality (core.go pure, postgres.go adapter), design by contract (function signatures = contracts), crash-early (overflow checks, DomainError taxonomy).

---

## What's New in v0.2.0

- **PDV Type System**: `KYCLevel`, `TxType`, `TxStatus`, `VAStatus`, `BankProvider`, and other string discriminators replaced with typed newtypes. Each type has a `Parse()` constructor that validates at the boundary. Zero raw string comparisons remain in business logic — all matching is done on the typed variant.
- **Config-Driven Fees**: `fees.yaml` drives fee calculation: flat fee for basic KYC, percentage rate for verified KYC, configurable minimum fee floor. Promo codes support discount percentages and free-transfer time windows. Parse-Don't-Validate at startup (crash-early on malformed config).
- **i18n Bilingual**: TUI labels available in Indonesian (`id`) and English (`en`) via locale JSON files (~177 keys). API error messages respond to `Accept-Language` header, returning `id` or `en` messages.
- **Senpai-Full Nudge Engine**: Velocity rate monitoring, trend detection, anomaly flagging, and exhaustion projection rules power a financial nudge system. Dashboard nudge cards include severity coloring (green/yellow/red). Optional LLM-powered tips via a provider-agnostic adapter supporting OpenAI-compatible, OpenAI Responses, and Anthropic Messages APIs.

---

## Architecture

```
┌──────────────┐     HTTP/WS    ┌───────────────────┐     pgx/SQL      ┌────────────┐
│   TUI (:0)   │────────────────→│   Backend (:8384)  │───────────────→│ PostgreSQL │
│  Bubble Tea  │                 │  net/http ServeMux  │               │ :5432      │
│  6 screens   │                 │                     │               └────────────┘
└──────────────┘                 │  ┌──service/──────┐ │                    ↑
                                 │  │ wallet.go      │ │     Redis SETNX     │
                                 │  │ transfer.go    │─┼──────────────────┐  │
                                 │  │ auth.go        │ │                  ↓  │
                                 │  │ bank.go        │ │              ┌──────┴───┐
                                 │  └────────────────┘ │              │  Redis   │
                                 │  ┌──core/──────────┐ │              │ :6379    │
                                 │  │ ledger.go       │ │              └──────────┘
                                 │  │ fee.go          │ │                    ↑
                                 │  │ auth_core.go    │ │   NATS tx.event     │
                                 │  └─────────────────┘ │              ┌──────┴───┐
                                 │  ┌──adapter/───────┐ │────────────→│  NATS    │
                                 │  │ postgres.go     │ │             │ :4222    │
                                 │  │ redis.go        │ │             └──────────┘
                                 │  │ snap.go         │ │
                                 │  └─────────────────┘ │
                                 └───────────────────────┘
                                          │
                               SNAP HMAC_SHA512
                                          ↓
                                 ┌───────────────────┐
                                 │   Mock Bank (:0)   │
                                 │  (in-process)      │
                                 │  GET /bank/health   │
                                 │  POST /bank/api/v1/*│
                                 └───────────────────┘
```

### FCIS Package Convention

Every I/O package follows Functional Core / Imperative Shell:

```
internal/ledger/
├── core.go          # Pure functions — no I/O, no side effects
├── core_test.go     # Table-driven + property-based tests (rapid)
├── store.go         # LedgerStore interface
├── postgres.go      # PostgresLedgerStore — shell adapter
└── postgres_test.go # Contract tests
```

**Core rules:**
- Imports only stdlib + `internal/types`
- No I/O, no network, no filesystem, no `time.Now()`
- Deterministic: same input → same output
- All dependencies passed as parameters

**Shell rules:**
- Implements core interfaces
- Calls core functions with data from external world
- Never contains business logic

---

## Technology Stack

| Component         | Technology                    | Version  | Purpose                          |
|-------------------|-------------------------------|----------|----------------------------------|
| Language          | Go                            | 1.26.3   | Backend + TUI runtime            |
| Database          | PostgreSQL                    | 18.3     | Users, tx_log, balance snapshots |
| Cache             | Redis                         | 8.6.3    | Idempotency keys, in-flight markers |
| Messaging         | NATS                          | 2.14.0   | Async events (tx.completed)      |
| TUI Framework     | Bubble Tea + Bubbles + Lipgloss | 1.3.x  | Terminal UI (6 screens)          |
| HTTP Router       | net/http ServeMux (Go 1.22+)  | stdlib   | REST API routing                 |
| DB Driver         | pgx                           | 5.9.2   | PostgreSQL driver                |
| Redis Client      | go-redis                      | 9.19.0  | Redis operations                 |
| NATS Client       | nats.go                       | 1.52.0  | NATS pub/sub                     |
| YAML Config       | yaml.v3                       | 3.0.1   | fees.yaml + locale parsing       |
| JWT               | golang-jwt                    | 5.2.1   | Access (30min) + refresh (7d)    |
| UUID              | google/uuid                   | 1.6.0   | UUID v7 primary keys             |
| PIN Hashing       | bcrypt (golang.org/x/crypto)  | stdlib  | PIN storage (cost 12)            |
| Property Testing  | rapid                         | 1.2.0   | Invariant property-based tests   |
| Container Runtime | Docker + Compose              | -       | PostgreSQL, Redis, NATS          |
| Task Runner       | just                          | -       | Build/test/db commands           |

---

## Quick Start

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- `just` command runner (optional, can use raw commands)

### 1. Infrastructure

```bash
# Start PostgreSQL, Redis, and NATS
docker compose up -d

# Wait for all services to be healthy
docker compose ps
```

### 2. Configuration

Copy the example env file and adjust as needed:

```bash
cp .env.example .env
# Edit .env with your settings (defaults work for local dev)
```

Required env vars (`.env`):
```
DATABASE_URL=postgres://senpay:senpay_dev@localhost:5432/senpay?sslmode=disable
REDIS_URL=redis://localhost:6379
NATS_URL=nats://localhost:4222
JWT_SECRET=change-me-in-production
SENPAI_FULL_ENABLED=false
BANK_PROVIDER=stub
```

### 3. Run Migrations

```bash
# Using just
just db-migrate

# Or directly
go run ./cmd/migrate up
```

### 4. Start the Server

```bash
# Using just
just run-server

# Or directly
go run ./cmd/server
```

Server starts on `http://127.0.0.1:8384`. Health check: `curl http://127.0.0.1:8384/health`

### 5. Start the TUI (Optional)

In a separate terminal:

```bash
# Using just
just run-tui

# Or directly
go run ./cmd/tui
```

---

## API Reference

All amounts in **sen** (1 IDR = 100 sen). Protected endpoints require `Authorization: Bearer <access_token>` header.

### Public Endpoints

| Method | Path                  | Description                          | Auth |
|--------|-----------------------|--------------------------------------|------|
| GET    | `/health`             | Health check                         | No   |
| GET    | `/metrics`            | Prometheus metrics                   | No   |
| POST   | `/v1/auth/register`   | Register new user                    | No   |
| POST   | `/v1/auth/login`      | Login with phone + PIN               | No   |
| POST   | `/v1/auth/refresh`    | Refresh JWT tokens (uses refresh_token) | No* |

\* Refresh endpoint validates its own token type, not Bearer access token.

### Protected Endpoints

| Method | Path                          | Description                                 |
|--------|-------------------------------|---------------------------------------------|
| POST   | `/v1/auth/kyc`                | KYC verification (basic → verified)         |
| GET    | `/v1/auth/me`                 | Current user profile                        |
| GET    | `/v1/balance`                 | Current balance (from balance_snapshot)     |
| GET    | `/v1/wallet/balance`          | Projected balance (from tx_log)             |
| POST   | `/v1/transfer`                | Send money to another user (supports `promo_code`) |
| GET    | `/v1/transactions`            | Transaction history (cursor paginated)      |
| GET    | `/v1/transactions/{id}`       | Transaction detail                          |
| POST   | `/v1/topup`                   | Top-up via Virtual Account                  |
| POST   | `/v1/withdraw`                | Withdraw to bank account                    |
| GET    | `/v1/senpai/summary`          | Monthly spending by category                |
| GET    | `/v1/senpai/trend`            | 6-month spending trend                      |
| POST   | `/v1/senpai/budgets`          | Create category budget                      |
| GET    | `/v1/senpai/budgets`          | List budgets with spending vs limit         |
| GET    | `/v1/senpai/nudge`            | Nudge cards: velocity, trend, anomaly, exhaustion (LLM tips optional) |

### Mock Bank Endpoints (In-Process)

| Method | Path                         | Description                | Auth   |
|--------|------------------------------|----------------------------|--------|
| GET    | `/bank/health`               | Mock bank health check     | No     |
| POST   | `/bank/api/v1/credit`        | Credit/VA payment          | SNAP   |
| POST   | `/bank/api/v1/withdraw`      | Withdraw debit             | SNAP   |
| POST   | `/bank/api/v1/reversal`      | Withdraw reversal          | SNAP   |
| POST   | `/bank/webhook`              | Bank webhook callback      | No†    |

† Webhook endpoint is called by mock bank internally after simulating payment.

### Request/Response Format

All responses follow a consistent envelope:

```json
// Success
{"data": { ... }}

// Error
{"error": {"code": "ERROR_CODE", "message": "Pesan error dalam Bahasa Indonesia"}}
```

### Error Codes

| Code                      | HTTP Status | Description                               |
|---------------------------|-------------|-------------------------------------------|
| `INVALID_PIN`             | 401         | PIN salah                                 |
| `UNAUTHORIZED`            | 401         | Sesi habis, silakan login ulang           |
| `USER_NOT_FOUND`          | 404         | Pengguna tidak ditemukan                  |
| `PHONE_ALREADY_REGISTERED`| 409         | Nomor telepon sudah terdaftar             |
| `INVALID_AMOUNT`          | 400         | Jumlah tidak valid                        |
| `INSUFFICIENT_BALANCE`    | 400         | Saldo tidak cukup                         |
| `SELF_TRANSFER`           | 400         | Tidak bisa transfer ke diri sendiri       |
| `EXCEEDS_TRANSACTION_LIMIT`| 400        | Melebihi batas transaksi                  |
| `REQUEST_IN_FLIGHT`       | 202         | Permintaan sedang diproses                |
| `SERIALIZATION_CONFLICT`  | 409         | Silakan coba lagi                         |
| `DUPLICATE_TRANSACTION`   | 409         | Transaksi duplikat                        |
| `LIMIT_EXCEEDED`          | 422         | Batas transaksi harian terlampaui         |

---

## TUI Screens

| Screen    | Description                                  |
|-----------|----------------------------------------------|
| Login     | Phone + masked PIN entry                     |
| Dashboard | Balance (IDR format), quick actions, senpai tip |
| Transfer  | Recipient phone, amount, description field   |
| History   | Cursor-paginated list with color-coded amounts |
| Top-up    | Amount + method selector + VA number display |
| Withdraw  | Amount + bank account + confirmation         |

Launch: `go run ./cmd/tui` (requires backend running on `:8384`)

---

## Project Structure

```
senpay/
├── cmd/
│   ├── server/       # Backend HTTP server entry point
│   ├── tui/          # Bubble Tea TUI entry point
│   └── migrate/      # Database migration runner
├── internal/
│   ├── auth/         # Auth: register, login, JWT, KYC, middleware
│   ├── bank/         # Bank adapter: SNAP, mock server, VA, withdraw
│   ├── config/       # Environment config loader
│   ├── fee/          # Fee calculation core
│   ├── gateway/      # HTTP middleware (rate-limit, recovery, req_id, BI)
│   ├── idempotency/  # Idempotency guard (Redis + PostgreSQL)
│   ├── ledger/       # Ledger core + PostgreSQL tx_log store
│   ├── nats/         # NATS client adapter
│   ├── projection/   # Balance projection core
│   ├── saga/         # Saga coordinator (retry 3 + compensate)
│   ├── senpai/       # Analytics, budgets, feature flags
│   ├── store/        # DB migrations
│   ├── telemetry/    # Metrics, OTEL trace context
│   ├── transactions/ # Transaction history handler
│   ├── transfer/     # Transfer service + handler
│   ├── tui/          # TUI screens (6 Bubble Tea screens)
│   └── types/        # Domain types (Money, User, Transaction, DomainError)
├── docs/
│   └── adr/          # Architecture Decision Records (001-008)
├── docker-compose.yml
├── justfile
├── go.mod
└── .env.example
```

---

## Testing

| Layer              | Tooling                | Count  | Command                              |
|--------------------|------------------------|--------|--------------------------------------|
| Core unit tests    | testing + table-driven | 200+   | `go test ./internal/... -short`      |
| Property tests     | rapid                  | 20-40  | `go test ./internal/... -short`      |
| Contract tests     | PostgreSQL (Docker)    | 30-50  | `go test ./internal/... -tags=integration` |
| Handler tests      | httptest               | 10-20  | `go test ./internal/... -short`      |

```bash
# Run all unit tests (quick)
just test
# or: go test ./internal/... -count=1 -timeout 60s -short

# Run linter
just lint
# or: go vet ./... && golangci-lint run ./...

# Build everything
just build
# or: go build ./cmd/server && go build ./cmd/tui
```

---

## Non-Functional Requirements

- **Money**: int64 sen, never float. Overflow-checked via `SafeAdd()`/`SafeSub()`.
- **Ledger**: Append-only `tx_log`. No `UPDATE`/`DELETE` on financial records.
- **Idempotency**: Client-generated UUID key, 24h TTL in Redis, HTTP 202 for in-flight.
- **Balance**: Projected from `tx_log` (SUM of committed entries), not a mutable column.
- **BI Limits**: Rp 2M basic KYC, Rp 10M verified KYC per transaction.
- **JWT**: 30min access token, 7d refresh token, single-use rotation.
- **Errors**: Indonesian Bahasa Indonesia, typed `DomainError{Code, Message, HTTPStatus}`.
- **SNAP**: HMAC_SHA512 signing with mandatory headers (X-TIMESTAMP, X-SIGNATURE, X-PARTNER-ID, X-EXTERNAL-ID, CHANNEL-ID).
- **i18n**: Bilingual Indonesian (id) / English (en). TUI labels via ~177 locale JSON keys. API errors respond to `Accept-Language` header.
- **Senpai-Full**: Nudge engine with velocity, trend, anomaly, exhaustion projection. Optional LLM tips via provider-agnostic adapter.

---

## CI Pipeline

GitHub Actions runs on every push:

1. **lint** — `go vet ./...` + `golangci-lint run ./...`
2. **core-test** — `go test ./internal/... -short -count=1` (200+ unit tests)
3. **shell-test** — `go test ./internal/... -tags=integration -count=1` (integration tests)

---

## License

Internal project — not for public distribution.
