# ADR 005: Use SERIALIZABLE Isolation for Financial Transactions

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

Financial transactions (transfer, fee deduction) involve multiple row updates within a single logical operation: debit sender, credit receiver, record fee. Without proper isolation, concurrent transactions could cause:

- Lost updates (two transfers from same balance both succeed)
- Non-repeatable reads (balance changes mid-transaction)
- Write skew (concurrent transfers reference stale balance)

PostgreSQL offers four isolation levels: Read Uncommitted, Read Committed, Repeatable Read, Serializable.

## Decision

Use **SERIALIZABLE** isolation level for all financial transactions involving balance changes or tx_log entries.

### Rationale

| Requirement | Read Committed | Repeatable Read | Serializable |
|---|---|---|---|
| Prevents lost updates | ❌ | ❌ | ✅ |
| Prevents phantom reads | ❌ | ❌ | ✅ |
| Prevents write skew | ❌ | ❌ | ✅ |
| Performance overhead | None | Low | Moderate |
| Max concurrency for same account | High | High | Moderate |

SERIALIZABLE is the only level that guarantees transaction serializability — the result is equivalent to running all transactions one after another. For an e-wallet where money conservation is an invariant, this is non-negotiable.

### Retry Strategy

SERIALIZABLE transactions fail with `SQLSTATE 40001` (serialization failure) when a conflict is detected. The saga coordinator:

1. Retries up to **3 times** with exponential backoff (50ms, 250ms, 500ms)
2. If all retries fail, **compensates** any partial writes and returns `SERIALIZATION_CONFLICT` (HTTP 409)

### Transaction Template

```sql
BEGIN ISOLATION LEVEL SERIALIZABLE;
  -- 1. Check sender balance (snapshot)
  -- 2. Deduct sender
  -- 3. Credit receiver
  -- 4. Insert tx_log entries
COMMIT;  -- may fail with 40001
```

## Consequences

**Positive:**

- Strongest isolation guarantee — money conservation invariant is provable
- No explicit locking needed — PostgreSQL detects conflicts automatically
- Retry + compensate pattern handles conflicts gracefully

**Negative:**

- Higher abort rate under contention (same account, concurrent transfers)
- Requires retry logic with exponential backoff
- Performance degrades under high contention for same account
- Not suitable for hot-account scenarios (>100 concurrent transfers/second)

## Compliance

All PostgreSQL transactions that modify `balance_snapshot` or append to `tx_log` must use `SERIALIZABLE` isolation. Violations are caught during code review.
