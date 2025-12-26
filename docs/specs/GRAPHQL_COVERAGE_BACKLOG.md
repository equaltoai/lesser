# GraphQL Coverage Backlog (Lift Routes)

> **Status**: Active  
> **Last updated**: 2025-12-26  
> **Source of truth**: `docs/specs/graphql_coverage.yaml`

This document is a curated backlog for driving `policy: graphql_required` routes from `status: missing` to `status: covered`.
The complete per-route inventory (including status + GraphQL mappings) lives in `docs/specs/graphql_coverage.yaml`.

## Snapshot
- Total Lift routes tracked: **234**
- `graphql_required`: **200**
  - `covered`: **126**
  - `missing`: **74**
- `rest_only`: **34**

## Where the gaps are (missing by route family)

Top missing clusters (`policy: graphql_required`, `status: missing`):
- `/api/v1/admin/*`: 43
- `/api/v1/statuses/*`: 7
- `/api/v1/instance/*`: 5
- `/api/v1/announcements/*`: 4
- `/api/v1/moderation/*`: 4
- `/api/v1/reputation/*`: 4
- `/api/v1/vouches/*`: 3
- `/api/v1/notifications/*`: 1
- `/api/v1/reports/*`: 1
- `/api/v1/timelines/*`: 1
- `/api/v2/instance/*`: 1

Admin missing sub-clusters:
- `/api/v1/admin/accounts/*`: 10
- `/api/v1/admin/moderation/*`: 7
- `/api/v1/admin/reports/*`: 6
- `/api/v1/admin/statuses/*`: 5
- `/api/v1/admin/domain_blocks/*`: 5
- `/api/v1/admin/{domain_allows|email_domain_blocks}/*`: 3 each
- `/api/v1/admin/federation/*`: 3
- `/api/v1/admin/announcements/*`: 1

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

This is the current `status: missing` remainder after mapping all obvious parity to existing schema operations.
Use this list as the source for Phase 1–3 backlog slicing.

- **Timelines**
  - `GET /api/v1/timelines/link` (statuses by link URL)
- **Statuses**
  - `PUT /api/v1/statuses/{id}` (edit)
  - `GET /api/v1/statuses/{id}/{favourited_by|reblogged_by}` (engagement actors)
  - `GET /api/v1/statuses/{id}/history`
  - `POST /api/v1/statuses/{id}/{mute|unmute}` (thread mute)
  - `POST /api/v1/statuses/{id}/translate`
- **Notifications**
  - `GET /api/v1/notifications/{id}`
- **Instance + announcements**
  - `GET /api/v1/instance` + `GET /api/v2/instance`
  - `GET /api/v1/instance/{activity|peers|domain_blocks|translation_languages}`
  - `GET /api/v1/announcements` + reactions + dismiss
- **Moderation**
  - `POST /api/v1/moderation/review`
  - `GET /api/v1/moderation/{history/{object_id}|consensus/{event_id}}`
  - `GET /api/v1/moderation/trust/{actor_id}/score`
- **Reports**
  - `POST /api/v1/reports`
- **Reputation + vouches**
  - `GET /api/v1/reputation/{actor_id}` + import/export/verify
  - `POST /api/v1/vouches` + list/revoke
- **Admin**
  - All `status: missing` routes under `/api/v1/admin/*` (see counts above)
