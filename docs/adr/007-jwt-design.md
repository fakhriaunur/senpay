# ADR 007: JWT-Based Authentication with Access and Refresh Tokens

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

Senpay needs a stateless authentication mechanism suitable for both HTTP API and TUI clients. Requirements:

- Short-lived access tokens (30 minutes) to limit exposure from token theft
- Refresh token rotation to detect and invalidate stolen refresh tokens
- No session state on the server (stateless auth reduces infrastructure complexity)
- JWT must carry the user ID and KYC level for middleware access control

## Decision

Adopt **dual-token JWT** authentication with refresh token rotation:

### Access Token

- **TTL**: 30 minutes
- **Claims**: `sub` (user UUID), `kyc` (KYC level), `iat`, `exp`, `type: "access"`
- **Storage**: Client memory only (never persisted to disk in TUI)
- **Signature**: HMAC-SHA256 with server secret

### Refresh Token

- **TTL**: 7 days
- **Claims**: `sub` (user UUID), `exp`, `type: "refresh"`, `jti` (unique token ID)
- **Rotation**: Each use issues a new refresh token and invalidates the old one (single-use)
- **Storage**: Client memory (TUI) or secure storage (mobile/web)

### Auth Flow

```
1. POST /v1/auth/login → returns {token, refresh_token}
2. Every API call uses Authorization: Bearer <token>
3. POST /v1/auth/refresh {refresh_token} → returns new {token, refresh_token}
4. Old refresh_token is invalidated after use
```

### Security Properties

- Access token theft limited to 30-minute window
- Refresh token reuse detected via Redis blacklist (stored as `used_refresh:{jti}` with TTL = 7 days)
- PIN never returned in any response
- bcrypt (cost 12) for PIN hashing — no plaintext storage

## Consequences

**Positive:**

- Stateless: no server-side session storage needed
- Short-lived access tokens limit exposure window
- Refresh rotation detects token theft (reused refresh token → all tokens for user invalidated)
- JWT carries KYC level for middleware enforcement without DB lookup

**Negative:**

- Requires Redis for refresh token blacklist (or DB lookup for single-use enforcement)
- Token rotation adds complexity compared to simple session cookies
- Cannot revoke access tokens without Redis blacklist (stateless by design)
- JWT size (~800 bytes) adds overhead to every API request

## Compliance

All protected endpoints verify JWT signature and expiry from `Authorization: Bearer <token>` header. JWT `type` claim distinguishes access vs refresh tokens. PIN is never returned in any response.
