# lesser: Lift → AppTheory Migration Roadmap

Generated: 2026-01-31

This document is a repo-specific migration plan from Lift (`github.com/pay-theory/lift`) to AppTheory. If DynamORM
(`github.com/pay-theory/dynamorm`) is detected in this repo, it also includes a DynamORM → TableTheory plan.

## Scope
- In scope: inventory Lift usage, plan an incremental migration to AppTheory, and define acceptance criteria per milestone.
- Optional: inventory DynamORM usage and plan a migration to TableTheory only when DynamORM is present.
- Out of scope: unrelated refactors or feature changes.

## Destination (pinned): AppTheory + TableTheory

This section defines the **pinned destination frameworks** for this migration. These values are **constants** provided by
the GovTheory pack (do not guess; do not use placeholders).

### AppTheory (pinned)
- Go module: `github.com/theory-cloud/apptheory@v0.5.0`
- Go runtime import: `github.com/theory-cloud/apptheory/runtime`
- Docs entrypoints (for tag `v0.5.0`):
  - `docs/getting-started.md`
  - `docs/migration/from-lift.md`
- Copy/paste dependency command:
  - `go get github.com/theory-cloud/apptheory@v0.5.0`
- Recommended pinned docs links:
  - `https://github.com/theory-cloud/AppTheory/blob/v0.5.0/docs/getting-started.md`
  - `https://github.com/theory-cloud/AppTheory/blob/v0.5.0/docs/migration/from-lift.md`

### TableTheory (pinned)
- Go module: `github.com/theory-cloud/tabletheory@v1.3.0`
- Docs entrypoints (for tag `v1.3.0`):
  - `docs/getting-started.md`
  - `docs/api-reference.md`
  - `docs/migration-guide.md`
- Copy/paste dependency command:
  - `go get github.com/theory-cloud/tabletheory@v1.3.0`
- Recommended pinned docs links:
  - `https://github.com/theory-cloud/TableTheory/blob/v1.3.0/docs/getting-started.md`
  - `https://github.com/theory-cloud/TableTheory/blob/v1.3.0/docs/api-reference.md`
  - `https://github.com/theory-cloud/TableTheory/blob/v1.3.0/docs/migration-guide.md`

## Repo inventory (fill from repo scan)

### Lift usage
- Lift detected: YES
- Evidence:
  - `go.mod`:
    - `github.com/pay-theory/lift v1.0.82`
  - `infra/cdk/go.mod`:
    - `github.com/pay-theory/lift v1.0.82`
  - Imports/usages (representative examples; repo contains many more):
    - API routing and handler wiring:
      - `cmd/api/main.go` (imports `github.com/pay-theory/lift/pkg/lift`, `github.com/pay-theory/lift/pkg/middleware`)
      - `cmd/api/routes_lift.go` (configures HTTP routes on `*lift.App`)
      - `cmd/api/lift_handlers.go` + `cmd/api/lift/*.go` (Lift handler implementations; directory is explicitly named `lift/`)
    - Shared Lift bootstrapping abstractions:
      - `pkg/lambda/main_framework.go` (creates `*lift.App`, adds middleware, calls `app.HandleRequest`)
      - `pkg/lift/*.go` (local wrapper package that builds/configures Lift apps and middleware)
    - Other Lambda entrypoints using Lift directly:
      - `cmd/activity-processor/main.go` (imports `github.com/pay-theory/lift/pkg/lift`, `github.com/pay-theory/lift/pkg/middleware`)
      - Many other `cmd/*/main.go` files import Lift for standardized middleware and request handling.

- Primary impacted entrypoints / services (highest-value/most central):
  - Public API Lambda:
    - `cmd/api/main.go`
    - `cmd/api/routes_lift.go`
    - `cmd/api/lift_handlers.go`
    - `cmd/api/lift/` (all route handlers)
  - Shared Lambda framework used across many services:
    - `pkg/lambda/main_framework.go`
  - Local Lift abstraction layer (likely becomes the AppTheory landing zone or gets replaced):
    - `pkg/lift/`
  - Infrastructure code that depends on Lift (likely only for types/helpers; verify):
    - `infra/cdk/go.mod` and any `infra/cdk/**/*.go` imports that reference Lift.

- Notes / risks:
  - The API Lambda explicitly states “All routing is handled by the Lift framework” (`cmd/api/main.go` header comment). Any framework migration risks breaking Mastodon-compatible path matching and middleware behavior.
  - Route surface area is large: `cmd/api/routes_lift.go` configures many endpoints; most handlers live in `cmd/api/lift/*.go`. This increases the need for slice-by-slice migration plus strong regression tests.
  - There is a shared “standardized Lambda main” abstraction in `pkg/lambda/main_framework.go` that is Lift-based. Migrating it affects many Lambdas at once; do it deliberately (or keep it as a compatibility shim during early slices).

### DynamORM usage (optional)
- DynamORM detected: YES
- Evidence:
  - `go.mod`:
    - `github.com/pay-theory/dynamorm v1.0.39`
  - Imports/usages (representative examples; repo contains many more):
    - Direct DB core import (DynamORM core):
      - `cmd/api/main.go` imports `dynamormCore "github.com/pay-theory/dynamorm/pkg/core"`
      - `cmd/activity-processor/main.go` imports `github.com/pay-theory/dynamorm/pkg/core`
    - DynamORM storage layer and adapter patterns:
      - `pkg/storage/dynamorm/adapter.go` (explicitly described as a “critical StorageAdapter bridge”)
      - `pkg/storage/dynamorm/base.go`, `pkg/storage/dynamorm/client.go`, `pkg/storage/dynamorm/lambda_init.go`
      - `pkg/storage/dynamorm/repositories/*`
    - Lambda-optimized client construction:
      - `cmd/api/main.go` references `dynamorm.NewLambdaOptimizedClient` (via `newLambdaOptimizedClient` var)

- Tables/models involved (repo-specific, from scan and code organization):
  - Primary DynamoDB table name is environment/config driven:
    - `cfg.DynamoTableName` (e.g., in `cmd/api/main.go`, `cmd/activity-processor/main.go`)
    - A fallback default string appears in `cmd/api/main.go` manual init path: `lesser-main`
  - Domain model packages driving table items:
    - `pkg/storage/models` (shared model definitions)
    - `pkg/storage/interfaces` (repository interfaces)
    - `pkg/storage/dynamorm/repositories/*` (per-entity repositories)
  - Key schema and access patterns are critical and documented in-code:
    - `pkg/storage/dynamorm/adapter.go` comment block lists preserved PK/SK patterns (Users, Actors, Objects, DNS Cache) and mentions GSIs/TTL.

- Notes / risks:
  - This repo heavily centralizes business state in a single DynamoDB table with multiple entity types and key patterns. Any TableTheory migration must preserve PK/SK composition, GSIs, and TTL semantics.
  - Multiple Lambdas construct DB clients and repository factories; shifting the data layer will be highly cross-cutting.

## Migration principles
- Prefer incremental change: introduce adapters/shims first, then migrate call sites.
- Keep behavior stable: add regression tests at boundaries before swapping implementations.
- Remove legacy dependencies only after all call sites are migrated and verified.

## Roadmap (sequenced milestones)

### M0 — Baseline + guardrails
**Goal:** create an accurate inventory and reduce migration risk.

**Steps**
1. Lock in a “current behavior” baseline for the API Lambda routing surface:
   - Identify which files define the canonical route list:
     - `cmd/api/routes_lift.go`
     - `cmd/api/lift_handlers.go`
   - Use existing verification tooling to ensure route configuration/spec artifacts are current:
     - `make verify-openapi-strict`
     - `make verify-graphql-coverage`
2. Complete the “Repo inventory” section above with anything still missing after deeper scans:
   - Run repo-wide searches and save counts/output into an internal engineering note (not required in this doc):
     - `rg "github.com/pay-theory/lift" -n`
     - `rg "github.com/pay-theory/dynamorm" -n`
3. Identify the highest-risk behaviors that must not change:
   - HTTP path matching (Mastodon endpoints) and middleware order in:
     - `cmd/api/main.go`
     - `cmd/api/middleware.go`
     - `pkg/lift/app.go` (local Lift middleware builder)
   - Lambda handler wiring / event shapes (API Gateway vs stream processors):
     - `cmd/api/main.go`
     - `cmd/activity-processor/main.go`
4. Add/expand regression tests around the highest-risk workflows (do this before swapping frameworks):
   - Candidate test locations already present and likely to extend:
     - `cmd/api/routes_lift_test.go` (route behavior)
     - `cmd/api/routes_lift_manifest_test.go` + `cmd/api/testdata/routes_lift_manifest.txt` (route manifest snapshot; regenerate with `UPDATE_ROUTE_MANIFEST=1 go test ./cmd/api -run TestConfigureLiftRoutes_RouteManifestMatchesSnapshot`)
     - `cmd/api/lift/*_test.go` (handler-level tests)
     - `pkg/lambda/main_framework_test.go` (framework-level behavior)

**Acceptance criteria**
- Inventory has concrete file paths (not generic statements).
- A minimal test safety net exists for the highest-risk workflows.
- Current verification loop is documented (see “Suggested verification commands”).

### M1 — Introduce AppTheory structure (no behavior change)
**Goal:** add a clear landing zone for AppTheory migration work without changing production behavior.

**Implemented (this branch)**
- Landing package boundary: `pkg/apptheory/` (AppTheory + TableTheory wiring namespace; no production use yet).
- Destination deps pinned in `go.mod`:
  - `github.com/theory-cloud/apptheory@v0.5.0`
  - `github.com/theory-cloud/tabletheory@v1.3.0`
- First migration slice selected: API health endpoints (see Step 3).

**Steps**
1. Decide the target architectural boundaries for AppTheory in this repo.
   - Recommended approach for *lesser* (serverless, many Lambdas):
     - Introduce a new package namespace for AppTheory wiring (example names only; choose one and standardize):
       - `pkg/apptheory/` (AppTheory runtime + app construction + middleware)
       - or `pkg/runtime/` (if you want to treat AppTheory as “the runtime”)
   - Keep the existing Lift-based packages (`pkg/lift`, `pkg/lambda`) intact until a slice is migrated.
2. Define an explicit mapping plan from Lift concepts used here to AppTheory concepts.
   - Lift touchpoints to map (repo-specific):
     - `*lift.App` creation (`lift.NewHTTPApp(...)` in `pkg/lift/app.go` and `createStandardizedLiftApp(...)` in `pkg/lambda/main_framework.go`)
     - Middleware stack order (`pkg/lambda/main_framework.go`, `cmd/api/middleware.go`, `pkg/lift/app.go`)
     - Handler signatures and context usage (`cmd/api/lift/*.go` use Lift `*lift.Context` heavily)
   - Mapping notes for Lesser (AppTheory runtime import: `github.com/theory-cloud/apptheory/runtime`):
     - App container:
       - Lift: `lift.New(...)` / `lift.New(lift.WithDebug())`
       - AppTheory: `apptheory.New(...)` (Tier defaults to P2; can set `apptheory.WithTier(...)`)
     - Routes:
       - Lift: `app.GET/POST/PUT/PATCH/DELETE(...)` and `app.Handle(...)`
       - AppTheory: `app.Get/Post/Put/Patch/Delete(...)` and `app.Handle(...)`
       - Route patterns: Lift-style `:param` segments are accepted by AppTheory (canonicalized to `{param}`); verify any wildcard/proxy usage via the Lift route manifest snapshot.
     - Handlers:
       - Lift handler signature: `func(*lift.Context) error`
       - AppTheory handler signature: `func(*apptheory.Context) (*apptheory.Response, error)`
       - Lift response pattern: `ctx.Status(code).JSON(value)` / `ctx.Text(...)` / `ctx.Bytes(...)`
       - AppTheory response helpers: `apptheory.JSON(code, value)` / `apptheory.Text(...)` / `apptheory.Binary(...)`
     - Middleware:
       - Lift: `app.Use(lift.Middleware)` (custom ordering is fully controlled by registration order)
       - AppTheory: `app.Use(apptheory.Middleware)`; portable built-ins run in a contract-defined order in P1/P2 (request-id → recovery → logging → CORS → auth → handler), and user middleware wraps the final handler stage.
       - Context value bag: Lift `ctx.Set/Get` maps to AppTheory `ctx.Set/Get` for request-scoped state sharing.
     - AWS entrypoints:
       - Lift: `app.HandleRequest(ctx, event)` (event type is inferred at runtime)
       - AppTheory: prefer explicit entrypoints per Lambda trigger (e.g. API Gateway v2 HTTP API), or use `app.HandleLambda(ctx, json.RawMessage)` when keeping Lift’s “single entrypoint router” posture.
3. Select one low-risk vertical slice to migrate first.
   - Recommended first slice for this repo:
     - A minimal endpoint group with clear request/response behavior and strong tests, such as the health endpoints:
       - `cmd/api/main.go` (`configureHealthRoutes`)
       - Health endpoints (all `GET`): `/health`, `/health/live`, `/health/ready`, `/health/detailed`
   - Alternative low-risk slice:
     - A background Lambda with narrow event shape (if AppTheory supports it cleanly), but avoid stream processors first because they also depend on DynamORM.

**Acceptance criteria**
- Target package boundaries are documented with repo-specific paths.
- A first migration slice is selected with explicit files and scope.
- No production behavior change yet (planning-only milestone).

### M2 — Migrate Lift call sites in slices
**Goal:** move Lift usage to AppTheory incrementally.

**Important repo-specific note:** Lift is used both directly and indirectly:
- Directly by service entrypoints (many `cmd/*/main.go` import Lift types).
- Indirectly through shared wrappers (`pkg/lambda`, `pkg/lift`).

That means you should plan slices so you don’t accidentally migrate “everything” by changing the shared wrapper too early.

**Steps**
1. Slice 1 (recommended): API health endpoints
   - Files likely in-scope:
     - `cmd/api/main.go` (`configureHealthRoutes`)
     - Any shared middleware referenced by these handlers:
      - `cmd/api/middleware.go`
      - `pkg/middleware/*` (if used)
   - Implementation status (2026-01-31):
     - AppTheory now serves the health endpoints via explicit dispatch in `cmd/api/main.go` (`handleAPIRequest`).
     - Added regression test: `cmd/api/main_round12_test.go` (`TestMain_HealthEndpointsUseAppTheoryRound12`).
   - Acceptance test focus:
     - Status codes, headers (especially CORS), and response JSON.
   - Verification commands:
     - `go test ./cmd/api -run TestMain_HealthEndpointsUseAppTheoryRound12`
     - `make verify-unit`
   - Rollback plan:
     - Revert the slice commit (or temporarily route `/health*` back to Lift by restoring the Lift route registration and removing AppTheory dispatch).
2. Slice 2: “Auth bootstrap” and OAuth-adjacent endpoints
   - Files likely in-scope:
     - `cmd/api/routes_lift.go` routes for:
       - `/oauth/*`
       - `/setup/*`
       - `/auth/wallet/*`
       - `/api/v1/auth/webauthn/*`
     - Handler implementations in:
       - `cmd/api/lift/oauth.go`
       - `cmd/api/lift/oauth_consent.go`
       - `cmd/api/lift/setup.go`
       - `cmd/api/lift/webauthn.go`
       - `cmd/api/lift/apps.go`
   - Implementation status (2026-01-31):
     - AppTheory now serves the “auth bootstrap” routes using Lift handler shims:
       - Route wiring: `cmd/api/routes_apptheory_auth.go` (`configureAuthRoutesAppTheory`)
       - Lift→AppTheory adapter: `cmd/api/apptheory_lift_adapter.go`
       - Dispatch: `cmd/api/main.go` (`handleAPIRequest`, `shouldRouteToAuthAppTheory`)
     - Tests added for routing + stage-prefix stripping:
       - `cmd/api/apptheory_dispatch_round12_test.go`
   - Risks:
     - Rate limiting and middleware order (`ratelimit.ApplyRateLimit` wrappers in `cmd/api/routes_lift.go`).
   - Verification commands:
     - `go test ./cmd/api -run TestShouldRouteToAuthAppTheoryRound12`
     - `make verify-unit`
   - Rollback plan:
     - Revert the slice commit (or temporarily route these endpoints back to Lift by disabling the AppTheory dispatch predicate).
3. Slice 3+: migrate remaining API route groups
   - Group by API domain to keep PRs reviewable:
     - Accounts: `cmd/api/lift/accounts*.go`
     - Statuses/timelines: `cmd/api/lift/status*.go`, `cmd/api/lift/timelines.go`
     - Moderation/admin: `cmd/api/lift/admin*.go`, `cmd/api/lift/moderation.go`
     - Media: `cmd/api/lift/media.go`
     - Federation discovery: `cmd/api/lift/nodeinfo.go`, `cmd/api/lift/webfinger.go`, `cmd/api/lift/discovery.go`
   - Slice 3 (Accounts) implementation status (2026-01-31):
     - AppTheory now serves account + user-level endpoints using Lift handler shims:
       - Route wiring: `cmd/api/routes_apptheory_accounts.go`
       - API route aggregation: `cmd/api/routes_apptheory_api.go`
       - Dispatch + stage-prefix normalization: `cmd/api/main.go` (`shouldRouteToAPIAppTheory`, `normalizeLambdaEventForAppTheory`)
     - Tests:
       - `cmd/api/apptheory_dispatch_round12_test.go` (accounts routes + stage-prefix normalization regression)
   - Verification commands:
     - `go test ./cmd/api -run TestShouldRouteToAPIAppTheoryRound12`
     - `go test ./cmd/api -run TestHandleAPIRequest_StripsStagePrefixForAppTheoryRound12`
     - `make verify-unit`
   - Rollback plan:
     - Revert the slice commit (or temporarily route these endpoints back to Lift by removing the accounts routes from the dispatch predicate).
   - Slice 4 (Statuses/timelines) implementation status (2026-01-31):
     - AppTheory now serves the main “social API” surfaces using Lift handler shims:
       - Route wiring: `cmd/api/routes_apptheory_statuses.go`
       - API route aggregation: `cmd/api/routes_apptheory_api.go`
       - Dispatch predicate: `cmd/api/main.go` (`apiAppTheoryRoutes`)
     - Tests:
       - `cmd/api/apptheory_dispatch_round12_test.go` (spot checks for common routes)
   - Verification commands:
     - `go test ./cmd/api -run TestShouldRouteToAPIAppTheoryRound12`
     - `make verify-unit`
   - Rollback plan:
     - Revert the slice commit (or temporarily route these endpoints back to Lift by removing the statuses routes from the dispatch predicate).
   - Slice 5 (Moderation/admin) implementation status (2026-01-31):
     - AppTheory now serves moderation and admin APIs using Lift handler shims:
       - Route wiring: `cmd/api/routes_apptheory_moderation.go`
       - API route aggregation: `cmd/api/routes_apptheory_api.go`
       - Dispatch predicate: `cmd/api/main.go` (`apiAppTheoryRoutes`)
     - Tests:
       - `cmd/api/apptheory_dispatch_round12_test.go` (spot checks for moderation + admin routes)
   - Verification commands:
     - `go test ./cmd/api -run TestShouldRouteToAPIAppTheoryRound12`
     - `make verify-unit`
   - Rollback plan:
     - Revert the slice commit (or temporarily route these endpoints back to Lift by removing the moderation routes from the dispatch predicate).
   - Slice 6 (Media + misc) implementation status (2026-01-31):
     - AppTheory now serves media and miscellaneous endpoints using Lift handler shims:
       - Route wiring: `cmd/api/routes_apptheory_media.go`
       - API route aggregation: `cmd/api/routes_apptheory_api.go`
       - Dispatch predicate: `cmd/api/main.go` (`apiAppTheoryRoutes`)
     - Tests:
       - `cmd/api/apptheory_dispatch_round12_test.go` (spot checks for media + misc routes)
   - Verification commands:
     - `go test ./cmd/api -run TestShouldRouteToAPIAppTheoryRound12`
     - `make verify-unit`
   - Rollback plan:
     - Revert the slice commit (or temporarily route these endpoints back to Lift by removing the media + misc routes from the dispatch predicate).
   - Slice 7 (Federation discovery) implementation status (2026-01-31):
     - AppTheory now serves federation discovery endpoints using Lift handler shims:
       - Route wiring: `cmd/api/routes_apptheory_federation.go`
       - API route aggregation: `cmd/api/routes_apptheory_api.go`
       - Dispatch predicate: `cmd/api/main.go` (`apiAppTheoryRoutes`)
     - Tests:
       - `cmd/api/apptheory_dispatch_round12_test.go` (spot checks for discovery routes)
   - Verification commands:
     - `go test ./cmd/api -run TestShouldRouteToAPIAppTheoryRound12`
     - `make verify-unit`
   - Rollback plan:
     - Revert the slice commit (or temporarily route these endpoints back to Lift by removing the federation discovery routes from the dispatch predicate).
4. After the API Lambda is migrated, evaluate shared wrapper migration:
   - If `pkg/lambda/main_framework.go` is used broadly for non-API Lambdas, migrating it may migrate many services at once.
   - Consider keeping `pkg/lambda` as a Lift compatibility layer until you have migrated a representative set of Lambdas.
5. Update dependency management:
   - Remove `github.com/pay-theory/lift` from **both** module files only after code search confirms no remaining imports/usages:
     - Root `go.mod`
     - `infra/cdk/go.mod`

**Acceptance criteria**
- Each slice has:
  - Explicit file list.
  - Verification commands and required checks.
  - Rollback plan (revert slice PR; keep Lift-based path available until slice verified).
- `rg "github.com/pay-theory/lift" -n` returns no results before removing Lift from `go.mod`.
- `go test ./...` and `make verify` remain green.

### M3 — (Optional) DynamORM → TableTheory migration
**Goal:** migrate DynamoDB access layer only when DynamORM is present.

DynamORM detected = YES, so this section applies.

**Repo-specific warning:** This repo already contains a substantial “adapter bridge” layer (`pkg/storage/dynamorm/adapter.go`) and many repositories under `pkg/storage/dynamorm/repositories/*`. Treat the migration as a data-access rewrite with strict behavioral compatibility requirements.

**Steps (when applicable)**
1. Inventory DynamORM usage at the “construction points” (where clients/factories are created), because those are the best slice boundaries.
   - Representative construction points to audit first:
     - API Lambda:
       - `cmd/api/main.go` (uses `dynamorm.NewLambdaOptimizedClient` via `newLambdaOptimizedClient`)
     - Stream processor:
       - `cmd/activity-processor/main.go` (calls `dynamorm.GetClient(...)` and `factory.NewRepositoryFactory(...)`)
     - Shared data layer:
       - `pkg/storage/dynamorm/client.go`
       - `pkg/storage/dynamorm/lambda_init.go`
2. Define a TableTheory mapping at the repository boundary.
   - Current boundary interfaces:
     - `pkg/storage/interfaces/*` (repository interfaces)
     - `pkg/storage/core/*` (factory/storage abstractions)
   - Current DynamORM implementation:
     - `pkg/storage/dynamorm/repositories/*`
     - `pkg/storage/dynamorm/adapter.go` (bridge)
   - Migration decision to make:
     - Either:
       1) Re-implement repositories using TableTheory while keeping interfaces stable, or
       2) Introduce a new TableTheory-native repository interface layer and adapt callers.
     - For *lesser*, option (1) is usually lower-risk because it avoids touching all service logic at once.
3. Migrate in slices (table/model oriented, but executed via repositories).
   - Suggested first slice (lowest blast radius):
     - Choose a read-mostly, low-write entity (example only; decide based on usage in code): DNS cache patterns referenced in `pkg/storage/dynamorm/adapter.go` comments.
   - Next slices:
     - Users/Profiles
     - Actors
     - Objects
     - Timeline + notification fanout (higher risk)
4. Validate data-layer behavior before removing DynamORM.
   - Add or strengthen integration tests where feasible.
   - For local validation, consider a DynamoDB Local test harness if already present; otherwise, define one.
5. Remove DynamORM dependency from `go.mod` only after all call sites migrated:
   - `rg "github.com/pay-theory/dynamorm" -n` returns no results.

**Acceptance criteria**
- A repository-by-repository migration plan exists with explicit file lists.
- Data-layer behavior is validated (unit tests + targeted integration checks) before removing DynamORM.
- No remaining DynamORM imports/usages before removing `github.com/pay-theory/dynamorm` from `go.mod`.

### M4 — Cleanup + verification hardening
**Goal:** ensure the migration is complete, and future drift is prevented.

**Steps**
1. Confirm no remaining Lift/DynamORM references via repo-wide search:
   - `rg "github.com/pay-theory/lift" -n` => 0 results
   - `rg "github.com/pay-theory/dynamorm" -n` => 0 results
2. Strengthen CI verification around migrated surfaces:
   - Ensure `make verify` remains the canonical gate (already aggregates many checks).
   - Ensure the OpenAPI/GraphQL coverage generators remain consistent post-migration:
     - `make verify-openapi-strict`
     - `make verify-graphql-coverage`
3. Document follow-on work:
   - Performance tuning after framework swap.
   - Observability alignment (logging/tracing/metrics) if AppTheory defaults differ from Lift.

**Acceptance criteria**
- Repo-wide search shows zero Lift/DynamORM imports/usages.
- CI checks remain green and cover the migrated pathways.

## Suggested verification commands (fill from repo tooling)
- Build:
  - `go build ./...`
  - (full deployment build) `make build`
- Unit tests:
  - `go test ./...`
  - (short unit suite) `make verify-unit`
- Integration tests (if present / networked smoke tests):
  - `make verify-smoke` (runs `smoke-core` + `smoke-federation`)
- Lint/format / repo gates:
  - `make verify` (lambda set, inventory, docs, ai-training docs, schema, graphql coverage, openapi, unit tests)
  - Optional: `make verify-cdk` (requires CDK toolchain)
