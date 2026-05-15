# ADR 001: Adopt Functional Core / Imperative Shell Architecture

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

Senpay is an Indonesian e-wallet system where correctness and auditability are paramount. We need an architecture that:

- Makes business logic testable without I/O or mocks
- Keeps financial invariants (money conservation, append-only ledger) easy to verify
- Allows shell adapters (PostgreSQL, Redis, NATS, HTTP) to be swapped without affecting core logic
- Avoids framework lock-in

## Decision

Adopt the **Functional Core / Imperative Shell (FCIS)** pattern:

- **Core layer** (`core.go` files): Pure functions. Import only stdlib + `internal/types`. No I/O, no network, no filesystem, no `time.Now()`. All dependencies passed as parameters. Deterministic: same input → same output.

- **Shell layer** (`postgres.go`, `redis.go`, `handler.go`, `service.go`): Implement core interfaces. Call core functions with data from the external world. Never contain business logic.

- **Store interfaces** (`store.go`): Defined in core packages, implemented by shell adapters.

### Structure per package

```
internal/ledger/
├── core.go          # Pure functions (ExecuteTransfer)
├── core_test.go     # Table-driven + property-based tests
├── store.go         # LedgerStore interface
├── postgres.go      # PostgresLedgerStore implements LedgerStore
└── postgres_test.go # testcontainers-go contract tests
```

### Dependency direction

```
handler → service → core (never reverse)
```

Shell adapters implement core interfaces — the core never imports shells.

## Consequences

**Positive:**

- Core business logic testable without mocks, containers, or I/O
- Property-based tests (via `rapid`) verify invariants like money conservation
- Shell adapters are thin and easy to review
- Infrastructure changes (e.g., MySQL → PostgreSQL) only affect shell layer

**Negative:**

- More files per package (core vs shell split)
- Requires discipline to keep pure boundaries — easy to accidentally pull in I/O
- Some operations (e.g., generating UUID v7, timestamps) must be passed as parameters or lifted to shell

## Compliance

All new packages must follow FCIS split. Code review enforces that `core.go` files have zero I/O imports. Violations are caught by `go vet` integration in CI.
