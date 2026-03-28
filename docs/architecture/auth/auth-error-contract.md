# Auth Error Contract

Version: `2026-03-19`

This document is the Lesser-owned auth error contract for:

- Lesser bearer-token API responses
- Lesser OAuth token/authorization responses
- MCP clients and other machine clients deciding whether to refresh, re-authenticate, back off, or fail

The companion lesser-body translation handoff lives in [lesser-body-auth-error-handoff.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/architecture/auth/lesser-body-auth-error-handoff.md).

## Scope

This contract intentionally distinguishes two families:

1. `/oauth/*` token and authorization endpoints
1. authenticated `/api/*` routes rejecting bearer tokens

`/oauth/*` stays on OAuth-style `error` + `error_description`.
Authenticated `/api/*` routes use the bearer-token contract introduced by `#249`.

## Canonical client actions

| Action | Meaning |
| --- | --- |
| `refresh` | Try silent token refresh if the client has a refresh token or equivalent session state. |
| `reauth` | Start an interactive login / consent flow again. |
| `reconfigure` | Operator or client configuration must change before retrying. |
| `backoff` | Wait and retry later. |
| `retry` | Retry the same request because the failure is transient server-side. |
| `fail` | Surface the error without automatic retry. |

## Bearer-token API contract (`/api/*`)

### Body shape

Authenticated API routes that reject bearer tokens must return:

```json
{
  "error": "invalid_token",
  "error_description": "authentication required"
}
```

or:

```json
{
  "error": "insufficient_scope",
  "error_description": "insufficient scope: requires read",
  "scope": "read"
}
```

401 and 403 bearer failures also include a `WWW-Authenticate: Bearer ...` header.

### Status/body/action matrix

| Scenario | HTTP status | Body `error` | Client action | Notes |
| --- | --- | --- | --- | --- |
| Missing bearer token on protected `/api/*` route | `401` | `invalid_token` | `refresh` | If no refresh path exists, the client escalates to `reauth`. |
| Expired or otherwise invalid bearer token on protected `/api/*` route | `401` | `invalid_token` | `refresh` | This is the primary MCP retry bucket. |
| Authenticated request lacks required scope | `403` | `insufficient_scope` | `fail` | Requires a different grant/scope set; refresh does not help. |
| Auth-related rate limit on API or token acquisition path | `429` | `slow_down` | `backoff` | Honor `Retry-After` when present. |
| Transient auth infrastructure failure | `500` / `503` | implementation-specific | `retry` | Only for server-side faults, not credential failures. |

## OAuth token/authorization contract (`/oauth/*`)

### Status/body/action matrix

| Scenario | HTTP status | Body `error` | Client action | Notes |
| --- | --- | --- | --- | --- |
| Refresh token invalid, expired, revoked, or reused | `400` | `invalid_grant` | `reauth` | Silent refresh is exhausted. |
| Authorization code invalid, expired, mismatched, or PKCE check failed | `400` | `invalid_grant` | `reauth` | Restart the auth flow. |
| Client authentication failed | `401` | `invalid_client` | `reconfigure` | Usually bad or rotated client credentials. |
| Client not allowed to use requested grant | `400` | `unauthorized_client` | `reconfigure` | Connector registration or grant policy mismatch. |
| Requested scope is invalid | `400` | `invalid_scope` | `reconfigure` | Client requested a scope Lesser does not allow. |
| Polling or token issuance rate-limited | `429` | `slow_down` | `backoff` | Present on device-code polling and OAuth rate limiting. |
| Temporary storage or server failure | `500` / `503` | `server_error` | `retry` | Safe to retry with backoff. |

## Concrete examples

### Protected API request with a stale access token

Status: `401`

```json
{
  "error": "invalid_token",
  "error_description": "invalid token"
}
```

Client action: `refresh`

### Protected API request with missing `read` scope

Status: `403`

```json
{
  "error": "insufficient_scope",
  "error_description": "insufficient scope: requires read",
  "scope": "read"
}
```

Client action: `fail`

### Refresh token exchange after revocation or expiry

Status: `400`

```json
{
  "error": "invalid_grant",
  "error_description": "Invalid or expired refresh token"
}
```

Client action: `reauth`

### OAuth client secret mismatch

Status: `401`

```json
{
  "error": "invalid_client",
  "error_description": "Invalid client credentials"
}
```

Client action: `reconfigure`

## `ErrTokenTooOld`

`ErrTokenTooOld` no longer has an active bearer-token rejection path after the `#247` token-lifecycle changes.

The compatibility rule is still explicit:

- if `ErrTokenTooOld` ever reappears on a protected `/api/*` route, it MUST map to `401` + `error=invalid_token`
- client action for that path MUST be `refresh`
- it MUST NOT surface as `403`

If a future token-endpoint flow surfaces the same logical condition during refresh, the `/oauth/token` response must stay in the OAuth family and use the appropriate OAuth error code instead of inventing a third contract.
