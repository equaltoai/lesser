# API Surface Audit (REST + GraphQL)

> **Last updated**: 2025-12-26  
> **Scope**: REST (Lift + other HTTP Lambdas) and GraphQL (schema + resolvers)  
> **Sources of truth**: `docs/specs/openapi.yaml`, `docs/specs/graphql_coverage.yaml`, `graph/*.graphql`, `cmd/*` route registrations

This audit focuses on **gaps**: missing routing, missing documentation, and explicit “not supported”/fallback implementations that indicate incomplete functionality.

## Executive summary

- **OpenAPI**: `make verify-openapi-strict` passes (`docs/specs/openapi.yaml` has 215 paths, 255 operations).
- **GraphQL parity mapping**: `make verify-graphql-coverage` passes (`docs/specs/graphql_coverage.yaml` tracks 234 routes; 34 are `rest_only`, 200 are `graphql_required` and all are `covered`).
- **Lift handler routing**: all Lift handler “handles METHOD /path” declarations are registered; 0 unrouted handler endpoints detected.
- **Non-OpenAPI HTTP entrypoints exist** (GraphQL, SSE streaming, ActivityPub/protocol Lambdas); they are reachable but intentionally outside the single OpenAPI file.

## Inventory

### REST: API Lambda (`cmd/api`)

**What it covers**
- Mastodon REST (`/api/v1/*`, `/api/v2/*`)
- OAuth (registration, authorize, token)
- Wallet auth + WebAuthn + setup flows (explicitly REST-only)
- Some protocol-ish endpoints are also served here (e.g., NodeInfo) depending on deployment routing

**Docs + route inventory**
- OpenAPI: `docs/specs/openapi.yaml`
- GraphQL parity mapping (per REST route): `docs/specs/graphql_coverage.yaml`

**Routing completeness**
- Handler declarations (195) vs registered routes: no missing registrations detected (registration parity is complete).

### REST: Other HTTP Lambdas (not covered by `docs/specs/openapi.yaml`)

These routes exist in code and are deployed/routed by infra, but they are not currently described in the single OpenAPI file:
- GraphQL HTTP: `cmd/graphql/main.go` (`/graphql`, `/api/graphql`, `/playground`, `/subscriptions`, health/ready).
- Streaming SSE: `cmd/sse/main.go` (`/api/v1/streaming/*`).
- ActivityPub/protocol routes:
  - Actor: `cmd/actor/main.go` (`/users/:username`)
  - Inbox: `cmd/inbox/main.go` (`/users/:username/inbox`)
  - Outbox: `cmd/outbox/main.go` (`/users/:username/outbox`)
  - Object fetch: `cmd/objects/main.go` (`/objects/:id`)
  - Collections: `cmd/collections/main.go` (`/users/:username/{followers|following|liked}`)
  - WebFinger/NodeInfo: `cmd/webfinger/main.go` (`/.well-known/*`, `/nodeinfo/*`)

### GraphQL

**Schema sources**
- `graph/core.graphql`
- `graph/phase1.graphql`
- `graph/phase2.graphql`
- `graph/phase3.graphql`

**Root operation counts (schema)**
- Queries: 135
- Mutations: 144
- Subscriptions: 21

**REST ↔ GraphQL mapping**
- Unique GraphQL ops referenced by REST parity mapping: 167
- GraphQL-only ops (not mapped from a REST route): 133 (includes all subscriptions and CMS-specific operations)

## Gaps and “missing implementation” findings

### P0 — Functional gaps

- **Remote account IDs (actor URLs) are not supported in REST account resolution**
  - `cmd/api/lift/helpers.go:42` rejects remote actor URLs (`remoteAccountsNotSupported`), which blocks REST endpoints that accept account IDs in URL form for non-local users.
  - This is a hard functional gap for “remote user” workflows in REST.

- **Moderation image analysis requires S3 URLs**
  - `pkg/moderation/advanced/image_analyzer.go:86` returns an error for non-S3 URLs (“non-S3 URLs not yet supported”).
  - If clients submit remote/media URLs that are not already in S3, analysis cannot run without a prefetch/upload step.

### P1 — Scale / correctness gaps (works, but with fallback behaviors)

- **CMS series lookup by slug can fall back to a scan**
  - `graph/query_resolvers_cms.go:386` falls back to scanning series across all authors (`Limit(1000)`) when viewer context doesn’t find a match.
  - If we want “no scans” at scale, this needs a slug index (or require `(authorID, slug)` in the API).

- **CMS publication membership can fall back to a scan**
  - `graph/query_resolvers_cms.go:648` scans `PublicationMember` items as a back-compat fallback when the membership index query returns none.
  - This should be removable after any legacy membership state is migrated or once we guarantee the paginated membership query is canonical.

### P2 — Explicitly unsupported mutations (API works but cannot do the action)

- **Category slug updates are not supported**
  - `graph/mutation_resolvers_cms.go:829` returns an error if `slug` changes.
- **Publication slug updates are not supported**
  - `graph/mutation_resolvers_cms.go:1089` returns an error if `slug` changes.

If slug changes are required, we need an ID/redirect strategy and updates to deterministic ID invariants.

### P2 — Deployment/infra footguns (duplicate implementations)

The same method+path is implemented in multiple Lambdas. The deployed router must pick one (or you’ll get ambiguous behavior):
- NodeInfo + reputation keys exist in both API and WebFinger lambdas:
  - `cmd/api/routes_lift.go:94`
  - `cmd/webfinger/main.go:62`
- Followers/following collections exist in both API and Collections lambdas:
  - `cmd/api/routes_lift.go:523`
  - `cmd/collections/main.go:83`

This isn’t inherently wrong, but it increases the chance of drift unless the infra contract is explicit.

### P3 — Documentation gaps

- `docs/specs/openapi.yaml` intentionally covers the API Lambda (Mastodon REST + auth/setup), but it does **not** describe:
  - GraphQL endpoints (`cmd/graphql/main.go:705`)
  - SSE streaming (`cmd/sse/main.go:111`)
  - Protocol endpoints served by non-API Lambdas (`cmd/*/main.go`)

If we want “one place to discover everything”, we should either:
1) extend OpenAPI to include these additional HTTP entrypoints, or
2) publish a second spec (or a short registry doc) for the non-API Lambdas.

## Recommended next steps

1. Decide whether REST should support remote account actor URLs (and where): fix `resolveAccountID` to handle remote actors or explicitly scope REST operations to local-only.
2. Remove CMS scan fallbacks by introducing the required indexes or tightening query requirements (e.g., require author for slug lookups).
3. Decide slug-change policy for CMS categories/publications (if needed) and implement a safe migration/redirect strategy.
4. Document (or consolidate) duplicated HTTP routes across Lambdas to reduce infra drift risk.

