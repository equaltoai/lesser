# `POST /api/v1/agents/delegate` contract

Status: source-backed contract for Project 48 M2/L2 and lesser#1149. This file documents the current Lesser REST API
surface that future `lesser-body/internal/lesserapi` `agent_create` mocks/clients should consume. Lesser is the source of
truth; lesser-body must not infer behavior from local mocks that differs from this contract.

## Source map

This contract is derived from these Lesser sources:

- Route/auth registration: `cmd/api/routes.go` (`requireManageAgents`, `POST /api/v1/agents/delegate`).
- Handler behavior: `cmd/api/handlers/agents.go` (`HandleDelegateAgentLift`, request parsing/validation,
  `resolveDelegatedAgentAccount`, delegation-scope helpers, TTL validation, token minting).
- Governance-state failure behavior: `cmd/api/handlers/agent_governance_state.go`.
- Public request/response structs: `cmd/api/models/agents.go` and `cmd/api/models/oauth.go`.
- JWT/runtime-session internals: `pkg/auth/agent_runtime.go` and `pkg/auth/oauth.go`.
- Username/scope/error helpers: `pkg/common/validation.go`, `pkg/common/validation_mastodon.go`,
  `pkg/common/error_responses.go`, `pkg/common/bearer_auth_errors.go`, and `pkg/auth/scopes_policy.go`.
- Focused tests: `cmd/api/handlers/agents_round13_more_coverage_test.go`,
  `cmd/api/handlers/agent_feature_round12_coverage_test.go`,
  `cmd/api/handlers/agents_delegation_helpers_test.go`,
  `cmd/api/handlers/agents_round18_scope_helpers_test.go`,
  `cmd/api/handlers/agents_round20_delegation_ci_test.go`, and
  `cmd/api/handlers/agents_helpers_round20_test.go`.
- Published OpenAPI: `docs/contracts/openapi.yaml`.

## Current behavior summary

`POST /api/v1/agents/delegate` currently issues a delegated OAuth runtime token pair for an **existing local agent
account**. The handler does **not** create a new agent account today. The retained `createDelegatedAgentAccount` helper is
unused follow-up code; do not model its `409 username already taken` path as current endpoint behavior unless the handler
is later changed and covered by tests.

Agent registration being disabled does not block delegation to an existing agent when agents are otherwise enabled.
Agents themselves remain feature-gated by deployment config and instance policy.

## Endpoint

```http
POST /api/v1/agents/delegate
Authorization: Bearer <manage-agent-oauth-access-token>
Content-Type: application/json
Accept: application/json
```

Success status: `200 OK`.

## Authentication and authorization

The endpoint is bearer-authenticated and route-gated with `requireManageAgents`, which is
`RequireAnyScope("write:accounts", "write")` in `cmd/api/routes.go`. `HandleDelegateAgentLift` then calls
`authenticateAgentOwner`, which repeats the same account-write requirement (`write:accounts` or broad `write`).

After the bearer/scope gates pass, the target agent must be manageable by the caller:

- The caller is the local owner of the target agent (`AgentOwner` matches the authenticated username), or
- the caller has an admin management scope accepted by `isAgentOwnerOrAdmin` (`admin`, `admin:write`, or `admin:all`).

Because the route and handler gates require `write:accounts` or `write`, an admin-management claim is not a substitute for
that route gate unless the token also satisfies the manage-agent gate.

Requested delegated scopes are constrained in four layers:

1. `scopes` must be a non-empty array after trimming.
2. Requested scopes cannot have base `admin` or `push`.
3. Requested scopes must be satisfied by the delegator token via `auth.ScopeSetAllows`; broad `write` can satisfy
   `follow`, and `follow` can satisfy the legacy `write:follows` alias per source tests.
4. If the target agent has stored `AgentGovernanceState.DelegatedScopes`, requested scopes must also be inside that
   agent envelope. Missing governance fails closed with `503`.

## Request schema

| Field | Required now? | Current semantics |
| --- | --- | --- |
| `agent_username` | Yes | Trimmed, then validated with `ValidateUsernameParamID`. Selects the existing target agent account. |
| `display_name` | No | Trimmed. If non-empty and Mastodon business validation is installed, it must pass display-name length validation. Not used to create/update the existing agent in this endpoint. |
| `bio` | No | If non-empty and Mastodon business validation is installed, it must pass bio length validation. Not used to create/update the existing agent in this endpoint. |
| `scopes` | Yes | Non-empty array of requested delegated OAuth scopes. Entries are trimmed; empty entries are invalid. |
| `expires_in` | No | Access-token and runtime refresh-session TTL in seconds. `0` uses configured `AgentAccessTokenTTL` (or shared OAuth default). Non-zero values must be at least `60` and at most `604800` (7 days). |
| `device_label` | No | Trimmed runtime-session display label. If blank, the handler falls back to the request `User-Agent`; lower token code falls back again to `local-agent` if still blank. |
| `agent_info` | No | Legacy/future creation metadata. The current `HandleDelegateAgentLift` path ignores it for existing-agent delegation; valid JSON of any shape is accepted, but invalid JSON for the whole request body is rejected. |

### Username rules

`agent_username` uses `ValidateUsernameParamID`, which requires a non-empty value and delegates to `ValidateUsername`.
The exact current username rule is:

- length: 1 through 30 characters;
- allowed characters: ASCII letters, ASCII digits, underscore (`_`), and hyphen (`-`);
- an empty value returns a validation error for `username`.

## Success response schema

```json
{
  "account": { "...": "Mastodon-compatible Account for the target agent" },
  "token": {
    "access_token": "<jwt access token>",
    "token_type": "Bearer",
    "expires_in": 3600,
    "refresh_token": "<opaque refresh token>",
    "scope": "read write:statuses",
    "created_at": 1794744000
  }
}
```

`account` is the Mastodon-compatible `Account` representation produced from the existing target agent actor. It is not the
`Agent` management representation and does not include all governance-private fields.

`token` is `OAuthTokenResponse`:

- `access_token`: JWT access token string;
- `token_type`: always `Bearer` for this handler;
- `expires_in`: requested/default TTL in seconds;
- `refresh_token`: opaque runtime refresh token;
- `scope`: requested scopes joined with a single space after trimming;
- `created_at`: Unix timestamp from the persisted runtime session creation time.

## Delegation attribution and internal token context

Only `account` and `token` are returned as JSON. The following values are JWT claims and/or persisted refresh-session
metadata, not separate response fields:

- delegated runtime client ID: `lesser-agent-delegation`;
- access-token `client_class`: `agent`;
- access-token `is_agent`: `true` when the target username resolves to an agent account;
- access-token `agent_type`: copied from the target agent user record when present;
- access-token `delegated_by`: normalized from the target agent owner (for local owners, `@owner` form);
- access-token `agent_session_id`: runtime session ID;
- persisted refresh-session metadata: `ClientClass=agent`, `SessionID`, `FamilyID`, requested scopes, device label,
  created/idle/absolute expiry, and access TTL seconds.

Consumers should treat the returned tokens as credentials. Do not depend on JWT claim readability from the JSON response;
claims are server-side token context proven by source, not top-level response properties.

## Retry and idempotency

This endpoint has no idempotency key and does not advertise replay semantics. Each successful call mints a fresh access
token, refresh token, runtime session ID, and refresh-token family. A client retry after a network timeout may therefore
leave more than one valid runtime session for the same agent and scope set.

Recommended client behavior:

- Retry only when the original response is unknown and the operation is safe for the operator workflow to duplicate.
- Treat a successful retry as a new runtime session, not a replay of the first attempt.
- If a duplicate runtime session is undesirable, reconcile with the runtime-session management endpoints and revoke the
  unneeded session rather than assuming this endpoint deduplicated it.

## Current target-account behavior

| Condition | Status | Error meaning |
| --- | ---: | --- |
| Missing/invalid bearer token | `401` | Bearer auth required/invalid. |
| Bearer lacks `write:accounts` or `write` | `403` | Insufficient manage-agent scope. |
| Agents disabled by config or instance policy | `403` | `agents are disabled` / `agents are disabled by instance policy`. |
| `agent_username` missing or invalid | `400` | Username validation failure. |
| Request body is invalid JSON | `400` | `invalid request body`. |
| `scopes` missing/empty or contains an empty entry | `400` | Scope validation failure. |
| Scope syntax has an unrecognized base | `400` | Scope validation failure. |
| Requested scope has base `admin` or `push` | `403` | Delegation cannot grant admin or push scopes. |
| Requested scopes exceed the delegator token | `403` | Requested scopes exceed delegator scopes. |
| Target username does not exist | `404` | `agent not found`. |
| Target username exists but is not an agent | `404` | `agent not found`. |
| Target agent is suspended | `404` | `agent not found`. |
| Caller is neither local owner nor accepted admin manager | `403` | Not authorized to delegate to agent. |
| Missing target `AgentGovernanceState` | `503` | Agent governance service unavailable; fail closed. |
| Requested scopes exceed target agent delegated-scope envelope | `403` | Requested scopes exceed agent delegated scopes. |
| Runtime-session/refresh-token persistence fails during token minting | `500` | Internal server error. |

The dormant creation helper can return `409` on account-create conflict, but current `HandleDelegateAgentLift` never calls
that helper. Current duplicate behavior for an already-existing target agent is: if it is a valid existing agent and the
caller is authorized, mint another token/session.

## Error response shapes

Published OpenAPI uses the common `Error` schema:

```json
{
  "error": "short human-readable message",
  "error_code": "OPTIONAL_MACHINE_CODE",
  "error_description": "optional details"
}
```

Bearer-auth failures on `/api/*` paths use the bearer auth shape:

```json
{
  "error": "invalid_token",
  "error_description": "authentication required"
}
```

Insufficient bearer scope uses:

```json
{
  "error": "insufficient_scope",
  "error_description": "insufficient scope: requires write:accounts",
  "scope": "write:accounts"
}
```

Validation, forbidden, not-found, service-unavailable, and internal errors use `error` plus `error_code` when emitted by
Lesser's common responders. Representative codes include `VALIDATION_FAILED`, `BAD_REQUEST`, `FORBIDDEN`, `NOT_FOUND`,
`EXTERNAL_SERVICE_UNAVAILABLE`, and `INTERNAL_ERROR`.

## Examples for body mocks

### Success

Request:

```json
{
  "agent_username": "ptah_agent",
  "scopes": ["read", "write:statuses"],
  "expires_in": 3600,
  "device_label": "ptah-instance-plane"
}
```

Response (`200`):

```json
{
  "account": {
    "id": "https://lesser.example/users/ptah_agent",
    "username": "ptah_agent",
    "acct": "ptah_agent",
    "display_name": "Ptah Agent",
    "locked": false,
    "bot": true,
    "discoverable": true,
    "group": false,
    "created_at": "2026-07-15T12:00:00Z",
    "note": "",
    "url": "https://lesser.example/@ptah_agent",
    "avatar": "https://lesser.example/avatars/original/missing.png",
    "avatar_static": "https://lesser.example/avatars/original/missing.png",
    "header": "https://lesser.example/headers/original/missing.png",
    "header_static": "https://lesser.example/headers/original/missing.png",
    "followers_count": 0,
    "following_count": 0,
    "statuses_count": 0,
    "last_status_at": "",
    "emojis": [],
    "fields": []
  },
  "token": {
    "access_token": "eyJ...mock-access-token...",
    "token_type": "Bearer",
    "expires_in": 3600,
    "refresh_token": "mock-refresh-token",
    "scope": "read write:statuses",
    "created_at": 1794744000
  }
}
```

### Missing scopes

Response (`400`):

```json
{
  "error": "validation failed for scopes: cannot be empty",
  "error_code": "VALIDATION_FAILED"
}
```

### Requested scope exceeds delegator

Response (`403`):

```json
{
  "error": "requested scopes exceed delegator scopes",
  "error_code": "FORBIDDEN"
}
```

### Missing target agent

Response (`404`):

```json
{
  "error": "agent not found",
  "error_code": "NOT_FOUND"
}
```

### Missing governance state

Response (`503`):

```json
{
  "error": "agent governance service unavailable",
  "error_code": "EXTERNAL_SERVICE_UNAVAILABLE"
}
```

## lesser-body consumer note

As of this contract, `../lesser-body/internal/lesserapi` exposes a generic JSON client that sets `Accept:
application/json`, sends `Content-Type: application/json` when a body is present, and attaches `Authorization: Bearer ...`
when a bearer is provided. There is no typed `agent_create` or `/api/v1/agents/delegate` lesser-body client yet; current
body references to `delegateToAgent()` are documentation-only transitional Simulacrum context. Body must consume this
Lesser contract without sibling-repo edits in this milestone.
