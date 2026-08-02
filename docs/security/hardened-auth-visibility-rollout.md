# Hardened auth, visibility, and rollout semantics

This note summarizes the M11 operator contract for auth, visibility, and rollout validation. It is intentionally aligned
with the generated REST contract (`docs/contracts/openapi.yaml`) and generated GraphQL schema
(`docs/contracts/graphql-schema.graphql`).

## Auth contract

- REST write APIs and non-public GraphQL fields require OAuth bearer authentication.
- GraphQL anonymous reads are limited to the documented public-read subset and only expose public/unlisted content.
- GraphQL WebSocket authentication is rechecked when each `subscribe` operation starts. An authenticated connection whose
  JWT is expired receives `extensions.code=TOKEN_EXPIRED`; anonymous access to a gated operation remains
  `extensions.code=UNAUTHENTICATED`. Expiry is not rechecked continuously or while delivering an already-established
  subscription, so token expiry does not tear down an in-flight operation.
- Authenticated WebSocket connection rows without a persisted `token_expires_at` fail closed on their next operation and
  receive `TOKEN_EXPIRED`, forcing one reconnect/re-authentication. This includes rows created before expiry persistence
  was introduced; anonymous rows remain unaffected.
- Device-code authorization is disabled by default. When enabled, `client_class=cli` tokens are governed by the CLI
  automation rails in `docs/configuration.md` regardless of account type.
- Browser CORS is fail-closed by default to the instance origin unless the operator configures an explicit trusted origin
  allowlist.

## Visibility contract

- Public/unlisted objects may be returned by anonymous public-read surfaces.
- Private/followers/direct content requires an authenticated viewer and the same visibility and participant checks used
  by REST handlers.
- Reply and quote reach is ordered `public < unlisted < private/followers < direct`, from widest to narrowest audience.
  REST status replies, GraphQL note/quote mutations, and `POST /api/v1/statuses/{id}/reblog` quote requests with a
  non-empty `comment` reject a child visibility wider than the referenced status with the structured
  `UNPROCESSABLE_ENTITY` error. GraphQL quote inputs whose visibility is omitted inherit the target visibility; the REST
  reblog-quote request retains its documented public default, which is therefore rejected when it would widen reach.
  These paths never silently clamp an explicit author choice.
- GraphQL quote mutations and REST reblog-quotes resolve the target storage-first, fetch and materialize a canonical
  remote ActivityPub Note when absent locally, and then apply viewer-access and reach checks. Deleted or inaccessible
  targets remain indistinguishable from missing statuses. A fetched remote quote target may therefore remain persisted
  locally even when the requesting viewer is denied. This is intentional and mirrors reply-parent materialization
  (operator ruling 2026-08-01); persistence does not grant the denied viewer access. The separate lesser-exclusive
  `POST /api/v1/statuses/{id}/quote` extension returns **501 Not Implemented** before target lookup pending a real
  authorization-, persistence-, and federation-aware implementation; it is not the Mastodon-compatible reblog-quote
  creation path described above and does not disclose whether a target status exists. The companion
  `GET /api/v1/statuses/{id}/quotes` extension likewise returns **501 Not Implemented** after parameter validation and
  before storage access so neither target existence nor a real quote-row count is exposed. Authenticated
  `PUT /api/v1/accounts/quote_permissions` returns **501 Not Implemented** after authentication and body validation
  until permission persistence exists; it never echoes request values as though they were saved. The
  `GET /api/v1/accounts/{id}/quote_permissions` extension requires bearer authentication, so anonymous callers receive
  **401 Unauthorized**. Its handler performs no additional per-target authorization and returns **501 Not Implemented**
  after path-parameter validation and before storage access for existent, missing, and hostile-text account IDs alike;
  the route never fabricates all-permissive settings.
- The REST account quote-permission preference surfaces are deliberately neither settable nor readable: their `PUT` and
  `GET` handlers remain **501 Not Implemented** pending persistence. `createQuoteBoostLift` marks the future enforcement
  hook at its `Quoteable: true` assignment; that hook must consult `QuotePermissions.IsAllowed` when storage lands.
- `UpdateStatus` does not accept or propagate a visibility field, so an existing status cannot be widened by editing it.
- Direct messages are 1:1 in v1. `POST /api/v1/statuses` with `visibility=direct` must include exactly one resolvable
  local or remote `@mention`; group DMs are not accepted.
- Once a direct message is stored, ActivityPub addressing fields (`to`, `cc`, `bto`, `bcc` and their stored status
  projections) are the authoritative recipient source. Content mentions are not authorization and are not sufficient for
  repair/backfill participant recovery.
- DM conversation backfill must derive participant sets from the stored addressing fields. Local actor URLs normalize to
  local usernames when they share the author host; remote actor URLs remain actor URIs for typed conversation lookup.

## Operational rollout checks

Before promoting a release beyond `dev`, validate the behaviors that changed in M11:

1. Generated VAPID keys are stored as base64url raw P-256 scalar private keys and push JWT signing succeeds.
2. Federation strongest-edge reads return concrete edges for all-type queries, and federation timeseries windows
   aggregate counters instead of overwriting prior samples.
3. Stream-router record processing returns failed-record errors so Lambda/DynamoDB Streams can retry side effects.
4. API transformations emit instance-correct URLs from the shared base-url context.
5. Cost date-range reads honor both start and end dates.
6. Lambda-optimized TableTheory clients retain the configured timeout safety buffer when a Lambda context deadline is
   applied.
7. DM conversation backfill ignores message-text mentions and uses stored recipient fields.
8. A pre-expiry-persistence authenticated GraphQL WebSocket row is refused with `TOKEN_EXPIRED` on its next subscribe
   operation, while a newly authenticated row with an unexpired JWT can subscribe and anonymous public subscriptions are
   unchanged.
9. REST and GraphQL quotes can materialize a previously unseen remote target, reject inaccessible targets, and reject
   public quotes of private/followers/direct targets without changing empty-comment reblog behavior.

The release path remains `dev -> staging -> live` (with staging where the deployment uses it). Soak is evidence-based:
check API auth behavior, anonymous public-read visibility, direct-message visibility, federation delivery/receipt,
stream retry metrics, cost reporting, SQS DLQ depth, CloudWatch error rate, and operator unlock state before promotion.
Never set timeouts on CDK deploy commands and never delete the bootstrap mnemonic.
