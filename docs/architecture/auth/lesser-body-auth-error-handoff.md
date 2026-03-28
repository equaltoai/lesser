# Lesser-body Auth Error Handoff

This handoff defines what `lesser-body` must preserve when translating Lesser auth failures through `comm_errors.go`.

The source-of-truth client-visible contract lives in [auth-error-contract.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/architecture/auth/auth-error-contract.md).

## Translation rules

`lesser-body` should prefer pass-through. If it must wrap or translate, it must preserve all of the following:

1. HTTP status code
1. top-level `error`
1. top-level `error_description` when present
1. top-level `scope` for `insufficient_scope` responses

### Must-pass-through cases

| Lesser response | Why |
| --- | --- |
| `401` + `error=invalid_token` on `/api/*` | MCP clients treat this as the refresh bucket. |
| `403` + `error=insufficient_scope` on `/api/*` | MCP clients must not downgrade this into a refreshable error. |
| `400` + `error=invalid_grant` on `/oauth/token` | Signals refresh exhaustion or stale auth artifacts. |
| `401` + `error=invalid_client` on `/oauth/token` | Signals public client-auth configuration failure. |
| `429` + `error=slow_down` | Preserves backoff semantics and `Retry-After`. |

### Allowed wrapping rules

If `lesser-body` adds its own envelope, it must not erase the Lesser machine fields. Equivalent preserved fields are acceptable only if the final MCP client still receives:

- the original HTTP status
- the original Lesser `error`
- the original Lesser `error_description`

Preferred pattern: embed Lesser’s body unchanged and add transport metadata around it instead of renaming fields.

## Fixtures to copy into lesser-body tests

### API bearer refresh case

Status: `401`

```json
{
  "error": "invalid_token",
  "error_description": "invalid token"
}
```

Expected MCP action: `refresh`

### API scope failure

Status: `403`

```json
{
  "error": "insufficient_scope",
  "error_description": "insufficient scope: requires read",
  "scope": "read"
}
```

Expected MCP action: `fail`

### Refresh-token exhaustion

Status: `400`

```json
{
  "error": "invalid_grant",
  "error_description": "Invalid or expired refresh token"
}
```

Expected MCP action: `reauth`

### Client authentication mismatch

Status: `401`

```json
{
  "error": "invalid_client",
  "error_description": "Invalid client credentials"
}
```

Expected MCP action: `reconfigure`

### Rate-limited auth path

Status: `429`

```json
{
  "error": "slow_down",
  "error_description": "Too many dynamic client registration requests"
}
```

Expected MCP action: `backoff`

## `ErrTokenTooOld`

Current Lesser behavior after `#247`: no active runtime emission on protected bearer-token API routes.

If the code is ever reintroduced, the cross-repo compatibility requirement is:

- Lesser must emit `401` + `error=invalid_token`
- `lesser-body` must preserve that meaning as a refreshable failure
- `lesser-body` must not translate it into `403`, `invalid_grant`, or an opaque transport error

## Follow-up requirement outside this repo

Before the broader auth-consolidation track is considered complete, `lesser-body` should add translation tests that pin the fixtures above in `comm_errors.go`.

Tracked follow-up:

- `equaltoai/lesser-body#47` — preserve Lesser auth error contract in `comm_errors.go`
