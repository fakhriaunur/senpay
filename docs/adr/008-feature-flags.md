# ADR 008: Config-Driven Feature Flags for Feature Gating

**Status:** Accepted  
**Date:** 2026-05-15 (amended 2026-05-16)  
**Deciders:** Senpay Engineering Team

## Context

Senpay uses feature flags to control rollout of optional functionality. Flags serve two purposes: gating incomplete features during development, and gating delivered features that can be toggled per deployment.

In v0.2.0, `SENPAI_FULL_ENABLED` gates the implemented Senpai-Full nudge engine (see [ADR 011](011-senpai-nudge-engine.md)). The feature flag pattern applies equally to deferred and delivered features — it controls whether a feature is active in a given deployment. We need a mechanism to:

- Ship core functionality without waiting for all features to be complete
- Toggle delivered features on/off without code changes or redeploys
- Safely develop and test features (incomplete or complete) in production-like environments
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
| `SENPAI_FULL_ENABLED` | `false` | Enables Senpai-Full nudge engine (see ADR 011) |

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

- Ship features behind flags — both deferred and delivered — without affecting users
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
