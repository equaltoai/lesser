# GraphQL Coverage: 4‑Phase Implementation Plan

> **Status**: Active  
> **Owner**: Lesser  
> **Last updated**: 2025-12-25  
> **Related**: `docs/specs/GRAPHQL_COVERAGE.md`, `docs/specs/graphql_coverage.yaml`

This document is the concrete, phase-based plan for achieving **GraphQL parity** for all in-scope Lesser product functionality while keeping REST as the source of truth for standards/protocol endpoints and other explicitly REST-only flows.

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

## Phase 1 — CMS GraphQL Parity (Planned)

**Goal**: Expose CMS functionality via GraphQL to support Greater-based content/admin experiences.

**Scope**
- Articles/drafts/revisions/publications
- Series/categories/tags
- Author/editor workflows needed by the intended Greater client

**Deliverables**
- Schema additions (types + queries + mutations) under `graph/*`.
- Resolvers wired through `pkg/services/cms/*` (or equivalent service layer).
- Update `docs/specs/graphql_coverage.yaml` routes related to CMS/admin content management to:
  - `status: covered`
  - `graphql: [Query.<field>, Mutation.<field>]` mappings

**Exit criteria**
- CMS user-facing flows are implementable via GraphQL without REST fallbacks.

## Phase 2 — Client Parity Features (In Progress)

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

## Phase 3 — Admin Parity + Strict Enforcement (Planned)

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

