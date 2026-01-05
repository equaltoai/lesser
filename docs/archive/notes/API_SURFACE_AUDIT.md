# API Surface Audit (REST + GraphQL)

> **Last updated**: 2025-12-26  
> **Scope**: REST (Lift + other HTTP Lambdas) and GraphQL (schema + resolvers)  
> **Sources of truth**: `docs/specs/openapi.yaml`, `docs/specs/graphql_coverage.yaml`, `graph/*.graphql`, `cmd/*` route registrations

This audit focuses on **gaps**: missing routing, missing documentation, and explicit “not supported”/fallback implementations that indicate incomplete functionality.

## Executive summary

- **OpenAPI**: `make verify-openapi-strict` passes (`docs/specs/openapi.yaml` has 215 paths).
- **GraphQL parity mapping**: `make verify-graphql-coverage` passes (`docs/specs/graphql_coverage.yaml` tracks 232 routes).
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
  - WebFinger: `cmd/webfinger/main.go` (`/.well-known/webfinger`)

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

### Resolved (2025-12-26)

- **REST account resolution supports remote actor URLs/handles**
  - `cmd/api/lift/helpers.go` now resolves remote actor URLs via `pkg/federation/RemoteSearchService` and supports `@user@domain` handles.
- **Moderation image analysis supports non-S3 URLs**
  - `pkg/moderation/advanced/image_analyzer.go` now downloads bytes via `pkg/httpclient` and uses Rekognition `Bytes` input.
- **CMS series slug lookup no longer relies on global scans**
  - `pkg/storage/models/cms_series_slug_index.go` introduces a slug index.
  - `graph/query_resolvers_cms.go` uses the index and backfills it for legacy rows.
- **CMS publication membership scan fallback is self-healing**
  - `graph/query_resolvers_cms.go` backfills missing GSI keys on legacy `PublicationMember` rows so future lookups use the index.
- **Slug updates no longer appear as supported mutations**
  - `graph/phase1.graphql` removes `slug` from `UpdateCategoryInput` and `UpdatePublicationInput`.
- **Duplicate HTTP route implementations removed**
  - `cmd/api/routes_lift.go` no longer registers `/users/{username}/{followers|following}` (owned by `cmd/collections`).
  - `cmd/webfinger/main.go` only registers `/.well-known/webfinger` (NodeInfo + reputation keys handled by API Lambda routing).

### P3 — Documentation gaps

- `docs/specs/openapi.yaml` intentionally covers the API Lambda (Mastodon REST + auth/setup), but it does **not** describe:
  - GraphQL endpoints (`cmd/graphql/main.go:705`)
  - SSE streaming (`cmd/sse/main.go:111`)
  - Protocol endpoints served by non-API Lambdas (`cmd/*/main.go`)

If we want “one place to discover everything”, we should either:
1) extend OpenAPI to include these additional HTTP entrypoints, or
2) publish a second spec (or a short registry doc) for the non-API Lambdas.

## Recommended next steps

1. Decide whether slug changes are required (categories/publications) and, if so, define an ID/redirect strategy.
