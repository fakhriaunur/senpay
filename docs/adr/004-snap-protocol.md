# ADR 004: Implement SNAP Standard for Bank Integration

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

Senpay needs to integrate with Indonesian banks for top-up (virtual account) and withdraw (disbursement) operations. The SNAP (Sistem Navigasi Antar Payment) standard, defined by Bank Indonesia and ASPI, is the mandated protocol for open API payments in Indonesia. Key requirements:

- All inter-bank requests must be signed with HMAC-SHA512
- Standardized headers for auditability across the ecosystem
- Support for simulated bank behavior during development
- Ability to swap between stub (development) and real SNAP (production) providers

## Decision

Adopt the **SNAP standard** for all bank adapter communication:

### Mandatory Headers

Every outbound request to the bank adapter includes:

| Header | Value | Example |
|--------|-------|---------|
| `X-TIMESTAMP` | ISO 8601 UTC timestamp | `2026-05-15T10:00:00Z` |
| `X-SIGNATURE` | HMAC-SHA512 of `stringToSign` | `a1b2c3d4e5f6...` |
| `X-PARTNER-ID` | Partner identifier from bank | `SENPAY_PROD` |
| `X-EXTERNAL-ID` | Unique request identifier | UUID v7 |
| `CHANNEL-ID` | Channel identifier | `MOBILE` |

### Signature Algorithm

```go
stringToSign = X-TIMESTAMP + "|" + X-EXTERNAL-ID + "|" + requestBody
X-SIGNATURE = HMAC_SHA512(stringToSign, clientSecret)
```

### Provider Abstraction

The bank adapter interface (`PaymentRail`) has two implementations:

- **`provider_snap.go`**: Real SNAP adapter — signs requests, sends to bank server, validates responses
- **`provider_stub.go`**: Development stub — returns canned responses, zero network I/O

Selection is config-driven via `BANK_PROVIDER` env var (`stub` | `snap`).

### Mock Bank Server

An in-process mock bank server validates SNAP headers on incoming requests and simulates:
- Success responses (200)
- Rejection responses (422)
- Timeouts (configurable delay, up to 60s)
- Reversal endpoint for timeout recovery

## Consequences

**Positive:**

- Compliant with Bank Indonesia/ASPI mandate
- HMAC-SHA512 provides strong request authenticity
- Provider abstraction enables development without live bank credentials
- Mock bank enables comprehensive test coverage of timeout/reversal flows

**Negative:**

- HMAC-SHA512 adds ~2ms per request (negligible)
- Mock bank must be kept in sync with real bank API changes
- Development environment needs mock bank running alongside backend

## Compliance

All bank adapter requests must include all five mandatory SNAP headers with valid HMAC-SHA512 signatures. The mock bank rejects requests with missing or invalid headers (HTTP 400/401).
