# Auth Error Audit

This audit captures the auth-related response shapes that existed on `main` before the `#249` normalization work.

## Summary

The pre-normalization codebase served auth failures through three incompatible families:

| Family | Representative paths | Statuses | Shape on `main` |
| --- | --- | --- | --- |
| OAuth token/authorization endpoints | `cmd/api/handlers/oauth.go`, `cmd/api/handlers/oauth_revoke.go` | `400`, `401`, `403`, `429`, `503` | `{"error":"<oauth_code>","error_description":"..."}` |
| Shared API auth helpers | `pkg/common/error_responses.go`, `cmd/api/public_surface_middleware.go`, bearer-protected `/api/*` handlers | `401`, `403` | `{"error":"...","error_code":"..."}` with optional `error_description` only on selected helpers |
| Ad hoc bearer-token handlers | `cmd/api/handlers/ai.go`, `cmd/api/handlers/discovery.go`, `cmd/api/handlers/debug.go` | `401`, `403` | `{"error":"..."}` only |

That split meant MCP-facing clients could see different machine fields for the same bearer-token failure depending on which endpoint rejected the token.

## Inventory

### OAuth-spec responses

These already used OAuth-style fields and were not the source of the collision:

| Code path | Typical failure | Status | Shape |
| --- | --- | --- | --- |
| `cmd/api/handlers/oauth.go` | invalid grant, invalid client, invalid scope, `slow_down` | `400` / `401` / `403` / `429` | `error`, `error_description` |
| `cmd/api/handlers/oauth_revoke.go` | invalid request, server error | `400` / `503` | `error`, `error_description` |
| `pkg/ratelimit/helpers.go` on `/oauth/token` and `/oauth/register` | rate limited auth bootstrap | `429` | `error`, `error_description`, plus rate-limit metadata |

### Shared bearer-token API helpers

These powered most authenticated `/api/*` routes, but they still used the legacy Lesser error contract:

| Helper | Typical callers | Status | Shape on `main` |
| --- | --- | --- | --- |
| `pkg/common.RespondMissingAuth()` | `cmd/api/public_surface_middleware.go`, `cmd/api/handlers/misc.go`, `cmd/api/handlers/moderation.go` | `401` | `error`, `error_code` |
| `pkg/common.RespondInvalidToken()` | limited direct usage | `401` | `error`, `error_code` |
| `pkg/common.RespondExpiredToken()` | limited direct usage | `401` | `error`, `error_code` |
| `pkg/common.RespondInsufficientScope()` | `cmd/api/handlers/exports.go`, `cmd/api/handlers/follow_requests.go`, `cmd/api/handlers/agent_governance.go` | `403` | `error`, `error_code` |
| `pkg/common.RespondUnauthorized()` | many `/api/*` handlers returning bearer auth failures | `401` | `error`, `error_code` |

### Ad hoc bearer-token emitters

These bypassed the shared helpers entirely and emitted their own JSON:

| Code path | Failure class | Status | Shape on `main` |
| --- | --- | --- | --- |
| `cmd/api/handlers/ai.go` | missing token, invalid token, missing moderation scope | `401` / `403` | `{"error":"authentication required"}`, `{"error":"invalid token"}`, `{"error":"moderation scope required"}` |
| `cmd/api/handlers/discovery.go` | missing token, invalid token | `401` | `{"error":"authentication required"}`, `{"error":"invalid token"}` |
| `cmd/api/handlers/debug.go` | missing token, invalid token, missing admin/debug scope | `401` / `403` | `{"error":"unauthorized"}`, `{"error":"admin or debug scope required"}` |

### Auth-adjacent but out of bearer-token scope

The following paths also use custom auth-looking payloads, but they are not bearer-token API rejections and therefore were left out of the first normalization pass:

| Code path | Why excluded |
| --- | --- |
| `cmd/api/handlers/recovery_emailfree.go` | recovery-token / session bootstrap, not bearer-token API auth |
| `cmd/api/handlers/wallet.go` | wallet challenge and signature workflow, not bearer-token API auth |
| `cmd/api/handlers/webauthn.go` | passkey registration/login flow, not bearer-token API auth |
| `cmd/api/handlers/setup.go` | setup-session and bootstrap auth, not MCP bearer-token traffic |

## Classification by client meaning

| Meaning | Pre-normalization emitters | Notes |
| --- | --- | --- |
| Missing bearer auth | `RespondMissingAuth()`, `RespondUnauthorized()`, ad hoc `{"error":"authentication required"}` | Same client action as invalid/expired bearer token for MCP refresh logic |
| Invalid or expired bearer token | `RespondUnauthorized()`, ad hoc `{"error":"invalid token"}` | No stable machine field distinguished this from missing auth |
| Insufficient scope | `RespondInsufficientScope()`, ad hoc `{"error":"moderation scope required"}` | Stable status `403`, unstable body |
| Auth/token-path rate limiting | `/oauth/token`, `/oauth/register` via `pkg/ratelimit/helpers.go` | Already OAuth-friendly with `slow_down` |
| OAuth refresh / re-auth failures | `cmd/api/handlers/oauth.go` | Already emitted RFC-style errors |

## `ErrTokenTooOld`

After the `#247` token-lifecycle work, the `ErrTokenTooOld` symbol still exists in `pkg/auth/errors.go`, but this audit found no active runtime emitter in `pkg/auth`, `pkg/common`, or `cmd/api`. The compatibility contract still needs an explicit rule for it so the error cannot silently reappear with an undefined HTTP status.

## Migration targets for `#256`

The normalization work should:

1. Keep `/oauth/*` endpoints on OAuth-style `error` + `error_description`.
1. Move bearer-token `/api/*` failures onto one RFC-friendly contract with explicit `401` / `403` / `429` meaning.
1. Route `cmd/api/public_surface_middleware.go`, `cmd/api/handlers/handler.go`, and the ad hoc bearer handlers in `ai.go`, `discovery.go`, and `debug.go` through shared helpers.
1. Leave non-bearer auth flows alone unless they later opt into the same contract deliberately.
