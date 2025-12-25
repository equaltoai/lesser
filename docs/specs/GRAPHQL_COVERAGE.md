# GraphQL Coverage (Product Contract + Drift Guardrail)

## Summary
Lesser must expose **100% of product functionality** via GraphQL so Greater-based clients and admin UIs can operate
without falling back to REST.

This spec defines:
- the **coverage contract** (what “100%” means for Lesser), and
- the **guardrail** that prevents REST/GraphQL drift as features evolve.

## Scope
### In scope (must be GraphQL-covered)
- All product and admin capabilities used by first-party clients (including CMS).
- Import/export, filters, trends, grouped notifications (explicitly required for Greater client parity).

### Explicit exemptions (REST-only)
These remain REST-first/standards-first and are **not required** to be GraphQL-covered:
- **OAuth/OIDC** flows (authorize/token/discovery/redirects/client registration).
- **Wallet auth, WebAuthn, and bootstrap/setup** flows.
- **Protocol/infra endpoints**:
  - ActivityPub actor/inbox/outbox/object handlers
  - `/.well-known/*`, `/nodeinfo/*`
  - `/health*`

## Current state
- GraphQL is served from `/api/graphql` (`cmd/graphql`) with subscriptions via `cmd/graphql-ws`.
- GraphQL schema sources are listed in `gqlgen.yml` (`graph/core.graphql`, `graph/phase2.graphql`, `graph/phase3.graphql`).
- CMS services exist (`pkg/services/cms/*`), but CMS is not yet exposed in the GraphQL schema.

## Known gaps (non-exhaustive)
These are high-value missing areas that must be added to reach “100%” in-scope coverage:
- **CMS**: articles/drafts/revisions/publications/series/categories (see `docs/HEADLESS_CMS_DESIGN.md`).
- **Data portability**: imports/exports (status + signed URL patterns).
- **Mastodon v2 client features**: filters, trends, grouped notifications.
- **Admin ops parity**: reports queue/actions, domain allows/blocks, admin user/account actions.

Phase 0 does not implement these; it only locks the contract and installs drift guardrails.

## Contract artifact: `docs/specs/graphql_coverage.yaml`
`docs/specs/graphql_coverage.yaml` is the canonical, machine-readable coverage inventory for API-Lambda Lift routes.

Each route is tracked as one of:
- `policy: graphql_required`: the REST route must have an equivalent GraphQL operation.
- `policy: rest_only`: the route is REST-only by design (matched by an exemption).

For `policy: rest_only`, `exemptedBy` must be set to the exemption `id` that justifies the classification.

For `policy: graphql_required`, `status` is used to track parity:
- `covered`: there is at least one GraphQL operation that provides equivalent capability.
- `missing`: capability exists in REST today but is not yet in GraphQL (must be driven to zero).

## Guardrail: coverage verifier
A verifier enforces that:
- every Lift route is present in `docs/specs/graphql_coverage.yaml` (no silent drift), and
- every claimed GraphQL mapping in that file exists in the current schema (no stale mappings).

### Commands
- Regenerate coverage file (adds new routes, removes deleted routes):
  - `make generate-graphql-coverage`
- Verify coverage file matches code + schema:
  - `make verify-graphql-coverage`

## Next steps (Phase 1+)
- Add CMS schema + resolvers (phase 1).
- Add imports/exports, filters/trends/grouped notifications, admin parity (phase 2/3).
- Turn on strict enforcement (fail CI unless `missing == 0`) once the backlog is burned down.
