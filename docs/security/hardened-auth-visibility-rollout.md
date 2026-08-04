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
  `UNPROCESSABLE_ENTITY` error. For the public or unlisted targets that remain quotable, GraphQL quote inputs whose
  visibility is omitted inherit the target visibility; the REST reblog-quote request retains its documented public
  default, which is therefore rejected when it would widen reach. These paths never silently clamp an explicit author
  choice.
- A quote target must be public or unlisted to be quotable at all. This invariant applies to all four creation paths:
  REST create-quote, REST boost-of-quote, GraphQL `createQuoteNote`, and GraphQL `createNote` with `quoteTargetID`. This is
  an intentional GraphQL behavior tightening: quoting a followers-only target now returns `BUSINESS_RULE_VIOLATED`
  rather than inheriting its reach. These paths resolve the target storage-first, fetch and materialize a canonical
  remote ActivityPub Note when absent locally, and then apply viewer-access and reach checks. Deleted or inaccessible
  targets remain indistinguishable from missing statuses. A fetched remote quote target may therefore remain persisted
  locally even when the requesting viewer is denied. This is intentional and mirrors reply-parent materialization
  (operator ruling 2026-08-01); persistence does not grant the denied viewer access. Quote Posts REST creation then uses
  `Notes.CreateNote` and `QuoteService.AttachQuoteToStatus`, the same persistence and federation path as GraphQL quote
  creation. Missing and invisible targets share the same **404 Not Found** response. Account-level and per-note quote
  controls are both enforced through `QuoteService.CheckQuotePermissions`; follower, mentioned, and policy-storage error
  arms fail closed.
- `GET /api/v1/statuses/{id}/quotes` verifies the target and every returned quote through the notes service's viewer-aware
  visibility predicate before applying the public `limit` and `offset` window. A missing and an invisible target share the
  same **404 Not Found** response.
- `PUT /api/v1/accounts/quote_permissions` updates only the authenticated account through the shared quote service and
  validates all policy arms. `GET /api/v1/accounts/{id}/quote_permissions` requires `read:accounts` and returns the raw
  policy only for that authenticated account. Other and missing account IDs share **404 Not Found**, preventing both a
  block-list disclosure and an account-existence oracle. The GraphQL account quote-permission read remains uniformly
  unavailable for self, other, and missing targets until its separate consumer contract is activated.
- Quote deletion deliberately returns the same **404 Not Found** response for a missing relationship and one owned by a
  different account, preventing ownership from becoming an existence oracle; successful owner deletion is unchanged.
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
