# Demo Script — Senpay Backend Developer Interview

**Duration:** 10 minutes
**Audience:** Technical interviewer (backend engineer / architect)
**Goal:** Demonstrate FCIS architecture depth, PP principles, and a working live system

---

## Minute 0-2: Architecture Overview

> *"Let me walk you through the platform architecture before we dive into live demos."*

### Show: Project Structure

```bash
# Show the FCIS package layout — every module splits core.go + shell adapter
ls internal/
```

**Narrative:** *"Every business module follows Functional Core / Imperative Shell. `core.go` is pure Go — deterministic functions, no imports beyond stdlib. The shell layer — `postgres.go`, `handler.go` — handles all I/O and calls into the pure core. This means we can unit test business logic without Docker, without databases, without mocking."*

### Show: Core Purity

```bash
# Prove core.go imports only stdlib
head -5 internal/ledger/core.go
head -5 internal/fee/core.go
head -5 internal/senpai/nudge.go
```

**Narrative:** *"Every core file imports only `errors`, `fmt`, `math` — no database drivers, no HTTP, no infrastructure. The fee engine, transfer logic, nudge rules — all pure functions. A `CalcFee` change is covered by a unit test in 0.01s. No database needed."*

### Show: Interface Contracts

```bash
# Show the adapter interfaces — the shell contracts
head -30 internal/ledger/store.go
head -30 internal/auth/store.go
```

**Narrative:** *"Every dependency crosses module boundaries through an interface. `LedgerStore`, `UserStore`, `IdempotencyStore` — any implementation can be swapped. PostgreSQL today, Spanner tomorrow, same handler code. This is Design with Contracts — the interface defines the contract."*

### Show: Tracer Bullet History

```bash
# Commit log tells the story of incremental layering
git log --oneline -15
```

**Narrative:** *"We built this in tracer bullet layers. Foundation first (types, config, telemetry). Then core business logic with rapid property tests. Then PostgreSQL adapters. Then HTTP handlers. Then TUI. Then nudge engine. Each layer was a working milestone — never a big-bang integration."*

---

## Minute 2-4: Live Pipeline

> *"Let me show you the system running live."*

### Show: All Services Up

```powershell
# Start everything — 3 Docker services
docker compose up -d

# Verify all healthy
docker compose ps --format "table {{.Name}}\t{{.Status}}"
```

**Expected output:** 3 services with `(healthy)` — PostgreSQL, Redis, NATS.

**Narrative:** *"Three services in a single docker-compose. PostgreSQL for the append-only ledger. Redis for idempotency cache and rate limiting. NATS for event publishing. Healthchecks with proper `depends_on` — the API won't start until they're all healthy."*

### Show: Test Suite

```powershell
# Full suite green
go test ./internal/... -count=1 -timeout 60s -short
```

**Expected output:** All 20 packages passing.

**Narrative:** *"Tests cover every module. Core packages have fast deterministic tests — no database, no network. Shell packages have integration tests against real PostgreSQL and Redis. Property tests for fee calculation and balance projection — `rapid` generates thousands of random inputs to find edge cases."*

---

## Minute 4-6: Core Flow — Transfer

> *"Let me trace a transfer through the system — API → core → ledger → event."*

### Show: Register + Login

```powershell
# Register a user
curl -s -X POST http://localhost:8384/v1/auth/register `
  -H 'Content-Type: application/json' `
  -d '{"phone":"08123456789","pin":"123456"}' | python -m json.tool

# Login
$TOKEN = curl -s -X POST http://localhost:8384/v1/auth/login `
  -H 'Content-Type: application/json' `
  -d '{"phone":"08123456789","pin":"123456"}' | python -c "import sys,json; print(json.load(sys.stdin)['access_token'])"
```

**Narrative:** *"Registration creates a user, seeds a zero balance, returns a UUID. Login returns JWT access token (30min) + refresh token (7d, single-use rotation). All PINs are bcrypt-hashed at cost 12 in the pure auth core."*

### Show: Transfer with Fee Deduction

```powershell
# Transfer 500,000 sen
curl -s -X POST http://localhost:8384/v1/transfer `
  -H 'Content-Type: application/json' `
  -H "Authorization: Bearer $TOKEN" `
  -d '{"to_phone":"08123456788","amount_sen":500000}' | python -m json.tool
```

**Narrative:** *"The transfer handler validates input, checks idempotency (generates UUID v7 key), runs the saga coordinator inside a PostgreSQL SERIALIZABLE transaction. The core `ExecuteTransfer` deducts from sender, credits receiver, creates three tx_log entries — debit, credit, and fee. Fee is 2,500 sen flat for basic KYC, or 0.7% for verified. All configurable via `fees.yaml`."*

### Show: Balance + History

```powershell
# Check balance
curl -s http://localhost:8384/v1/wallet/balance `
  -H "Authorization: Bearer $TOKEN" | python -m json.tool

# Transaction history with cursor pagination
curl -s "http://localhost:8384/v1/transactions?limit=5" `
  -H "Authorization: Bearer $TOKEN" | python -m json.tool
```

**Narrative:** *"Balance is projected live from the append-only transaction log — SUM of all committed credits minus debits. No mutable balance column. Transaction history uses cursor-based pagination — `next_cursor` + `has_more` — to avoid offset drift."*

---

## Minute 6-8: Business Engines

> *"Let me run through each business engine."*

### Nudge Engine

```powershell
# Nudge API — returns rule-based financial nudges
curl -s http://localhost:8384/v1/senpai/nudge `
  -H "Authorization: Bearer $TOKEN" | python -m json.tool
```

**Narrative:** *"The nudge engine runs 4 pure-function detectors: velocity rate (spending vs 7-day average), trend detection (3-period moving average), anomaly flagging (2 standard deviations from mean), and exhaustion projection (linear extrapolation to budget cap). No ML, no API calls — deterministic, fast, free."*

### Fee Configuration

```powershell
# Show the YAML config driving fee calculation
cat fees.yaml
```

**Narrative:** *"All fee parameters live in `fees.yaml` — parsed at startup via Parse-Don't-Validate. Invalid config means the server doesn't start. No hot-reload — cold reload only, changes are auditable in git. The fee YAML also supports promo codes with discount percentages and free-transfer windows."*

### i18n Bilingual

```powershell
# API error in Indonesian (default)
curl -s -X POST http://localhost:8384/v1/auth/login `
  -H 'Content-Type: application/json' `
  -d '{"phone":"08123456789","pin":"wrong"}' | python -m json.tool

# API error in English
curl -s -X POST http://localhost:8384/v1/auth/login `
  -H 'Content-Type: application/json' `
  -H 'Accept-Language: en' `
  -d '{"phone":"08123456789","pin":"wrong"}' | python -m json.tool
```

**Narrative:** *"Error messages follow the `Accept-Language` header. Indonesian is default, English when requested. Unknown languages fall back to Indonesian. The TUI has ~177 locale keys across both languages — switchable via a settings screen."*

---

## Minute 8-10: TUI Demo

> *"The Bubble Tea TUI is the primary demo surface."*

```powershell
# Launch TUI
go run ./cmd/tui
```

**Navigate:**

| Key | Screen |
|-----|--------|
| 1 / t | Transfer |
| 2 / u | Top-up |
| 3 / h | History |
| 4 / w | Withdraw |
| 5 / s | Settings (language toggle) |
| ? | Help overlay |
| q / Ctrl+C | Quit |

**Narrative:** *"Six screens — login, dashboard, transfer, history, top-up, withdraw. All keyboard-navigable, all bilingual. The dashboard shows balance with auto-refresh (30s ticker) and active nudge cards with severity coloring. Transfers show fee breakdown. History has scrollable cursor pagination. Settings toggles between Indonesian and English."*
