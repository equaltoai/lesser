# API Surface Overview (REST + GraphQL)

> **Last updated**: 2025-12-25  
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
- Total Lift routes tracked: **233**
- `rest_only`: **33**
- `graphql_required`: **200**
  - `covered`: **56**
  - `missing`: **144**

Important: `missing` means **“not yet implemented or not yet mapped/verified”**. It is a backlog signal, not a definitive statement that no GraphQL capability exists.

### Missing clusters (by route family)

Top missing clusters (`policy: graphql_required`, `status: missing`):
- `/api/v1/admin/*`: 46
- `/api/v1/statuses/*`: 16
- `/api/v1/lists/*`: 8
- `/api/v1/moderation/*`: 8
- `/api/v1/accounts/*`: 7
- `/api/v1/timelines/*`: 6
- `/api/v1/instance/*`: 5
- `/api/v1/notifications/*`: 4
- `/api/v1/push/*`: 4
- `/api/v1/reputation/*`: 4
- `/api/v1/scheduled_statuses/*`: 4
- `/api/v1/trends/*`: 4

Admin missing sub-clusters:
- `/api/v1/admin/accounts/*`: 10
- `/api/v1/admin/moderation/*`: 7
- `/api/v1/admin/reports/*`: 6
- `/api/v1/admin/domain_blocks/*`: 5
- `/api/v1/admin/statuses/*`: 5
- `/api/v1/admin/{custom_emojis|domain_allows|email_domain_blocks|federation}/*`: 3 each

## How to keep this accurate

- When Lift routes change: run `make generate-graphql-coverage`.
- CI/verification: `make verify-graphql-coverage` fails on route drift or invalid GraphQL mappings.
- Use `docs/specs/GRAPHQL_COVERAGE_PLAN.md` and `docs/specs/GRAPHQL_COVERAGE_BACKLOG.md` to drive the remaining `missing` routes to `covered`.

