# GraphQL Validation Gaps

**Note:** This file tracks known gaps in the GraphQL validation process. Use the `make seed-and-validate` target to run the automated seeding and validation process. Any new failures should be logged in this file.

Latest validation sweep: 2025-10-29 (dev.lesser.host) using `tests/system` helpers and ad-hoc queries with the seeded admin persona.

## High-Level Findings
- Core account resolver (`actor`) returns empty payloads and fails hard when requesting required fields (`createdAt`), despite complete items existing in `lesser-development`.
- Public timeline and object lookups are unable to hydrate seeded statuses (`Failed to retrieve timeline/status`), leaving downstream discovery/search flows empty.
- Several relationship and moderation/federation queries surface backend failures (`internal system error`, `failed to list relationships`) instead of graceful empty collections.
- Auxiliary analytics endpoints (`instanceMetrics`, `aiCapabilities`, `costBreakdown`) respond successfully, indicating the GraphQL gateway itself is reachable and authenticated correctly.

## Detailed Issues

| Query | Request | Error / Result | Notes |
| --- | --- | --- | --- |
| `actor(username:"admin")` | `id username displayName createdAt` | `the requested element is null which the schema does not allow` | Removing `createdAt` yields empty strings for `id`/`username`; Dynamo `ACTOR#admin` item includes `CreatedAt` and full ActivityPub document. Suggest inspecting `actor_repository` mapping / default projections. |
| `timeline(type: PUBLIC, first:5)` | Default selection (id/content/visibility) | `failed to get timeline\nFailed to retrieve timeline` | Home timeline returns empty array even with seeded status `e7aba65c-…`. Dynamo item carries `GSI2PK=PUBLIC_TIMELINE`, so check index/table hydration. |
| `object(id:"https://dev.lesser.host/users/admin/statuses/e7aba65c-a4c9-4b5a-bba7-07157d7030f7")` | `id type createdAt content` | `failed to get object\nFailed to retrieve status` | Confirms status row is invisible to GraphQL fetch path. |
| `conversations(first:3)` | `id unread` | `internal system error` | Needs repository guard for empty dataset; today it throws. |
| `lists` | `id title` | `failed to get lists\nfailed to get user lists` | Persona has no lists yet; expect empty array instead of failure. |
| `followers(username:"admin", limit:5)` | `actors { username } totalCount` | `failed to list relationships\nFailed to query follow (followers)` | Table currently lacks follower edges, but query should degrade gracefully. |
| `federationStatus(domain:"mastodon.social")` | `domain reachable` | `internal system error` | Federation probe path may require bootstrap config (DNS?)—needs defensive error handling. |
| `moderationQueue(first:3)` | `id decision` | `failed to retrieve moderation queue\nFailed to query moderation event (queue paginated)` | Appears to assume Dynamo pagination artifacts that do not exist yet. |

## Environment & Data Notes
- DynamoDB scan (2025-10-29) confirms bootstrap personas, OAuth clients, and at least one public status present. GraphQL resolvers are not surfacing this data.
- No lists/media/push subscriptions currently exist; queries expecting them should return empty payloads rather than errors.
- `instanceMetrics`, `costBreakdown`, `aiCapabilities`, `following`, `profileDirectory`, `suggestions`, and `userPreferences` all returned successfully (mostly empty), validating auth/token handling.

## Recommended Next Steps
1. Debug `actor` resolver path to ensure non-null fields (`createdAt`, `id`, `username`) hydrate from stored actor document.
2. Trace status fetch pipeline (`timeline`, `object`) to confirm `PUBLIC_TIMELINE` GSI usage and status repository projections after the recent reseed.
3. Add defensive guards to relationship/moderation/federation resolvers so empty tables do not bubble internal exceptions to the GraphQL layer.
4. Once data access is stable, re-run `tests/system/test_graphql_reads.py` end-to-end and extend coverage to mutations/subscriptions.
