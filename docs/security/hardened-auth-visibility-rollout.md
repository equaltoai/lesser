# Hardened auth, visibility, and rollout semantics

This note summarizes the M11 operator contract for auth, visibility, and rollout validation. It is intentionally aligned
with the generated REST contract (`docs/contracts/openapi.yaml`) and generated GraphQL schema
(`docs/contracts/graphql-schema.graphql`).

## Auth contract

- REST write APIs and non-public GraphQL fields require OAuth bearer authentication.
- GraphQL anonymous reads are limited to the documented public-read subset and only expose public/unlisted content.
- Device-code authorization is disabled by default. When enabled, `client_class=cli` tokens are governed by the CLI
  automation rails in `docs/configuration.md` regardless of account type.
- Browser CORS is fail-closed by default to the instance origin unless the operator configures an explicit trusted origin
  allowlist.

## Visibility contract

- Public/unlisted objects may be returned by anonymous public-read surfaces.
- Private/followers/direct content requires an authenticated viewer and the same visibility and participant checks used
  by REST handlers.
- Reply and quote reach is non-widening: `public > unlisted > private/followers > direct`. An explicit child visibility
  wider than the parent status is rejected with the structured `UNPROCESSABLE_ENTITY` error; mutation inputs whose quote
  visibility is optional inherit the parent status visibility when omitted. Lesser never silently clamps an explicit
  author choice.
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

The release path remains `dev -> staging -> live` (with staging where the deployment uses it). Soak is evidence-based:
check API auth behavior, anonymous public-read visibility, direct-message visibility, federation delivery/receipt,
stream retry metrics, cost reporting, SQS DLQ depth, CloudWatch error rate, and operator unlock state before promotion.
Never set timeouts on CDK deploy commands and never delete the bootstrap mnemonic.
