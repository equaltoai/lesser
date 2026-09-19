# Account avatar contract

Status: source-backed contract for the account avatar upload/serve/clear surface. Lesser is the source of truth; clients
must not infer behavior from local mocks that differs from this contract.

## Source map

- Route/auth registration: `cmd/api/routes.go` (`POST`/`DELETE /api/v1/accounts/avatar`, `GET /api/v1/avatars/{id}`).
- HTTP handlers: `cmd/api/handlers/avatar.go`.
- Stored-object service: `pkg/services/media/avatar.go`.
- Account ownership record and dual-write: `pkg/services/accounts/avatar.go`, with the recorded id persisted as the
  `avatarId` attribute on the user row (`pkg/storage/models/user.go`, `pkg/storage/types.go`,
  `pkg/storage/repositories/account_repository.go`).
- Bucket lifecycle that keeps all served prefixes retrievable: `infra/cdk/constructs/s3_lifecycle.go` (see
  "Serving-path storage classes").
- Public-surface allowlist: `pkg/auth/publicsurface/publicsurface.go` (`/api/v1/avatars/`, single-segment GET/HEAD).
- Published OpenAPI: `docs/contracts/openapi.yaml`.
- Tests: `cmd/api/handlers/avatar_walkthrough_test.go`, `cmd/api/avatar_routes_test.go`,
  `pkg/services/media/avatar_test.go`, `pkg/services/accounts/avatar_test.go`,
  `infra/cdk/constructs/s3_lifecycle_test.go`.

## Endpoints

```http
POST   /api/v1/accounts/avatar   # authenticated, write scope, rate limited (10/hour)
DELETE /api/v1/accounts/avatar   # authenticated, write scope
GET    /api/v1/avatars/{id}      # public, unauthenticated
```

The avatar URL is written **only** by `POST /api/v1/accounts/avatar`. `PATCH /api/v1/accounts/update_credentials`
rejects an `avatar` parameter with `400` and a message pointing at the upload route; the same rejection is enforced at the
accounts service boundary, so no caller (including the streaming `update_profile` command) can set a profile avatar URL.

### `POST /api/v1/accounts/avatar`

`multipart/form-data` with one file part named `file` or `avatar`.

- Request content type must be one of `image/jpeg`, `image/png`, `image/gif`, `image/webp`. `image/svg+xml` is rejected
  even when declared as a raster type, and the declared type must match the sniffed bytes.
- Maximum upload size is 512 KiB (`media.AvatarMaxUploadBytes`).
- The handler stores the bytes under a freshly minted opaque UUID key (`avatars/<uuid>`) and then attaches the served
  URL to the account, writing `user.Avatar`, `user.AvatarID` and `Actor.Icon` in the same operation. No path sets the URL
  without the id; each upload mints a new id, so bytes never change under a key.
- Cleanup: the object this account previously recorded is deleted best-effort, using only the id stored on the caller's
  own row. A cleanup failure is logged and never fails the upload.
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

- Deletion is authorized by ownership, not by a URL. The object id is read from the `avatarId` value the authenticated
  principal's own row recorded at upload time (`accounts.recordedAvatarID`); the handler never parses `user.Avatar`.
- A row that records no id — every row written before the id was recorded — deletes nothing, even when its stored avatar
  URL is a well-formed `/api/v1/avatars/<uuid>` URL. A row whose recorded id is not a lowercase UUID likewise deletes
  nothing.
- A cleanup failure is logged and never fails the clear.
- Success: `200` with the updated Mastodon `Account` projection. An account with no avatar reports `avatar` and
  `avatar_static` as empty strings. On the storage-account REST projection path this contract covers, the read path never
  substitutes a placeholder image URL: an absent avatar is reported as absent, never as a synthesized value.
  Actor-transform paths outside this contract still substitute a `missing.png` placeholder when an actor carries no image
  (`pkg/transformations/converters.go`, `pkg/mastodon/transformers/transformers_mastodon.go`), reachable for remote or
  otherwise unresolved actors; that pre-existing sweep is owned by the fabrication-audit milestone, not by this contract.

### `GET /api/v1/avatars/{id}`

Public, unauthenticated, `GET`/`HEAD`. Reachable through the deployed public-surface middleware only via the
`/api/v1/avatars/` single-segment allowlist rule.

- The route serves **only** lesser-stored avatar objects. The S3 key is derived solely from a validated opaque
  lowercase UUID under `avatars/`; the route never proxies, never fetches a caller-supplied URL, and never redirects.
- Success: `200` with the raw stored bytes, `Content-Type` set to the stored type, `Cache-Control: public,
  max-age=31536000, immutable`, `Content-Length`, and `X-Content-Type-Options: nosniff`.
- Malformed ids, absent ids, and objects whose stored content type drifts out of the allowlist all return the same
  byte-identical JSON `404` envelope, so the response never reveals which case applies and an unservable object is never
  reported as a server error.
- `503` when the object store is unavailable.

## Serving-path storage classes

An avatar URL is served by reading the stored object on demand, so every `avatars/` object must stay in a storage class
S3 serves directly. The media bucket's lifecycle rules therefore never transition bucket content to `GLACIER` or
`DEEP_ARCHIVE`; the bucket-wide rule stops at `GLACIER_IR`. It is bucket-wide rather than `avatars/`-scoped because a
lifecycle rule's filter takes prefix, tag, and object-size predicates and has no negation: no single rule can cover the
bucket while exempting `avatars/`, and a prefix-scoped rule cannot exempt a prefix from a bucket-wide rule because S3
applies every matching rule. The same reasoning covers the bucket's other read-back prefixes (`media/`, `published/`,
`imports/`, `exports/`). See `infra/cdk/constructs/s3_lifecycle.go` and
`infra/cdk/constructs/s3_lifecycle_test.go`.

`infra/cdk` is a shared stack (`LesserSharedStack`); the lifecycle change is code-only and takes effect on the operator's
next deploy. Removing a transition does not move objects that already transitioned: where a deployed media bucket has
carried the removed 180-day `GLACIER` rule, objects older than 180 days under `media/`, `published/`, `imports/`, or
`exports/` are already archived and stay unreadable until they are restored and re-copied. The operator should check the
deployed bucket's object storage classes and run that one-time restore/re-copy as part of this deploy. `avatars/` is
unaffected: the prefix is new, so no object under it predates the fix.
