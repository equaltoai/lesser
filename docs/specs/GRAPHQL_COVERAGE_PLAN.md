# GraphQL Coverage: 4‑Phase Implementation Plan

> **Status**: Active  
> **Owner**: Lesser  
> **Last updated**: 2025-12-26  
> **Related**: `docs/specs/GRAPHQL_COVERAGE.md`, `docs/specs/graphql_coverage.yaml`, `docs/specs/GRAPHQL_COVERAGE_BACKLOG.md`

This document is the concrete, phase-based plan for achieving **GraphQL parity** for all in-scope Lesser product functionality while keeping REST as the source of truth for standards/protocol endpoints and other explicitly REST-only flows.

## Reality check (current backlog size)

The route inventory is tracked in `docs/specs/graphql_coverage.yaml` and is enforced by `make verify-graphql-coverage`.

Current counts (from that file):
- Total Lift routes tracked: **234**
- `graphql_required`: **200**
- `rest_only` (explicit exemptions): **34**
- Marked `covered`: **200**
- Marked `missing`: **0**

Important: With Phase **0.5** complete, **`missing` represents real parity work** (schema/resolvers/service), not “unmapped”. The current inventory is at **0 missing**.

## Definitions

- **GraphQL parity**: A feature is considered “GraphQL-covered” when a Greater-based client can implement the same user-facing capability without using REST endpoints (except explicit exemptions).
- **REST inventory**: `docs/specs/graphql_coverage.yaml` is the canonical list of all configured Lift routes (excluding `OPTIONS`/`HEAD`), with each route classified as:
  - `policy: graphql_required` (must have GraphQL equivalent), or
  - `policy: rest_only` (explicitly exempt; `exemptedBy` required).
- **Exemptions** (REST-only; not GraphQL-covered):
  - OAuth/OIDC flows
  - Wallet auth + WebAuthn + setup/bootstrap flows
  - Protocol/infra endpoints (`/.well-known/*`, `/nodeinfo/*`, ActivityPub, `/health*`)

## Phase 0 — Contract + Drift Guardrail (Completed)

**Goal**: Lock down what “100% GraphQL coverage” means and prevent silent drift between Lift routes and the coverage inventory.

**Deliverables**
- `docs/specs/GRAPHQL_COVERAGE.md` defines scope + exemptions + how we measure coverage.
- `docs/specs/graphql_coverage.yaml` is generated and kept in sync with Lift route configuration; includes REST-only routes too.
- `tools/graphql_coverage/` verifier:
  - Ensures every Lift route exists in YAML (no silent additions/removals)
  - Ensures REST-only routes match exemptions (`exemptedBy`)
  - Validates declared GraphQL mappings exist in current schema (`gqlgen.yml` sources)
- Make targets:
  - `make generate-graphql-coverage`
  - `make verify-graphql-coverage`

**Exit criteria**
- `make verify-graphql-coverage` passes in CI.
- Coverage YAML contains *all* Lift routes (excluding `OPTIONS`/`HEAD`) with correct policy classification.

## Phase 0.5 — Coverage Audit + Mapping (Completed)

**Goal**: Convert “unknown/unmapped” into a real backlog by marking routes as `covered` **only when** a working GraphQL operation exists (and is appropriate for the exemption policy).

**Why this is required**
- The current `missing` count includes many routes that may already be GraphQL-covered but not yet recorded in the inventory.
- Without this audit, “% complete” is meaningless and planning is guessy.

**Deliverables**
- Update `docs/specs/graphql_coverage.yaml` for each `graphql_required` route:
  - If GraphQL parity exists: set `status: covered` and add `graphql: [Query.<field>|Mutation.<field>]` mappings.
  - If GraphQL parity does not exist: keep `status: missing` and add a note in the backlog doc.
- Add/refresh `docs/specs/GRAPHQL_COVERAGE_BACKLOG.md` with:
  - Remaining missing routes grouped by domain area (accounts/statuses/admin/etc)
  - Proposed GraphQL operation names for each missing area (or “new schema required”)

**Exit criteria**
- All `graphql_required` routes are either:
  - `covered` with valid `graphql:` mappings, or
  - `missing` with a clear implementation owner (schema vs resolver vs service) and a target phase.

## Phase 1 — CMS GraphQL Parity (Completed)

**Goal**: Expose CMS functionality via GraphQL to support Greater-based content/admin experiences.

**Scope**
- Articles/drafts/revisions/publications
- Series/categories/tags
- Author/editor workflows needed by the intended Greater client

**Deliverables**
- CMS schema additions in `graph/phase1.graphql`, wired into `gqlgen.yml`.
- CMS resolvers and conversion/helpers in:
  - `graph/cms_helpers.go`
  - `graph/cms_converters.go`
  - `graph/query_resolvers_cms.go`
  - `graph/mutation_resolvers_cms.go`
- Storage support to enable pagination and “my publications” lookups:
  - `pkg/storage/repositories/pagination_helpers.go`
  - `pkg/storage/repositories/article_repository.go`
  - `pkg/storage/repositories/draft_repository.go`
  - `pkg/storage/repositories/revision_repository.go`
  - `pkg/storage/repositories/series_repository.go`
  - `pkg/storage/models/publication.go`
  - `pkg/storage/repositories/publication_member_repository.go`
- Note: CMS parity is GraphQL-first and is not represented by the Lift route inventory, so `docs/specs/graphql_coverage.yaml` does not track these operations.

**Exit criteria**
- CMS user-facing flows are implementable via GraphQL without REST fallbacks.
  - `make gqlgen`, `make lint`, and `make test` are green.

## Phase 2 — Client Parity Features (Completed)

**Goal**: Cover the “client parity” feature set required for a first-party Greater client without REST fallbacks.

**Scope**
- **Data portability**: imports/exports (job creation + status + list + download URL patterns)
- **Mastodon v2 client features**:
  - Filters (CRUD + keywords + statuses + test)
  - Trends (tags/statuses/links + mixed trends)
  - Grouped notifications (list groups + mark group read)

**Deliverables**
- GraphQL schema + resolvers for:
  - Imports/exports job lifecycle
  - Filter management (including keywords/statuses)
  - Trending discovery endpoints
  - Grouped notifications
- Update `docs/specs/graphql_coverage.yaml` for related Lift routes:
  - mark as `status: covered`
  - add `graphql:` mappings to the implemented operations

**Exit criteria**
- A Greater client can implement the above features via GraphQL only (excluding explicit exemptions).

## Phase 3 — Admin Parity + Strict Enforcement (Completed)

**Goal**: Finish the remaining “product functionality” parity, then turn on strict enforcement.

**Scope**
- Admin operations parity (reports queue/actions, domain allows/blocks, admin account actions, etc.)
- Any remaining non-exempt routes required by first-party clients and admin UI

**Deliverables**
- GraphQL schema + resolvers for remaining `graphql_required` routes.
- Update `docs/specs/graphql_coverage.yaml` so `status: missing` trends toward zero.
- Enable strict enforcement mode:
  - CI fails if any `graphql_required` route remains `status: missing` (after backlog is burned down).

**Exit criteria**
- `graphql_required` routes have `status: covered` with valid `graphql:` mappings.
- Strict enforcement enabled and green in CI.

**Completion notes**
- `docs/specs/graphql_coverage.yaml`: `graphql_required covered=200, missing=0`
- `make verify-graphql-coverage` runs in strict mode and passes (`--check --strict`)
- `make lint` and `make test` are green after admin parity work landed

## Workstreams (how Phase 3 is executed)

These are the largest remaining clusters in the Lift route inventory (order matters because later work often depends on earlier primitives):

1. **Accounts + relationships** (`/api/v1/accounts/*`)
2. **Statuses + timelines + interactions** (`/api/v1/statuses/*`, `/api/v1/notes/*`)
3. **Media** (`/api/v1/media/*`)
4. **Conversations + notifications** (`/api/v1/conversations/*`, remaining notification gaps)
5. **Search + suggestions + instance endpoints** (`/api/v1/search/*`, `/api/v2/search`, `/api/v2/suggestions`, `/api/v1/instance/*`, `/api/v2/instance`)
6. **Admin parity** (`/api/v1/admin/*`) — biggest surface area; expect multiple sub-milestones.

Each workstream ends with:
- schema changes (if needed)
- resolvers + service wiring
- `make gqlgen`, `make schema`
- `docs/specs/graphql_coverage.yaml` updated to `covered` for the routes in that workstream
- `make verify-graphql-coverage`, `make test`, `make lint`
