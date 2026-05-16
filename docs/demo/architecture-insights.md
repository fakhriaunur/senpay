# Architecture Insights — Pragmatic Programmer Principles Applied

How PP and FCIS principles shaped every architectural decision in Senpay.

---

## Functional Core / Imperative Shell (FCIS)

> *"Separate the code that does things from the code that decides things."*
> — Adapted from Gary Bernhardt's Boundaries talk

### What We Did

Every business module splits into `core.go` (pure functions — deterministic, no I/O) and shell adapters (`handler.go`, `postgres.go`, `redis.go`). The core imports only stdlib — `errors`, `fmt`, `math`. The shell handles HTTP, databases, message queues. The core never imports shell packages.

### Why It Matters for the Interview

When the interviewer asks "how would you change the fee calculation?", the answer is: *"I change one pure function in `internal/fee/core.go`. Zero infrastructure code. Zero handler changes. The property tests in `core_test.go` verify correctness. The shell picks up the change at the interface level — no HTTP handlers or database queries are touched."*

### Code Pointers

| Evidence | Location |
|----------|----------|
| Core purity — imports only stdlib | `head -5 internal/ledger/core.go` |
| Core purity — no infra deps | `go list -f '{{.Imports}}' ./internal/ledger/` |
| Interface contract | `internal/ledger/store.go` — `LedgerStore` |
| PostgreSQL adapter | `internal/ledger/postgres.go` — implements `LedgerStore` |
| Test without database | `go test ./internal/ledger/... -short` (runs core tests only) |

---

## Parse, Don't Validate (PDV)

> *"Make illegal states unrepresentable."*
> — Yaron Minsky / Alexis King

### What We Did

Replaced 6 raw string discriminator domains (`KYCLevel`, `TxType`, `TxStatus`, `VAStatus`, `BankProvider`, `EntryType`) with typed newtypes. Each has a `Parse(s string) (T, error)` constructor that validates at the boundary. Business logic operates exclusively on typed constants — zero raw string comparisons.

### Why It Matters for the Interview

*"A typo like `== 'verifed'` doesn't compile. The type system enforces correctness at compile time, not runtime. Adding a new `KYCLevel` value requires updating the `Parse()` function and all consumers are caught by the compiler. No silent runtime failures from misspelled strings."*

### Code Pointers

| Evidence | Location |
|----------|----------|
| KYCLevel newtype | `internal/types/enums.go` — `ParseKYCLevel()` |
| TxType + TxStatus | `internal/types/enums.go` — `ParseTxType()`, `ParseTxStatus()` |
| PromoCode newtype | `internal/types/promo.go` — `ParsePromoCode()` |
| SQL CHECK constraints | `internal/store/migrations/006_*.sql`, `007_*.sql` |
| Parse tests | `internal/types/enums_test.go` — valid + invalid inputs |
| Before/after | Grep `"basic\|verified"` — no raw string comparisons remain |

---

## Design with Contracts

> *"Use contracts to document and verify that code does no more and no less than it claims."*
> — The Pragmatic Programmer, Topic 23

### What We Did

Every dependency crosses module boundaries through an interface — `LedgerStore`, `UserStore`, `IdempotencyStore`, `PaymentRail`, `NudgeLLM`. The interface IS the contract. Any implementation can be swapped behind it without changing consumers.

### Why It Matters for the Interview

*"When the interviewer asks 'what if PostgreSQL goes down?', the answer is: `LedgerStore` is an interface. We could swap in a read-replica adapter or a failover adapter without changing any handler code. The same interface powers our `storetest` fixtures — test PostgreSQL instead of production."*

### Code Pointers

| Evidence | Location |
|----------|----------|
| LedgerStore interface | `internal/ledger/store.go` |
| UserRepository interface | `internal/auth/store.go` |
| IdempotencyStore interface | `internal/idempotency/store.go` |
| PaymentRail interface | `internal/bank/provider.go` |
| NudgeLLM interface | `internal/senpai/llm/interface.go` |

---

## Tracer Bullets

> *"Build end-to-end before you build in depth."*
> — The Pragmatic Programmer, Topic 12

### What We Did

Built Senpay in 9 incremental milestones. Each milestone was a vertical slice — types → core logic → persistence → HTTP → TUI. Never a big-bang integration. Each layer was testable and demoable before the next began.

### Why It Matters for the Interview

*"The commit log shows the story — M1 shipped types and config before a single transfer could execute. M2 had pure core functions tested without infrastructure. M3 added PostgreSQL adapters. M4 wired HTTP handlers. M5 brought the TUI. Each milestone was a working, testable slice — the transfer logic was proven by M2 without any HTTP code written."*

### Code Pointers

| Evidence | Location |
|----------|----------|
| 9-milestone history | `git log --oneline \| head -40` |
| Pure core first | `internal/ledger/core.go` — no infra imports |
| Then adapters | `internal/ledger/postgres.go` — added in M3 |
| Then HTTP | `internal/transfer/handler.go` — added in M4 |
| Then TUI | `internal/tui/` — added in M7 |

---

## Crash Early

> *"A dead program normally does a lot less damage than a crippled one."*
> — The Pragmatic Programmer, Topic 26

### What We Did

Config parsing crashes at startup — missing `fees.yaml`, invalid YAML, impossible fee rates. The server won't start with bad config. No runtime surprises. The same applies to PDV types — invalid KYC level at the API boundary returns a 400, doesn't propagate into core with a default.

### Why It Matters for the Interview

*"If `fees.yaml` has a negative `flat_fee_basic_sen`, the server prints an error and exits with code 1. Not a 500 in production. Not a silent default. Crash early means the problem is detected and fixed before it reaches a customer. The same pattern applies to `ParseKYCLevel` — invalid input is rejected at the boundary, not silently coerced."_

### Code Pointers

| Evidence | Location |
|----------|----------|
| Fee config validation | `internal/fee/config.go` — `LoadFeeConfig()` |
| Crash-early behavior | `cmd/server/main.go` — `os.Exit(1)` on bad config |
| PDV boundary rejection | `internal/types/enums.go` — `Parse*()` returns `error` |
| Server won't start | `echo 0 > fees.yaml` then `go run ./cmd/server` → exits with error |

---

## Good-Enough Software

> *"Produce software that is good enough — not perfect."_
> — The Pragmatic Programmer, Topic 5

### What We Did

Deferred 17 low-priority assertions (in-flight marker timing-dependent tests, serialization failure injection, SNAP provider swap) to a deferred milestone. The core system is fully functional without them. The nudge engine is rule-based (not ML) — good enough to demonstrate financial awareness without requiring training data or external APIs.

### Why It Matters for the Interview

*"We consciously scoped down: the nudge engine uses deterministic rules rather than ML — cheaper, faster, more explainable. LLM tips are optional and feature-flagged. Config-driven fees use cold reload rather than hot reload — simpler, audit-safe, good enough. The test infrastructure for goroutine synchronization and serialization injection was deferred because the production code is proven by unit tests."_

### Code Pointers

| Evidence | Location |
|----------|----------|
| Feature-flagged nudge | `internal/senpai/handler.go` — `SENPAI_FULL_ENABLED` |
| Cold-reload fee config | `internal/fee/config.go` — no hot-reload endpoint |
| Rule-based nudges | `internal/senpai/nudge.go` — pure functions, no ML |
| Deferred assertions | `features.json` — milestone `misc-deferred-*` cancelled with justification |
