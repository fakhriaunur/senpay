# ADR 008: Config-Driven Feature Flags for Deferred Functionality

**Status:** Accepted  
**Date:** 2026-05-15  
**Deciders:** Senpay Engineering Team

## Context

Senpay has features that are planned but not needed in the initial release (e.g., the full Senpai nudge engine). We need a mechanism to:

- Ship core functionality without waiting for all features to be complete
- Toggle features on/off without code changes or redeploys
- Safely develop and test incomplete features in production-like environments
- Avoid conditional logic sprawl across the codebase

## Decision

Adopt **config-driven feature flags** using environment variables:

### Flag Convention

- All flags prefixed with `SENPAI_` and suffixed with `_ENABLED`
- Default value is `false` (feature disabled)
- Flags loaded at startup in the `config` package and passed to handlers

### Current Flags

| Flag | Default | Description |
|------|---------|-------------|
| `SENPAI_FULL_ENABLED` | `false` | Enables full Senpai features (nudge engine, advanced analytics) |

### Usage Pattern

Feature flags are checked at the handler boundary, not in core logic:

```go
func (h *SenpaiHandler) Nudge(w http.ResponseWriter, r *http.Request) {
    if !h.cfg.SenpaiFullEnabled {
        writeError(w, types.ErrFeatureNotAvailable)
        return
    }
    // ... actual implementation ...
}
```

### Design Principles

1. **Fail closed**: If a flag is misconfigured, the feature is disabled (safe default)
2. **Handler-level gate**: Flags checked at HTTP handler entry, not in core functions
3. **Config-only**: No per-user or gradual rollout flags — binary enable/disable per deployment
4. **Documented**: All flags listed in config documentation with defaults and descriptions

## Consequences

**Positive:**

- Ship incomplete features behind flags without affecting users
- Toggle features without code changes or redeploys
- Clear boundaries between implemented and deferred functionality
- Simple pattern easy to understand and audit

**Negative:**

- Binary flags only — no gradual rollout, A/B testing, or per-user targeting
- Code paths for disabled features still compiled into binary
- Flag sprawl risk if too many features are gated
- Requires discipline to remove flags once features are stable

## Compliance

Every feature-flagged endpoint checks the config flag at the handler level before proceeding. If a flag is missing or misconfigured, the feature behaves as disabled (HTTP 501 `FEATURE_NOT_AVAILABLE`).
