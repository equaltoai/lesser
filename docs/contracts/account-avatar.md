# Account avatar contract

Status: source-backed contract for the account avatar upload/serve/clear surface. Lesser is the source of truth; clients
must not infer behavior from local mocks that differs from this contract.

## Source map

- Route/auth registration: `cmd/api/routes.go` (`POST`/`DELETE /api/v1/accounts/avatar`, `GET /api/v1/avatars/{id}`).
- HTTP handlers: `cmd/api/handlers/avatar.go`.
- Stored-object service: `pkg/services/media/avatar.go`.
- Account dual-write: `pkg/services/accounts/avatar.go`.
- Public-surface allowlist: `pkg/auth/publicsurface/publicsurface.go` (`/api/v1/avatars/`, single-segment GET/HEAD).
- Published OpenAPI: `docs/contracts/openapi.yaml`.
- Tests: `cmd/api/handlers/avatar_walkthrough_test.go`, `cmd/api/avatar_routes_test.go`,
  `pkg/services/media/avatar_test.go`, `pkg/services/accounts/avatar_test.go`.

## Endpoints

```http
POST   /api/v1/accounts/avatar   # authenticated, write scope, rate limited (10/hour)
DELETE /api/v1/accounts/avatar   # authenticated, write scope
GET    /api/v1/avatars/{id}      # public, unauthenticated
```

### `POST /api/v1/accounts/avatar`

`multipart/form-data` with one file part named `file` or `avatar`.

- Request content type must be one of `image/jpeg`, `image/png`, `image/gif`, `image/webp`. `image/svg+xml` is rejected
  even when declared as a raster type, and the declared type must match the sniffed bytes.
- Maximum upload size is 512 KiB (`media.AvatarMaxUploadBytes`).
- The handler stores the bytes under a freshly minted opaque UUID key (`avatars/<uuid>`) and then attaches the served
  URL to the account, writing **both** `user.Avatar` and `Actor.Icon` in the same operation. No path sets one without
  the other; each upload mints a new id, so bytes never change under a key.
- Success: `200` with the updated Mastodon `Account` projection; `avatar` and `avatar_static` are the served URL
  `https://<domain>/api/v1/avatars/<uuid>`.

Failure mapping:

| Condition | Status |
| --- | --- |
| missing/unparsable multipart body or boundary | `400` |
| no single non-empty `file`/`avatar` part | `422` |
| declared or sniffed type not allowlisted | `422` |
| upload exceeds 512 KiB | `413` |
| object store unavailable | `500` |
| any other storage/account failure | `500` |

Error bodies never echo internal error strings.

### `DELETE /api/v1/accounts/avatar`

Clears `user.Avatar` and `Actor.Icon` together, then best-effort deletes the orphaned stored object.

- The object id is derived only from a URL whose path is exactly
  `/api/v1/avatars/<valid-lowercase-uuid>` (`media.AvatarIDFromURL`). URLs that do not match — for example
  `/avatars/original/missing.png`, a media CDN URL, or an arbitrary external profile image — never trigger a delete.
- A cleanup failure is logged and never fails the clear.
- Success: `200` with the updated Mastodon `Account` projection. Cleared accounts fall back to the pre-existing
  `https://<domain>/avatars/original/missing.png` projection used by `applyMastodonProfile`.

### `GET /api/v1/avatars/{id}`

Public, unauthenticated, `GET`/`HEAD`. Reachable through the deployed public-surface middleware only via the
`/api/v1/avatars/` single-segment allowlist rule.

- The route serves **only** lesser-stored avatar objects. The S3 key is derived solely from a validated opaque
  lowercase UUID under `avatars/`; the route never proxies, never fetches a caller-supplied URL, and never redirects.
- Success: `200` with the raw stored bytes, `Content-Type` set to the stored type, `Cache-Control: public,
  max-age=31536000, immutable`, `Content-Length`, and `X-Content-Type-Options: nosniff`.
- Malformed and absent ids both return the standard JSON `404` envelope, so the response never reveals whether an id was
  malformed or merely missing.
- `503` when the object store is unavailable.
