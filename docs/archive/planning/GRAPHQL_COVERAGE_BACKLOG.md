# GraphQL Coverage Backlog (Lift Routes)

> **Status**: Active  
> **Last updated**: 2025-12-26  
> **Source of truth**: `docs/specs/graphql_coverage.yaml`

This document is a curated backlog for driving `policy: graphql_required` routes from `status: missing` to `status: covered`.
The complete per-route inventory (including status + GraphQL mappings) lives in `docs/specs/graphql_coverage.yaml`.

## Snapshot
- Total Lift routes tracked: **234**
- `graphql_required`: **200**
  - `covered`: **200**
  - `missing`: **0**
- `rest_only`: **34**

## Where the gaps are (missing by route family)

None (`policy: graphql_required`, `status: missing` is **0**).

## Recommended workstream order

1. **Accounts + relationships** (`/api/v1/accounts/*`, `/api/v1/blocks`, `/api/v1/mutes`, `/api/v1/follow_requests*`, `/api/v1/domain_blocks*`)
2. **Statuses + interactions** (`/api/v1/statuses/*`, `/api/v1/favourites`, `/api/v1/bookmarks`)
3. **Instance + announcements** (`/api/v1/instance*`, `/api/v2/instance`, `/api/v1/announcements*`)
4. **Notifications + markers** (`/api/v1/notifications/{id}`, `/api/v1/markers*`)
5. **Moderation + trust + reputation** (`/api/v1/moderation/*`, `/api/v1/reports`, `/api/v1/reputation/*`, `/api/v1/vouches/*`)
6. **Admin parity** (`/api/v1/admin/*`)

Each workstream should end with:
- Schema additions (if needed) + resolvers + service wiring
- `docs/specs/graphql_coverage.yaml` updates marking the affected routes `status: covered` with correct `graphql:` mappings
- `make verify-graphql-coverage`, `make test`, `make lint`

## Important note: CMS parity

CMS parity is in-scope but is not necessarily represented by the Lift route inventory (depending on implementation approach).
Track CMS parity as a feature backlog until it has a stable, routable surface, then include it in the coverage inventory process.

## Remaining missing routes (Phase 0.5 audit result)

None.
