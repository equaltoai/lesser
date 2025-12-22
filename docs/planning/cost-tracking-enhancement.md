# Cost Tracking Enhancement (Design + Requirements)

Status: draft  
Last updated: 2025-12-22  
Related: `docs/planning/cost-tracking-findings.md`

## Intent

Cost tracking in Lesser is not just “billing visibility”. The core intent is **per-individual usage attribution** that enables **governance**:

- **Throttle** expensive behavior in ways that keep small communities stable and affordable.
- **Incentivize** behavior (and potentially contributions) by granting usage credits, higher tiers, or priority access.
- Keep the platform “serverless-first”: instances can cost pennies per user, while still controlling the real spend drivers (security, observability, egress, and long-lived sessions).

This should work with Lesser’s authentication principles (no email/password; identity comes from passkeys/crypto wallets) and must preserve the required compatibility layers (ActivityPub first, then Mastodon API where AWS allows, then Lesser-native enhancements).

## Goals

- Attribute spend and usage to an **actor** (user) and **tenant** (instance), with enough detail to explain “why” and to enforce policy.
- Support budgets and throttles at multiple levels:
  - per-user, per-tenant, per-feature, and per-federation-domain
  - by both **currency** (microcents) and **limits** (requests/min, session-minutes, bytes)
- Make the system operable:
  - low overhead in hot paths
  - good dashboards/alerts
  - deterministic, explainable decisions (“why was I throttled?”)
- Maintain compatibility expectations:
  - avoid breaking ActivityPub federation (prefer backpressure + queuing over hard rejection)
  - preserve Mastodon client expectations as much as possible (standard 429 behavior, Retry-After, etc.)

## Non-goals (for the pilot)

- Perfect alignment to AWS invoices at the per-request level.
- Full cost accounting for every AWS product on day one.
- A new billing product; this is governance and sustainability first.

## Current State (summary)

Lesser already has multiple cost-tracking systems and several domain-specific ledgers/budgets (search, AI, federation, websocket, import/export, notifications, scheduled jobs). It also has both:

- **Signals**: request-scoped counters and CloudWatch metric-oriented tracking (`pkg/cost`).
- **Durable ledgers**: multiple DynamORM-backed models in `pkg/storage/models/*cost*` and a stream-driven aggregation pipeline (`cmd/cost-aggregator`).

There is also a separate CostHistory path (`pkg/cost/middleware.go` + `pkg/cost/storage.go`) using the native DynamoDB SDK which appears “parallel” to the durable-ledger approach.

The biggest practical gaps for trials are:

- consistent **unit/currency semantics** (microcents vs dollars/cents conversions)
- explicit costs for **API Gateway (REST + streaming), WebSocket connection-minutes, and SSE duration/bytes**
- explicit costs for **CloudWatch logs, X-Ray, WAF, NAT, and egress** (often the true spend drivers)

### Existing durable ledgers/budgets worth building on (already implemented)

- Search: `models.SearchCostTracking` + `models.SearchBudget` (budget enforcement exists via `SearchCostRepository.CheckBudget(...)`).
- WebSocket: `models.WebSocketCostRecord` + `models.WebSocketCostBudget` (budget evaluation exists via `WebSocketCostRepository.CheckBudgetLimits(...)`).
- AI: `models.AICost` (token- and model-aware cost tracking).
- Federation: `models.FederationCostTracking` (domain-scoped, retry-aware costs) and `pkg/federation/cost` (budgeting/tiering framework).
- Import/Export: `models.ImportCostTracking`, `models.ExportCostTracking`, `models.ImportBudget`.
- Notifications: `models.NotificationCostTracking` + `models.NotificationCostAggregation`.
- Scheduled/background jobs: `models.ScheduledJobCostRecord` + `models.ScheduledJobCostAggregation`.

## Design Principles

1. **Ledger-first for governance**
   - Durable per-actor/per-tenant records are the foundation for throttles and incentives.
   - Metrics are necessary for dashboards/alerts, but not sufficient for user-level governance.
2. **Single “cost envelope” across subsystems**
   - Specialized models can remain, but they must map to a shared attribution vocabulary.
3. **Explainability**
   - Every budget decision should be explainable as “you exceeded budget X due to Y in window Z”.
4. **Compatibility-safe backpressure**
   - Prefer queuing and rate shaping over hard rejections for federation.
5. **Privacy-aware by default**
   - Store minimal PII. Use IDs, not emails; consider hashing where appropriate; apply TTLs aggressively.

## Canonical Cost Event (Envelope)

Define a common envelope used by all instrumentation paths, regardless of where data is persisted:

- `tenant_id` / `instance_id` (required)
- `actor_id` / `user_id` (optional, but required for user governance)
- `feature` (required): e.g., `rest:v1:timeline`, `graphql:Query.homeTimeline`, `federation:deliver`, `ws:stream:public`
- `resource` (required): e.g., `dynamodb`, `lambda`, `apigw_rest`, `apigw_ws`, `s3`, `cloudwatch_logs`, `xray`, `waf`, `egress_nat`
- `units` (optional but preferred): RCU/WCU, ms, bytes, messages, connection-minutes, etc.
- `cost_microcents` (required once calibrated; allowed to be estimated in early phases)
- `timestamp` + `window` (minute/hour/day)
- `correlation`: request ID, job ID, connection ID, federation domain, route name, etc.
- `tags/properties`: maps for subsystem-specific detail

**Storage direction:** Prefer using the existing durable record model (`models.DynamoDBCostRecord`) as the canonical persisted “envelope” via `Tags`/`Properties`, with specialized models continuing to exist where they provide domain value (websocket, federation, AI, notifications), but required to emit compatible envelope fields.

### Attribution vocabulary (recommended reserved keys)

To keep subsystems interoperable, reserve a small set of stable tag/property keys:

- `tenant_id` (or `instance_id`)
- `actor_id` (or `user_id`)
- `feature` (string identifier)
- `resource` (string identifier)
- `remote_domain` (for federation)
- `connection_id` (for websocket/SSE)
- `job_id` / `workflow_id` (for background processing)
- `route` (coarse route identifier; avoid full URLs)

## Units & Currency Requirements (must fix before “budgeting”)

Before budgets become “real”:

- Choose and document **one monetary base unit** (microcents vs microdollars) and provide conversion helpers.
- Audit all conversions that currently derive dollars/cents from stored integers.
- Ensure every persisted field named `*MicroCents` is actually the same unit everywhere.
- Make it explicit which numbers are “estimates” vs “accounting-grade”.

## Budget & Governance Model

### Budget scopes

- **Per-user**: primary for throttling/incentives.
- **Per-tenant**: caps runaway instance spend; used for “pilot guardrails”.
- **Per-feature**: protect expensive surfaces (search, AI, media processing, long-lived sessions).
- **Per-domain** (federation): prevent a single remote instance from dominating spend.

### Budget types

- **Spend budgets**: microcents per window (hour/day/month).
- **Rate budgets**: requests/min, messages/min, operations/min.
- **Session budgets**: websocket connection-minutes, SSE minutes, max concurrent sessions.
- **Burst/credit budgets**: allow short spikes via “credits” that refill or can be granted.

### Enforcement behaviors (compatibility-aware)

- **REST/Mastodon clients**: use `429` with `Retry-After` when throttling; prefer feature-specific messages; degrade limits instead of hard failure where possible.
- **GraphQL**: return structured errors with budget identifiers; allow partial data if safe.
- **ActivityPub inbound**: avoid rejecting valid deliveries due to budget; prefer enqueue with slower processing, or respond `202` and process later (where semantically safe).
- **ActivityPub outbound**: slow down delivery and retries (domain-scoped) rather than dropping; tier domains if needed.
- **WebSocket/SSE**: enforce via connection admission (connect) and message/stream rate shaping; close with an explicit reason message when exceeding hard limits.

### Incentives

Incentives should be built on the same primitives as throttling:

- grant microcent credits (per period), burst credits, or feature-specific credits
- tier upgrades (higher limits) based on community role, contributions, or reputation
- transparent “cost per action” visibility to help users self-regulate

## Instrumentation Coverage Targets (trial-driven)

### P0 must-have cost drivers

- API Gateway:
  - REST request counts + payload sizes (bytes in/out)
  - SSE/response streaming: connection duration + bytes out
  - WebSocket: connection-minutes + message counts + bytes
- Lambda: duration, memory (already tracked in multiple places; normalize)
- DynamoDB: consumed capacity (prefer real consumed capacity where possible; fallback to estimates)
- Federation: outbound delivery and retries by remote domain (bytes + requests)
- Observability:
  - CloudWatch log ingestion volume (bytes) and retention policy choice
  - X-Ray traces count (sampling rate), if enabled
- Security:
  - WAF request counts/rule evaluations (or allocate as shared overhead)
- Egress/NAT:
  - bytes out (and whether NAT is involved), especially for federation/media

### Shared overhead allocation (calibration requirement)

Some costs cannot be perfectly attributed per request (WAF, CloudWatch, NAT fixed components). The system needs:

- a periodic reconciliation job that:
  - pulls “total spend” (Cost Explorer or CUR/ATHENA in later phases)
  - computes overhead pools (WAF, logs, NAT, etc.)
  - allocates overhead proportionally using durable usage signals (requests, bytes, minutes, messages)
- a clear label on allocated costs as **allocated** (not measured).

## Data Retention, Privacy, and Safety

- Apply TTLs:
  - keep raw high-cardinality records short (days to weeks)
  - keep rollups longer (months) for trend analysis
- Avoid storing request bodies or full URLs; store route identifiers + hashed/query-length style signals where needed.
- Store actor identity as internal IDs (passkey/wallet-resolved account IDs), not emails.
- Ensure throttling decisions can be explained without revealing sensitive details about other users’ budgets.

## Operator & User Experience Requirements

- Admin surfaces:
  - set budgets by tier/user/feature/domain
  - view current burn rate and top drivers
  - configure enforcement mode (observe-only vs enforce)
- User surfaces:
  - view personal usage/budgets and “why throttled”
  - view “cost per action” guidance for self-regulation

## Phased Delivery (recommended)

### P0 — Make governance possible (pilot foundation)

- Decide canonical ledger + envelope fields; document the vocabulary.
- Normalize units and conversions (microcents semantics).
- Ensure every request/job that can be tied to a user/tenant emits those tags.
- Instrument the P0 drivers above (especially APIGW, logs, egress).
- Add “observe-only” mode everywhere enforcement exists; enable “enforce” selectively per feature.

### P1 — Generalize budgets and enforcement

- Consolidate budget evaluation into a single service (shared library) used by REST/GraphQL/websocket/federation.
- Implement consistent error/response semantics per surface.
- Add automated alerts + anomaly detection hooks that trigger budget changes or admin notifications.

### P2 — Calibration + incentives

- Reconcile estimates against Cost Explorer/CUR in a controlled trial.
- Implement overhead allocation and label it clearly.
- Add incentive mechanisms (credits/tiers) and document recommended governance defaults for small communities.
