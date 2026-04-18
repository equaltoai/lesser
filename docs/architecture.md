# Architecture Overview

Lesser is a serverless ActivityPub implementation built around a small set of AWS primitives:

- CloudFront + Route53 for the stage apex domain (and path routing)
- AWS Lambda for all compute (HTTP, WebSocket, stream and queue processors)
- DynamoDB single-table storage (plus GSIs for access patterns)
- S3 for media and static assets
- SQS + DynamoDB Streams for async fanout/processing

This page is intentionally high-level; deep dives live under `docs/architecture/`.

## “What runs where?” (inventory)

The canonical Lambda inventory (names, triggers, defaults) is generated here:

- `docs/specs/01-lambda-inventory-matrix.md`

If you need to understand a specific Lambda’s ownership or trigger wiring, start with that matrix.

## Top-level request surfaces

### REST API (Mastodon-compatible)

- Stage apex domain: `https://<stage-domain>`
- Primary paths: `/api/v1/*`, `/api/v2/*`
- Implementation: `cmd/api`
- Contract: `docs/contracts/openapi.yaml`

### GraphQL

- Endpoints: `/api/graphql` and `/graphql`
- Implementation: `cmd/graphql` (+ `cmd/graphql-ws` for subscriptions)
- Schema: `docs/contracts/graphql-schema.graphql` (generated from `graph/*.graphql`)

### Streaming

- SSE: `/api/v1/streaming/*` (`cmd/sse`)
- WebSocket: API Gateway WebSockets on a `ws.` custom domain (`cmd/streaming`)

### Federation (ActivityPub)

- WebFinger: `/.well-known/webfinger` (`cmd/webfinger`)
- Actor/object/collections: `cmd/actor`, `cmd/objects`, `cmd/collections`
- Inbox/outbox: `cmd/inbox`, `cmd/outbox`

### Soul + managed-agent integration

- Lesser-owned soul proof/discovery: `/.well-known/lesser-soul-agent` (`cmd/api`)
- MCP routes fronted through Lesser when `bodyEnabled` / legacy `soulEnabled` wiring is enabled:
  - `/mcp`
  - `/mcp/{actor}`
  - `/.well-known/mcp.json`
  - `/.well-known/oauth-protected-resource/mcp/{actor}`
- Runtime owner of those MCP routes: `lesser-body`
- Canonical boundary doc: `docs/soul.md`

## Data layer

### DynamoDB

Lesser uses a single-table design. Access-pattern guidance and GSI usage live here:

- `docs/architecture/dynamodb/gsi_usage_guide.md`
- `docs/architecture/dynamodb/dynamodb_index_registry.md`

### Media

Media is stored in S3 and served via CloudFront (provisioned by CDK in `infra/cdk/stacks/`).

## Async pipelines

Many operations write to DynamoDB and trigger downstream work via:

- DynamoDB Streams (fanout/indexing/aggregation)
- SQS queues (delivery, import/export, processors)
- EventBridge schedules (periodic aggregation/maintenance)

These pipelines keep interactive HTTP requests fast and make delivery/retries explicit.

## Deep dives

- Auth: `docs/architecture/auth/`
- DynamoDB: `docs/architecture/dynamodb/`
- CMS: `docs/architecture/cms/`
- Moderation/ML: `docs/architecture/moderation/`

## Operational References

- Deployment flow: `docs/deployment.md`
- Configuration knobs: `docs/configuration.md`
- Cost levers: `docs/cost-optimization.md`
- Logs + runbooks: `docs/operations/README.md`
