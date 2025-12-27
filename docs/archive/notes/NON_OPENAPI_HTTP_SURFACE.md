# Non-OpenAPI HTTP Surface (Registry)

> **Last updated**: 2025-12-26  
> **Purpose**: document HTTP entrypoints that are **not** described by `docs/specs/openapi.yaml` (which targets `cmd/api`).

## Sources of truth

- **OpenAPI (API Lambda only)**: `docs/specs/openapi.yaml` (generated; `make verify-openapi-strict`)
- **Lambda + route ownership**: `docs/specs/01-lambda-inventory-matrix.md` (generated; `make verify-inventory`)
- **Infra wiring**: `infra/cdk/constructs/api_routes.go`

## HTTP entrypoints (non-OpenAPI)

- **GraphQL**
  - `cmd/graphql/main.go`: `GET /api/graphql`, `POST /api/graphql`
  - Optional playground/subscriptions exist in code but are not currently inventory-routed.
- **Streaming (SSE)**
  - `cmd/sse/main.go`: `GET /api/v1/streaming/*`
- **ActivityPub / federation**
  - `cmd/actor/main.go`: `GET /users/{username}`
  - `cmd/inbox/main.go`: `GET /users/{username}/inbox`, `POST /users/{username}/inbox`
  - `cmd/outbox/main.go`: `GET /users/{username}/outbox`, `POST /users/{username}/outbox`
  - `cmd/objects/main.go`: `GET /objects/{id}`
  - `cmd/collections/main.go`: `GET /users/{username}/followers`, `GET /users/{username}/following`, `GET /users/{username}/liked`
  - `cmd/webfinger/main.go`: `GET /.well-known/webfinger`

## Notes

- **Protocol endpoints implemented in `cmd/api`** (e.g., NodeInfo) remain intentionally documented via OpenAPI *only insofar as they’re routed to the API Lambda*.
- For client generation, prefer GraphQL for app functionality and use REST only for explicitly REST-only flows (OAuth/OIDC, wallet + WebAuthn + setup) and protocol endpoints.

