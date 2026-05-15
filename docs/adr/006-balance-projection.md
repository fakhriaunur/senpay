# ADR 006: Project Balance from Append-Only Transaction Log

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

In financial systems, balances can be stored in two ways:

1. **Mutable column**: Directly update a `balance` column on the user row
2. **Projected**: Compute balance by summing committed transaction log entries

The mutable column approach is simpler but has significant drawbacks for auditability and correctness.

## Decision

Use the **append-only projection** model:

- **Primary record of truth**: `tx_log` table — append-only, never updated or deleted
- **Balance**: Computed by summing all committed credits minus debits for a user
- **Balance snapshot** (`balance_snapshot` table): Cached projection for O(1) reads, refreshed atomically within the same SERIALIZABLE transaction that appends to `tx_log`
- **Recovery**: If balance_snapshot is corrupted, it can be rebuilt from `tx_log`

### Projection Query (Conceptual)

```sql
SELECT COALESCE(
  SUM(CASE WHEN receiver_id = $1 THEN amount_sen ELSE 0 END) -
  SUM(CASE WHEN sender_id = $1 THEN amount_sen ELSE 0 END),
  0
) FROM tx_log
WHERE status = 'committed'
  AND (sender_id = $1 OR receiver_id = $1);
```

### Caching via balance_snapshot

For performance, `balance_snapshot` stores the projected balance with an optimistic lock (`version` column). Updates follow:

```sql
UPDATE balance_snapshot
SET balance_sen = $1, version = version + 1, updated_at = NOW()
WHERE user_id = $2 AND version = $3;
```

This prevents lost updates. If the version doesn't match, the SERIALIZABLE transaction retries.

## Consequences

**Positive:**

- **Full audit trail**: Every transaction that changes a balance has an immutable log entry
- **No destructive updates**: Financial record never modified — only appended
- **Replayable**: Balance can be reconstructed from genesis at any time
- **Temporal queries**: Can compute historical balances (balance as of any date)
- **Never negative**: Projected balance can never go negative (debits require prior credits)

**Negative:**

- More complex than a simple `UPDATE users SET balance = balance + $1`
- Requires SERIALIZABLE isolation for consistent projection + snapshot update
- Full replay from tx_log is slow for high-volume users (mitigated by snapshot)
- Snapshot can drift from tx_log if bug exists (mitigated by periodic reconciliation jobs)

## Compliance

Balance must always be derived from `tx_log`. Direct `UPDATE` of balance without a corresponding `tx_log` entry is forbidden. The snapshot cache is always updated within the same transaction that appends to `tx_log`.
