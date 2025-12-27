# API Surface Overview (REST + GraphQL)

> **Last updated**: 2025-12-26  
> **Source of truth**: `docs/specs/graphql_coverage.yaml` (enforced by `make verify-graphql-coverage`)

This document summarizes Lesser’s current API surface in two dimensions:
- **REST surface**: what endpoints exist and are reachable (no accidental 404s due to missing routing).
- **GraphQL parity**: which product endpoints are available via GraphQL for a Greater-based client/admin UI (excluding explicitly REST-only flows).

## Scope

- Inventory includes **all Lift-configured routes** (excluding `HEAD`/`OPTIONS`) extracted from:
  - `cmd/api/routes_lift.go`
  - `cmd/api/main.go`
- “GraphQL parity” is tracked per-route in `docs/specs/graphql_coverage.yaml`:
  - `policy: rest_only` routes are explicitly exempt (see `exemptions:`).
  - `policy: graphql_required` routes must have a GraphQL equivalent.

Out of scope for GraphQL parity (still tracked as `rest_only`):
- OAuth/OIDC flows (`/oauth/*`, app registration)
- Wallet auth + WebAuthn + setup/bootstrap flows
- Protocol/infra endpoints (`/.well-known/*`, `/nodeinfo/*`, ActivityPub collections, `/health/*`)
- UI routing / embed HTML endpoints (`/`, `/embed/*`, `/api/oembed`)

## REST Surface Parity (Lift)

**Goal**: if a Lift handler exists for a route, it is registered so it doesn’t 404.

**Status**: complete for all documented Lift handlers (no unrouted `// ... handles METHOD /path` endpoints remain).

Most-recent parity additions (previously had handlers but weren’t routed):
- Mutes: `POST /api/v1/accounts/{id}/mute`, `POST /api/v1/accounts/{id}/unmute`, `GET /api/v1/mutes`
- Trends v1: `GET /api/v1/trends`, `GET /api/v1/trends/{tags|statuses|links}`
- Translation: `GET /api/v1/instance/translation_languages`
- Admin: federation, email domain blocks, custom emojis, announcements

## GraphQL Coverage Snapshot

Counts are derived from `docs/specs/graphql_coverage.yaml`:
- Total Lift routes tracked: **232**
- `rest_only`: **32**
- `graphql_required`: **200**
  - `covered`: **200**
  - `missing`: **0**

Status note: `make verify-graphql-coverage` runs with `--strict`, so any `graphql_required` route that is not mapped to a schema field fails CI/verification.

## How to keep this accurate

- When Lift routes change: run `make generate-graphql-coverage`.
- CI/verification: `make verify-graphql-coverage` fails on route drift or invalid GraphQL mappings.
- Use `docs/specs/GRAPHQL_COVERAGE_PLAN.md` and `docs/specs/GRAPHQL_COVERAGE_BACKLOG.md` when introducing new route families or expanding parity requirements.
