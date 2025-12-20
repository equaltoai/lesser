# Spec 03: API Gateway + Federation Routing (Correctness and Ownership)

## Summary
HTTP routing must match where handlers are implemented. CDK currently routes several federation paths to the wrong Lambda(s), and some Lambdas implement routes that are not routed to them. This spec defines canonical route ownership and alignment between API Gateway templates and Lift route registration.

## Goals
- Canonical ownership for federation endpoints (WebFinger, actor, collections, objects, inbox/outbox).
- CDK routes match actual handler registrations.
- Remove “phantom routes” (routed but not implemented) and “orphan routes” (implemented but never routed).

## Non-Goals
- Implementing new behavior beyond what is needed for routing correctness.
- Adding speculative endpoints without a storage/index design.

## Requirements
### R1 — Canonical ownership for federation endpoints
Minimum federation routing targets:
- `GET /.well-known/webfinger` → `webfinger`
- `GET /users/{username}` → `actor`
- `GET /users/{username}/followers` → `collections`
- `GET /users/{username}/following` → `collections`
- `GET /users/{username}/liked` → `collections`
- `GET /objects/{id}` → `objects`

Inbox/outbox must route to the lambdas that actually register those paths (see R2).

### R2 — Route shapes must match handler route registration
The handler’s Lift routes and API Gateway templates must agree on:
- path shape
- path parameter names
- which methods exist

If a handler registers `/inbox/{username}`, CDK must not only route `/users/{username}/inbox` (and vice versa).

### R3 — Catch-all routing is explicit and safe
The `ANY /{proxy+}` fallback can exist for the Mastodon API, but it must not mask missing federation endpoints. Federation routes must be explicitly routed and smoke-tested (Spec 07).

### R4 — `/activities/{id}` is either removed or fully implemented
If activity dereference by ID is not part of the product, remove the route. If it is required, define storage/indexing and implement it with tests.

## Current Drift Findings (Examples)
- `infra/cdk/constructs/api_routes.go` routes `GET /users/{username}` to the Mastodon API Lambda, but the federation actor handler is expected to live in `cmd/actor`.
- `infra/cdk/constructs/api_routes.go` routes `GET /objects/{id}` to the Mastodon API Lambda, but dereference is implemented by `cmd/objects` (`/objects/:id`).
- `infra/cdk/constructs/api_routes.go` includes `GET /activities/{id}` routed to the API Lambda, but there is no corresponding handler in the product set (should be removed unless designed/implemented).
- `cmd/collections` implements `/users/:username/followers`, `/following`, `/liked`, but CDK currently routes followers/following to the API Lambda.
- `cmd/inbox` registers `/inbox/{username}`, while CDK routes `/users/{username}/inbox` and `/inbox` (requires reconciliation and a single canonical path).

## Proposed Implementation
1. Define a route inventory for HTTP in the machine inventory (Spec 01).
2. Generate CDK routes from the route inventory rather than ad-hoc definitions.
3. Update federation lambdas to register routes they own (e.g., `actor` must register `/users/{username}`).
4. Add federation routing smoke tests (Spec 07).

## Acceptance Criteria
- Every CDK route maps to an implemented handler.
- Every implemented federation route is reachable via CDK.
- No unexpected reliance on the `ANY /{proxy+}` fallback for federation behavior.
