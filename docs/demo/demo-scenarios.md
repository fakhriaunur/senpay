# Demo Scenarios — Senpay Interview

Five self-contained scenarios the interviewer might ask you to demonstrate. Each has setup, commands, expected output, and the narrative to speak while executing.

---

## Scenario 1: "Show me how you'd add a new business operation"

**Interviewer prompt:** *"We need to add a bill payment feature. Walk me through the architecture decisions."*

### Setup

```powershell
docker compose up -d
```

### Demo Commands

```powershell
# Step 1: Show the FCIS pattern — core.go is the pure model
cat internal/ledger/core.go
```

**Speak:** *"Every operation follows Functional Core / Imperative Shell. A bill payment would add a new pure function in the core — `ExecuteBillPayment(sender, amount, biller)`. Pure, deterministic, no I/O. Returns a Transaction and maybe a DomainError."*

```powershell
# Step 2: Show the interface contract — shell ports
head -40 internal/ledger/store.go
```

**Speak:** *"The shell — `LedgerStore` — needs one new method: `InsertBillPayment`. PostgreSQL adapter implements it. The test library (`storetest`) already has the fixture pattern — copy `storetest/postgres.go` structure. No handler changes until the interface is solid."*

```powershell
# Step 3: Show the existing handler pattern
cat internal/transfer/handler.go
```

**Speak:** *"The handler parses HTTP, calls service, writes JSON response. A bill payment handler would be structurally identical — validate, idempotency check, call core, respond. The gateway middleware chain (rate limit, auth, BI limit) applies automatically. The TUI would add a new screen following the same `screenState` pattern."*

**The key insight:** FCIS confines the change to one core function + one adapter + one handler. No business logic leak into infrastructure code.

---

## Scenario 2: "How do you handle concurrent requests and race conditions?"

**Interviewer prompt:** *"What happens if two transfers for the same idempotency key arrive simultaneously?"*

### Setup

```powershell
docker compose up -d
# Register two users and fund sender
```

### Demo Commands

```powershell
# Step 1: Show the idempotency protocol — client generates UUID v7 key
grep -A 10 "idempotency_key" internal/transfer/handler.go
```

**Speak:** *"The client generates a UUID v7 idempotency key and sends it with every transfer request. The server uses Redis SETNX to atomically acquire an in-flight marker. If the marker already exists, it returns HTTP 202 — 'request in flight, check back.'"*

```powershell
# Step 2: Show the concurrent test proving correctness
head -80 internal/transfer/concurrent_inflight_test.go
```

**Speak:** *"This test uses goroutine barrier synchronization — goroutine A acquires the in-flight marker via SETNX and blocks on a channel. Goroutine B sends the same key and receives 202 REQUEST_IN_FLIGHT. A then completes normally. The barrier ensures deterministic ordering — no timing-dependent flakiness."*

```powershell
# Step 3: Show SERIALIZABLE isolation
grep -B 5 -A 3 "SERIALIZABLE" internal/transfer/service.go
```

**Speak:** *"The actual balance update runs inside a PostgreSQL SERIALIZABLE transaction. If two concurrent transactions conflict on the same balance row, PostgreSQL detects the serialization anomaly and returns SQLSTATE 40001. The saga coordinator retries up to 3 times with exponential backoff. If all 3 fail, it compensates — cleans up Redis, returns 409 SERIALIZATION_CONFLICT."*

```powershell
# Step 4: Show the saga retry test
head -80 internal/saga/coordinator_test.go
```

**Speak:** *"The saga test injects a mock operation that returns ErrSerializationConflict. The coordinator retries exactly 3 times, calls the compensation callback, and returns 409. Verified with 40001 error injection — no actual parallelism needed for unit testing."*

**The key insight:** Three-layer concurrency defense — idempotency key (application), Redis marker (in-flight detection), SERIALIZABLE + saga (database).

---

## Scenario 3: "Walk me through the nudge engine"

**Interviewer prompt:** *"How does the nudge engine work? Is it ML-based?"*

### Setup

```powershell
docker compose up -d
# Register user, seed some transactions with varying amounts
```

### Demo Commands

```powershell
# Step 1: Show the nudge types
grep -A 10 "type Nudge struct" internal/senpai/nudge.go
```

**Speak:** *"Rule-based, not ML. Four pure functions — velocity, trend, anomaly, exhaustion. Each takes transaction data and returns a Nudge struct. No training data, no API calls, no latency."*

```powershell
# Step 2: Show velocity nudge logic
grep -A 25 "func VelocityRate" internal/senpai/nudge.go
```

**Speak:** *"Velocity compares current spending rate to 7-day average. If current rate exceeds 1.5x the rolling average, it flags as high velocity. Pure function — sum of recent transactions divided by period, compared to baseline. Returns a typed Nudge with severity='warning', message='Pengeluaran Anda meningkat pesat'."*

```powershell
# Step 3: Show the nudge API response
curl -s http://localhost:8384/v1/senpai/nudge -H "Authorization: Bearer $TOKEN" | python -m json.tool
```

**Speak:** *"The API returns structured nudges — type, severity, message, action, dismissible. The TUI dashboard renders each nudge with severity-colored icons — yellow for warning, red for critical, blue for info. Dismissible per session via the 'x' key."*

```powershell
# Step 4: Show the LLM adapter interface (optional expansion)
head -40 internal/senpai/llm/interface.go
```

**Speak:** *"We also support optional LLM-powered tips via a provider-agnostic adapter interface. Four backends — OpenAI-compatible, OpenAI Responses API, Anthropic Messages, or disabled (default). No SDK dependencies — raw POST + JSON. Graceful fallback to rule-based on failure. The LLM layer is purely additive, not required for core nudge functionality."*

**The key insight:** Rule-based nudges are deterministic, explainable, and interview-ready. LLM is an extension point, not a dependency.

---

## Scenario 4: "How would you extend this to support multiple currencies?"

**Interviewer prompt:** *"What if we need to support USD, EUR, and IDR simultaneously?"*

### Setup

No running services needed — pure type discussion.

### Demo Commands

```powershell
# Step 1: Show the Money type — int64 in smallest unit
cat internal/types/money.go
```

**Speak:** *"Money is int64 in the smallest unit — sen for IDR, cents for USD. SafeAdd and SafeSub prevent overflow. Any new currency just needs a multiplier constant. The type system enforces no raw arithmetic."_

```powershell
# Step 2: Show the Currency type
```

**Speak:** *"We'd add a Currency newtype (PDV pattern) — `ParseCurrency("USD")`, `ParseCurrency("IDR")`. All core functions accept `types.Money` with an optional currency parameter. The exchange rate service would be a pure function in the core — `Convert(amount, from, to, rates) -> Money`."_

```powershell
# Step 3: Show how it maps to existing interfaces
```

**Speak:** *"The `LedgerStore` interface would gain a currency field. The projection core would need to handle multi-currency balance snapshots. The TUI would format with the right currency symbol. The architecture isolates the change to: core function (new pure logic) -> store interface (new field) -> handler (new JSON field). No rewrite of the transfer flow."_

**The key insight:** The FCIS architecture contains the change. PDV types ensure invalid currency values don't compile. Core functions stay pure.

---

## Scenario 5: "How do you test this system?"

**Interviewer prompt:** *"Walk me through the testing strategy."*

### Setup

```powershell
# Run all tests
go test ./internal/... -count=1 -timeout 120s -short
```

### Demo Commands

```powershell
# Step 1: Show fast core tests — pure functions, no I/O
go test ./internal/fee/... -count=1 -v
go test ./internal/senpai/... -count=1 -v
```

**Speak:** *"Core tests run in milliseconds. Pure functions — no database, no network. `CalcFee` is tested with table-driven tests covering all KYC levels, edge amounts, and floor/percentage boundary conditions. Nudge rules are tested with historical transaction data — velocity, trend, anomaly, and exhaustion scenarios."_

```powershell
# Step 2: Show property tests — random inputs
go test ./internal/ledger/... -count=1 -run TestProperty
```

**Speak:** *"Property tests use `rapid` to generate random transaction sequences. `ProjectBalances` is tested against the invariant: sum of all credits minus sum of all debits equals the projected balance. For any valid sequence of transactions, this must hold. We find edge cases that manual test cases miss."_

```powershell
# Step 3: Show integration tests — real PostgreSQL
go test ./internal/... -count=1 -tags=integration -p=1 -timeout 120s
```

**Speak:** *"Integration tests run against real Docker services — PostgreSQL, Redis, NATS. Full end-to-end: register user, transfer, check balance, verify transaction history. The store test helpers (`storetest/postgres.go`) provide reusable fixtures so every adapter test can start from a known state."_

```powershell
# Step 4: Show PDV type tests — compile-time safety
grep -A 15 "TestParseKYCLevel" internal/types/enums_test.go
```

**Speak:** *"After our v0.2.0 PDV migration, string discriminators like KYCLevel, TxType, and TxStatus are typed newtypes with Parse() constructors. A typo like `"verifed"` doesn't compile. Tests verify valid values pass and invalid values return proper DomainErrors."_

**The key insight:** Testing pyramid with FCIS — fast core tests (80% coverage), targeted shell integration tests (15%), property tests for invariant validation (5%). No mocks — real containers for integration, pure functions for unit.
