# GraphQL Coverage Backlog (Lift Routes)

> **Status**: Active  
> **Last updated**: 2025-12-25  
> **Source of truth**: `docs/specs/graphql_coverage.yaml`

This document is a curated backlog for driving `policy: graphql_required` routes from `status: missing` to `status: covered`.
The complete per-route inventory (including status + GraphQL mappings) lives in `docs/specs/graphql_coverage.yaml`.

## Snapshot
- Total Lift routes tracked: **233**
- `graphql_required`: **200**
  - `covered`: **56**
  - `missing`: **144**
- `rest_only`: **33**

## Where the gaps are (missing by route family)

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
- `/api/v1/admin/announcements`: 1

## Recommended workstream order

1. **Accounts + relationships** (`/api/v1/accounts/*`, `/api/v1/blocks`, `/api/v1/mutes`)
2. **Timelines + statuses + interactions** (`/api/v1/timelines/*`, `/api/v1/statuses/*`, `/api/v1/favourites`, `/api/v1/bookmarks`)
3. **Notifications + lists + scheduled statuses** (`/api/v1/notifications/*`, `/api/v1/lists/*`, `/api/v1/scheduled_statuses/*`)
4. **Discovery** (`/api/v1/trends/*`, `/api/v1/directory`, `/api/v1/suggestions*`, `/api/v2/search`, `/api/v2/suggestions`)
5. **Moderation + trust + reputation** (`/api/v1/moderation/*`, `/api/v1/reports`, `/api/v1/reputation/*`, `/api/v1/vouches/*`)
6. **Admin parity** (`/api/v1/admin/*`)

Each workstream should end with:
- Schema additions (if needed) + resolvers + service wiring
- `docs/specs/graphql_coverage.yaml` updates marking the affected routes `status: covered` with correct `graphql:` mappings
- `make verify-graphql-coverage`, `make test`, `make lint`

## Important note: CMS parity

CMS parity is in-scope but is not necessarily represented by the Lift route inventory (depending on implementation approach).
Track CMS parity as a feature backlog until it has a stable, routable surface, then include it in the coverage inventory process.
