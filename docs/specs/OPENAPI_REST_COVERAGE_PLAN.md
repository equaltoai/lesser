# OpenAPI REST Coverage Plan

> **Status**: Active  
> **Last updated**: 2025-12-25  
> **OpenAPI file**: `docs/specs/openapi.yaml` (file-only; not served at runtime)

This plan defines how we get to **fully comprehensive OpenAPI coverage** for Lesser’s public HTTP surface so Greater-based clients (and third parties) can integrate via generated, typed SDKs with minimal guesswork.

## Definitions (what “covered” means)

An endpoint is “OpenAPI-covered” when its OpenAPI operation includes:
- Correct **path + HTTP method** (drift-checked vs deployed routes).
- Correct **auth** requirements (explicitly public or explicitly authenticated).
- All required **path/query/header parameters**.
- Correct **request body content type** (`application/json`, `multipart/form-data`, `application/x-www-form-urlencoded`, etc) and schema.
- **Response schemas** for success + expected failures (not just `200: OK` placeholders).
- Pagination contract where applicable (`limit`, cursors, `Link` header).

For coverage enforcement we will treat these as **levels**:
- **L0: routed** — operation exists (current baseline).
- **L1: shaped** — auth + parameters + status codes are correct.
- **L2: typed** — request/response bodies use explicit schemas (`$ref`), not “free-form object”.
- **L3: strict** — no placeholder schemas remain; common errors/pagination are consistent.

## Current state

- `tools/openapi` generates/validates `docs/specs/openapi.yaml` by merging API Lift routes, SSE routes, and inventory HTTP routes (file-only).
- `make generate-openapi` and `make verify-openapi` exist and are wired into `make verify`.
- The spec is currently **L0** (route skeleton). Most operations still use placeholder schemas and generic `200` responses.

## Known gaps (must fix early)

### 1) Route inventory gaps (OpenAPI missing deployed routes)

Resolved in Phase 0: `docs/specs/openapi.yaml` now includes the deployed surface contributed by:
- API Lift routes (`cmd/api/routes_lift.go`, `cmd/api/main.go`).
- SSE Lift routes (`cmd/sse/main.go`).
- Inventory-driven HTTP lambdas (`infra/cdk/inventory/lambdas.go`) including federation + `GET|POST /api/graphql`.

### 2) Routing mismatches (deployed route vs handler route)

Resolved in Phase 0:
- Inbox/outbox inventory routes align with their handlers (`/users/{username}/inbox`, `/users/{username}/outbox`).
- `/health` exists as a legacy alias for infra expectations.

### 3) Spec completeness gaps (OpenAPI exists but not useful for clients yet)

- Request bodies for most write endpoints are currently `type: object` with `additionalProperties: true`.
- Responses are often only `200: OK` with no schema and missing error responses.
- Query parameters are largely missing (pagination, filters, search parameters, etc).
- Multipart endpoints (media upload) aren’t modeled as multipart.
- OAuth endpoints aren’t modeled as `application/x-www-form-urlencoded` where required.
- Streaming endpoints need `text/event-stream`.
- Auth scopes are inconsistent across code (`admin`, `admin:read`, `admin:write`, `write:follows`, etc) and must be documented correctly.

## Plan (phased, comprehensive)

### Phase 0 — Expand OpenAPI route sources (L0 completion)

**Goal**: OpenAPI contains every public HTTP route we deploy (no missing paths).

1. Extend `tools/openapi` route extraction to merge:
   - API Lift routes: `cmd/api/routes_lift.go`, `cmd/api/main.go` (already).
   - SSE Lift routes: `cmd/sse/main.go`.
   - Inventory HTTP routes: `infra/cdk/inventory/lambdas.go` for federation HTTP lambdas + `/api/graphql`.
2. Add `x-lesser-lambda` / `x-lesser-routeSources` vendor extensions per operation (debuggability).
3. Resolve routing mismatches:
   - Fix `inbox` route mismatch (either update inventory to `/users/{username}/inbox` or register both paths in `cmd/inbox/main.go`).
   - Decide what `/health` should do (add alias handler or remove the route from infra).

**Exit criteria**
- `make verify-openapi` includes the full deployed surface (API + SSE + federation + webfinger + graphql HTTP).
- No known infra ↔ handler route mismatches remain.

### Phase 1 — Contract foundations (L1)

**Goal**: Provide consistent auth/error/pagination primitives so every endpoint can reference them.

1. Add/standardize OpenAPI components:
   - `Error` (and variants if needed: `ValidationError`, `UnauthorizedError`, etc).
   - Pagination params: `limit`, `max_id`, `since_id`, `min_id`, `page`, etc (as reusable parameters).
   - Common identifiers and formats (`SnowflakeID`, `URI`, `RFC3339DateTime`).
2. Normalize auth modeling:
   - Keep `bearerAuth` as the primary scheme for generated clients.
   - Keep `setupBearer` for `/setup/*` session token usage.
   - Keep `oauth2` for documentation (flows + scope descriptions), but do not rely on it for client generation.
   - Enumerate the complete scope taxonomy (including hierarchical scopes) in one place in the spec.
3. Add operation-level security defaults + explicit “public” operations:
   - `/api/v1/admin/*` always authenticated (admin scope documented).
   - `/setup/admin` uses `setupBearer`.
   - `/setup/finalize` uses `bearerAuth`.
   - Wallet challenge/login endpoints are public.

**Exit criteria**
- Every operation has correct `security` (or explicitly public) and correct status codes (2xx + expected 4xx/5xx), even if bodies remain placeholders.

### Phase 2 — Model schemas pipeline (L2)

**Goal**: Make schemas maintainable by generating them from Go API models wherever possible.

1. Inventory the response/request types:
   - Prefer existing `cmd/api/models/*` types for Mastodon-compatible responses.
   - Where handlers return `map[string]any`, introduce typed request/response structs in `cmd/api/models` (or a new `cmd/api/openapi` package).
2. Implement schema generation:
   - Add a generator that produces `components.schemas` from Go types (e.g., via JSON Schema reflection).
   - Keep generated schemas deterministic and replaceable.
3. Update OpenAPI operations to reference `$ref` schemas:
   - Replace placeholder bodies with `$ref` for request + success response.

**Exit criteria**
- A majority of operations use `$ref` schemas for 2xx responses, and core endpoints used by Greater can be generated with strong typing.

### Phase 3 — Endpoint-by-endpoint completion (L2 → L3)

**Goal**: Finish typing and parameter modeling for *all* REST endpoints.

Work in domain order (each workstream ends with updating OpenAPI + passing verification):

1. **Setup + auth** (`/setup/*`, `/auth/wallet/*`, `/api/v1/auth/webauthn/*`)
2. **OAuth** (`/api/v1/apps`, `/oauth/*`) including form-encoded token requests and redirect semantics.
3. **Core Mastodon v1**: accounts, relationships, statuses, timelines, media, notifications, lists, conversations, preferences, markers, follow requests, scheduled statuses.
4. **Core Mastodon v2**: search, suggestions, filters, grouped notifications, trends.
5. **Discovery + extras**: directory, announcements, custom emojis, translations, exports/imports, reputation/vouches, moderation/reporting.
6. **Admin**: `/api/v1/admin/*` (accounts, reports, moderation, federation, domain blocks/allows, email domain blocks, custom emojis, announcements).
7. **Federation HTTP**: actor, outbox, inbox, objects, webfinger, nodeinfo; document ActivityPub JSON-LD shapes at least generically.
8. **Streaming SSE**: `text/event-stream` responses + query params + auth requirements.

**Exit criteria**
- All operations are L3 (no placeholder schemas, correct params, correct status codes, correct content types).

### Phase 4 — Strict enforcement + regression guardrails

**Goal**: Prevent drift and prevent “half-documented” endpoints from shipping.

1. Extend `tools/openapi` with a stricter verifier:
   - Fail if any operation still uses placeholder request/response schemas.
   - Fail if required security is missing.
   - Fail if required pagination/query params are missing for known paginated endpoints (configurable allowlist).
2. Add `make verify-openapi-strict` and eventually wire it into CI once L3 is reached.
3. Add a small “how to generate clients” doc for Greater (TS client generation + auth injection patterns).

**Exit criteria**
- Strict verification enabled and green.
- OpenAPI becomes the authoritative contract for REST integrations.

## Tracking + reporting

- Keep `docs/specs/openapi.yaml` as the canonical contract.
- Use `make verify-openapi` for drift and `make verify-openapi-strict` (later) for completeness.
- Maintain a running “coverage status” section in this doc (counts by L0/L1/L2/L3) once strict tooling exists.
