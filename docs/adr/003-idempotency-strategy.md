# ADR 003: Client-Generated Idempotency Keys with Redis Caching

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

Financial operations (transfer, top-up, withdraw) are not naturally idempotent — submitting the same request twice could double-charge a user. We need a strategy to safely retry requests without side effects. Requirements:

- Clients must be able to retry on network errors without double-spending
- Concurrent duplicate requests must be detected and handled gracefully
- Idempotency guarantee must persist across server restarts
- Performance overhead should be minimal (< 10ms per request)

## Decision

Use **client-generated idempotency keys** with a **two-phase Redis marker** approach:

### Protocol

1. **Client** generates a UUID v7 idempotency key per request and includes it in the request body.
2. **Before processing**, server checks Redis:
   - **No key found**: Proceed with processing. Set in-flight marker (TTL: 30s).
   - **In-flight marker** (TTL active): Return HTTP 202 `REQUEST_IN_FLIGHT` — concurrent duplicate.
   - **Completed marker** (24h TTL): Return cached response — safe idempotent replay.
3. **After successful processing**: Clear in-flight marker, set completed marker with cached response (24h TTL).
4. **After failed processing**: Clear in-flight marker. Client may retry with same key.
5. **After 24 hours**: Key expires. Client must generate a new key for a new operation.

### PostgreSQL Backstop

A `UNIQUE` constraint on `tx_log.idempotency_key` provides a database-level guarantee against duplicate commits, even if Redis data is lost.

### Why Redis + PostgreSQL, Not PostgreSQL Alone

- Redis SETNX is faster (< 1ms) than serializable PG transactions for duplicate detection
- Redis in-flight marker enables HTTP 202 for concurrent duplicates (better UX)
- PostgreSQL UNIQUE constraint is the safety net, not the primary check

## Consequences

**Positive:**

- Safe retry with zero risk of double-spend
- Concurrent duplicate detection with HTTP 202 feedback
- Fast path for common case (no prior key): single Redis call
- Cached responses reduce backend load for replay requests

**Negative:**

- Requires Redis availability for idempotency check (but falls back to PG constraint)
- 24h TTL means idempotency keys occupy Redis memory (~100 bytes per key)
- Clients must generate UUID keys — cannot be anonymous
- Server restart loses in-flight markers (potential for concurrent race on restart)

## Compliance

Every mutating endpoint (`POST /v1/transfer`, `POST /v1/topup`, `POST /v1/withdraw`) MUST require an `idempotency_key` field. Missing keys are rejected with HTTP 400.
