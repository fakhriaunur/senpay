# Indonesian Fintech Landscape

A technical survey of the major e-wallet and payment platform players in Indonesia, their architecture, tech stacks, and engineering practices.

---

## Market Overview

Indonesia's digital payment market is dominated by five major e-wallets plus three core payment gateways, all operating under Bank Indonesia regulations and the SNAP (Standard Nasional Open API Pembayaran) standard.

| Platform | Parent | Est. Users | Type | Since |
|----------|--------|------------|------|-------|
| **GoPay** | GoTo Financial (Gojek + Tokopedia) | 29M+ active | E-wallet + payment gateway | 2016 |
| **OVO** | Grab / Lippo Group | 100M+ registered | E-wallet + financial services | 2017 |
| **DANA** | Elang Mahkota Teknologi (Emtek) | 200M+ registered | E-wallet + edtech | 2018 |
| **ShopeePay** | Sea Group (Shopee) | Tens of millions | E-wallet (in-app) | 2018 |
| **LinkAja** | Telkom / BUMN consortium | Millions | E-wallet + financial inclusion | 2019 |
| **Midtrans** | GoTo Financial | Merchant gateway | Payment gateway | 2012 |
| **Xendit** | Y Combinator-backed | Startup/merchant | Payment gateway | 2015 |
| **DOKU** | Lippo Group | Enterprise/govt | Payment gateway | 2007 |

---

## Tech Stack Comparison

### Backend Languages

```
GoPay:    Go (primary) ─── Java ─── Python
OVO:      Go (primary) ─── Java ─── PHP (legacy)
DANA:     Java (Spring Boot) ─── Go (new services)
ShopeePay: Go (primary) ─── Python ─── Java ─── C++
LinkAja:  Go (primary)
Midtrans: PHP ─── Go (SDK) ─── Python ─── Node.js
Xendit:   Node.js ─── Go ─── Python ─── Ruby
DOKU:     Java
```

**Trend**: Go is the dominant language for new service development across all platforms. DANA is the only major player still primarily Java, but explicitly hiring for Go roles.

### Databases

```
GoPay:    PostgreSQL ─── Redis ─── Kafka (Aiven managed)
OVO:      PostgreSQL ─── MySQL ─── Redis ─── Kafka
DANA:     OceanBase (from MySQL migration) ─── Redis ─── Kafka
ShopeePay: MySQL ─── TiDB ─── Redis ─── HBase ─── Kafka
LinkAja:  Unknown ─── Kafka
Midtrans: PostgreSQL ─── Redis
Xendit:   PostgreSQL ─── Redis
DOKU:     On-premise ─── MySQL
```

**Trend**: PostgreSQL common for ACID financial data. DANA notably migrated from MySQL to OceanBase (Ant Group distributed DB) for horizontal scaling. ShopeePay uses TiDB for HTAP workloads alongside MySQL.

### Architecture Patterns

```
GoPay:    Microservices ─── K8s ─── Istio service mesh ─── Gitlab CI
OVO:      Modular (feature flags) ─── Microservices ─── Load balancing ─── GCP
DANA:     3-layer SDK (Core → Use Case → Integration Kit) ─── Hybrid cloud
ShopeePay: Centralized core + country customization ─── K8s ─── Multi-region
LinkAja:  Clean Architecture ─── K8s ─── gRPC
Midtrans: Monolith ─── Strategy pattern ─── Snap UI ─── Gojek group
Xendit:   Microservices ─── K8s ─── Cloud-native
DOKU:     On-prem sensitive + cloud non-sensitive ─── PCI DSS
```

---

## Architectural Archetypes

### Archetype 1: The Ecosystem Player (GoPay, ShopeePay, OVO)

Built within larger platform ecosystems (ride-hailing, e-commerce, superapp). Start as payment method within parent app, later spin out as standalone.

Key traits:
- **Massive transaction volumes** (GoPay: 97M/year, ShopeePay: billions)
- **Feature flag driven** — OVO essential for modular release management
- **Multi-region customization** — ShopeePay single core per country
- **Centralized core services** with per-country customization layer

### Archetype 2: The Independent E-Wallet (DANA, LinkAja)

Built as standalone payment apps, not tethered to a platform. Focus on financial inclusion.

Key traits:
- **Modular SDK architecture** — DANA's 3-layer (Core → Use Case → Integration Kit)
- **Distributed databases** — DANA migrated to OceanBase for scale
- **Open API standard** — first to adopt SNAP
- **Multi-merchant integration** — DANA integrated into Bukalapak, Tix ID, BBM

### Archetype 3: The Payment Gateway (Midtrans, Xendit, DOKU)

Merchant-facing payment processors aggregating multiple payment methods.

Key traits:
- **Strategy pattern** — provider-agnostic interface, swap gateways
- **Webhook-based** — payment confirmations via async callbacks
- **SDK-first** — official Go/PHP/Node.js/Python SDKs
- **PCI DSS compliance** — for card processing

---

## Idempotency & Transaction Safety

### GoPay Pattern
- `Idempotency-Key` header (UUID, max 46 chars)
- **5 minute TTL**
- HTTP **202 Accepted** for concurrent in-flight requests
- Cached response returned on duplicate key after first completes

### OVO Pattern
- `X-EXTERNAL-ID` header
- **24 hour TTL**
- **Reversal concept**: on timeout (60s), call reversal API
- Max **3 retries at 15 second intervals**
- Manual reconciliation fallback (H+1) if reversal fails
- ISO 8583 message format for legacy Push-to-Pay

### ShopeePay Pattern
- `X-EXTERNAL-ID` + HMAC SHA-256
- Unique per day
- OAuth 2.0 + TLS 1.2+ mandatory
- No SDK — direct integration only

### DANA Pattern
- Shared/external ID across microservices
- Persistent logging of all integration requests for replay analysis
- Async retry with backoff

### Midtrans Pattern
- `order_id` must be unique
- Webhook-based status updates
- `X-Override-Notification` / `X-Append-Notification` headers for dynamic webhooks

---

## Observability & SRE

| Platform | Monitoring | Logging | Tracing | Alerting |
|----------|-----------|---------|---------|----------|
| **GoPay** | Prometheus + Grafana + Patroni | Structured (Zap) | Istio distributed tracing | Sentry |
| **OVO** | GCP Monitoring | Structured logging | GCP Trace | SRE-based |
| **DANA** | Custom + Notion automation | Persistent per-service logging | — | Predictive (Prophet/ARIMA) |
| **ShopeePay** | Prometheus + Grafana | Structured | — | — |
| **Midtrans** | Dashboard analytics | Webhook logging | — | — |
| **Xendit** | Dashboard + API | Structured | — | Postman monitors |

---

## Security & Compliance

### API Authentication Methods

| Platform | Auth | Signature | Token Lifetime |
|----------|------|-----------|----------------|
| GoPay | OAuth 2.0 Bearer | — | 30 min |
| OVO | HMAC + RSA | HMAC_SHA256 / RSA | Per-request |
| ShopeePay | OAuth 2.0 + HMAC + RSA | HMAC_SHA512 + SHA256withRSA | Access + refresh |
| DANA | OAuth 2.0 | — | 30 min |
| Midtrans | Server Key + OAuth 2.0 | — | Per-request |
| Xendit | Secret API Key | — | Static key |
| DOKU | Client ID + Signature | HMAC_SHA256 | Per-request |

### Regulatory Constraints

| Regulation | Requirement | Affects |
|------------|------------|---------|
| PBI No. 20/2019 | E-wallet max Rp 2M (unverified), Rp 10M (verified) | All e-wallets |
| POJK 12/2017 | KYC/AML for e-money issuance | All e-wallets |
| OJK 13/2018 | Digital financial innovation registration | All fintech |
| SNAP (BI 2021) | National Open API Payment Standard | All payment APIs |
| PCI DSS | Card data security | Gateway (card processing) |
| UU ITE | Electronic transaction law | All platforms |
| UU PDP (2024) | Personal data protection | All platforms |

---

## Common Engineering Patterns

### Consistent Across All Platforms

1. **Append-only ledger** — No mutation of financial records
2. **Double-entry accounting** — Debit + credit always balanced
3. **Idempotency for all writes** — Client-generated unique keys
4. **Structured error codes** — Typed errors, never raw panics
5. **Async event processing** — Kafka or NATS for non-critical paths
6. **Circuit breakers** — External API failures isolated from core
7. **Rate limiting** — Token bucket at gateway layer
8. **Go for new services** — All platforms adopting or already using Go
9. **Redis caching** — Idempotency keys, session, balance hot-read
10. **CI/CD with code review** — Automated gates before merge

### Differentiators

| Pattern | Where Used | Why Notable |
|---------|-----------|-------------|
| Feature flags | OVO | Essential for modular deployment at 100M users |
| Core/Use Case/Kit 3-layer | DANA | Clean separation that inspired our FCIS approach |
| Reversal API for timeouts | OVO | Fintech-specific: never leave partial state unresolved |
| HTTP 202 in-flight | GoPay | Better UX than blocking or returning error |
| SNAP signature compliance | ShopeePay, OVO, LinkAja | Government-mandated standard |
| Monolith-first, extract later | All started this way | Validates our approach |

---

## Open Source Footprint

### GoTo Financial (github.com/gopaytech) — 46 repos
- `patroni_exporter` (Go, 18 ★) — Prometheus PostgreSQL HA metrics
- `proctor` (JS) — Automation orchestrator
- `go-commons` (Go) — Shared Go libraries
- `istio-upgrade-consumer` (Go) — Auto Istio mesh upgrade
- `pergent` (Go, 2026) — Agentic PR review tool

### DANA Indonesia (github.com/dana-id) — 6 repos
- Official SDKs: PHP, Go, Node.js, Python, Java
- `uat-script` — End-to-end test scripts

### Midtrans (github.com/Midtrans)
- `midtrans-go` (135 ★, official) — Go SDK for payment API
- Core API, Snap UI, Iris disbursement clients

### Community Projects
- `imrenagi/go-payment` (401 ★) — Midtrans + Xendit proxy in Go
- `pandudpn/go-payment-gateway` — Unified SDK: Midtrans/Xendit/Doku

---

## The 2026 Shift: AI-Augmented Engineering

All major platforms investing in AI-assisted development:

- **DANA**: Remote coding agents drawing from centralized Notion knowledge base + design system patterns + codebase conventions. Design-to-code generation from Figma → production code. Seven automation modules for PR, test, deployment.

- **GoPay**: `pergent` — agentic PR review tool for CI/CD pipelines. RAG pipeline POC for infrastructure knowledge retrieval.

- **OVO**: AI-powered capacity planning (ARIMA/Prophet time series) for predictive autoscaling.

The trend: AI agents are being used for **generation** (DANA design-to-code), **review** (GoPay pergent), and **operations** (OVO predictive scaling).

---

## Key Takeaways for Our Spec

1. **Go + PostgreSQL** is the winning choice — all modern services use it
2. **30 min JWT** is industry standard, not 24h
3. **SNAP protocol** is mandatory knowledge for hiring at these companies
4. **In-flight idempotency** (HTTP 202) differentiates production-grade from toy
5. **Feature flags** are expected — OVO explicitly mentions this
6. **Prometheus** is universal — expvar alone signals inexperience
7. **Our FCIS approach** mirrors DANA's Core → Use Case → Integration Kit
8. **Monolith-first** is validated — every major player started this way

---

## Sources

- gopay.com/blog — GoPay engineering blog (Czech/English)
- engineering.grab.com — OVO/Grab engineering
- medium.com/dana-engineering — DANA product & tech
- shopee.sg/blog — Shopee insider (Go primary language)
- thejakartapost.com — GoPay.sh developer experience feature
- bi.go.id — SNAP standard regulation release
- github.com/gopaytech — 46 open source repos
- github.com/Midtrans/midtrans-go — Official Go SDK
- github.com/pandudpn/go-payment-gateway — Unified Indonesian payment SDK
- pingcap.com — Shopee TiDB case study
- oceanbase.com — DANA database case study
- Job postings from GoPay, OVO, DANA, ShopeePay, LinkAja (2024-2026)
- Partner integration docs: product.shopeepay.co.id, ovo.id, doc.gopay.com, docs.xendit.co
- pkg.go.dev/github.com/midtrans/midtrans-go — SDK documentation
- medium.com/dana-engineering/danakit-as-a-general-sdk-in-ios — 3-layer architecture
- hybrid.co.id — OVO scalable architecture interview
- notion.com/ja/customers/dana-indonesia — DANA AI/automation case study
