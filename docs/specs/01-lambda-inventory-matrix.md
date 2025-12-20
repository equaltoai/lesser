# Spec 01: Lambda Inventory Matrix (Product Source of Truth)

## Summary
Lesser’s “all 9’s” work requires eliminating drift between:
- `cmd/*` (what exists in code)
- `Makefile LAMBDAS` (what is packaged to `bin/*.zip`)
- CDK (what is deployed and wired)
- monitoring (what is observed)

This spec defines a single, canonical **Lambda inventory matrix** that becomes the source of truth for Specs 02–07.

## Goals
- Establish an authoritative list of product Lambdas and their responsibilities.
- Capture each Lambda’s trigger(s): HTTP routes, WebSockets, SQS queues, DynamoDB streams, schedules.
- Enable inventory-driven CDK generation and drift checks.

## Non-Goals
- Removing Lambdas or “core-only” deployment modes (entire product is required).
- Redesigning domain behavior beyond wiring/contract alignment.

## Canonical Product Set (Must Match `Makefile LAMBDAS`)
The inventory set is exactly the 36 Lambdas in `Makefile` `LAMBDAS`:

| Lambda | Type (target) | Primary triggers (target) | Notes |
|---|---|---|---|
| `activity-processor` | `processor-stream` | DynamoDB stream | Processes Activity/Object/Status changes. |
| `actor` | `api-http` | `GET /users/{username}` | Must register route(s) in handler and support ActivityPub negotiation. |
| `ai-processor` | `processor-stream` | DynamoDB stream | AI enrichment/classification. |
| `api` | `api-http` | `/api/v1/*`, `/api/v2/*`, OAuth/Auth, nodeinfo | Mastodon-compatible REST and some well-knowns. |
| `collections` | `api-http` | `/users/{username}/followers|following|liked` | ActivityPub collection endpoints. |
| `cost-aggregator` | `processor-stream` (or hybrid) | DynamoDB stream (and/or schedule) | Open question in Spec 04. |
| `dlq-processor` | `processor-sqs` + `processor-scheduled` | SQS + EventBridge schedule | DLQ handling + periodic sweeps. |
| `enhanced-federation-processor` | `processor-sqs` | SQS | Enhanced federation retry. |
| `export-generator` | `processor-sqs` | SQS | Export job processor. |
| `federation-aggregator` | `processor-scheduled` (and optional SQS) | EventBridge (and/or SQS) | Aggregation/cadence-driven metrics. |
| `federation-delivery` | `processor-sqs` | SQS | Outgoing federation delivery. |
| `federation-timeseries` | `processor-stream` | DynamoDB stream | Aggregates federation metrics/time windows. |
| `federation-tracker` | `processor-stream` | DynamoDB stream | Tracks federation relationships/health. |
| `graphql` | `api-http` | `GET/POST /api/graphql` | GraphQL over HTTP. |
| `graphql-ws` | `api-ws` | WebSocket API | GraphQL subscriptions over WS. |
| `import-processor` | `processor-sqs` | SQS | Import job processor. |
| `inbox` | `api-http` | `/inbox/{username}` (current) | CDK routes must match actual handler routes. |
| `media-processor` | `processor-sqs` | SQS | Media processing/transcoding. |
| `metrics-aggregator` | `processor-stream` | DynamoDB stream | Aggregates per-record cost/metrics signals. |
| `metrics-processor` | `processor-stream` | DynamoDB stream | Streams metrics extraction/normalization. |
| `ml-training-processor` | `processor-stream` | DynamoDB stream | ML training job lifecycle. |
| `moderation-processor` | `processor-stream` | DynamoDB stream | Moderation events/reviews/decisions. |
| `note-processor` | `processor-stream` | DynamoDB stream | Note/post processing pipeline. |
| `notification-processor` | `processor-sqs` | SQS | Notification generation/dispatch. |
| `objects` | `api-http` | `GET /objects/{id}` | ActivityPub object dereference. |
| `outbox` | `api-http` + `processor-sqs` | `/users/{username}/outbox` + SQS | Hybrid: serves outbox and may process queued federation/outbox work. |
| `push-delivery` | `processor-sqs` | SQS | Web push delivery worker. |
| `report-trust-updater` | `processor-stream` | DynamoDB stream | Updates trust from moderation/report signals. |
| `search-indexer` | `processor-stream` | DynamoDB stream | Search indexing updates. |
| `severance-processor` | `processor-stream` | DynamoDB stream | Federation severance detection. |
| `status-indexer` | `processor-stream` | DynamoDB stream | Status indexing updates. |
| `stream-router` | `processor-stream` | DynamoDB stream | Fanout/route streaming events. |
| `streaming` | `api-ws` | WebSocket API | Mastodon-style streaming. |
| `trend-aggregator` | `processor-scheduled` | EventBridge schedule | Trend aggregation. |
| `webfinger` | `api-http` | `GET /.well-known/webfinger` | WebFinger discovery. |
| `websocket-cost-aggregator` | `processor-scheduled` | EventBridge schedule | Periodic WS cost aggregation/cleanup. |

This table is a starting point. The final inventory must be precise enough to generate CDK wiring without inference.

## Requirements
### R1 — Inventory is exhaustive and exact
- The inventory includes all 36 Lambdas above (no missing, no extra).

### R2 — Each Lambda entry includes minimum metadata
For each Lambda, record:
- `name` (matches Makefile and zip name)
- `type` (`api-http`, `api-ws`, `processor-stream`, `processor-sqs`, `processor-scheduled`, `hybrid`)
- trigger details (routes, queue names, stream source, schedule cadence)
- required env vars (beyond baseline)
- role class (`basic` vs `encryption`)
- operational defaults (memory, timeout, log retention)

### R3 — Inventory is machine-consumable
Maintain a machine-readable inventory (Go/YAML/JSON) and generate (or validate) this Markdown view from it.

## Proposed Implementation
1. Add machine-readable inventory in `infra/cdk` (e.g., `infra/cdk/inventory/lambdas.go`).
2. Generate CDK functions and wiring from the inventory (Specs 02–06).
3. Add a drift check that enforces set equality between `Makefile LAMBDAS`, inventory, CDK, and monitoring (Spec 07).

## Acceptance Criteria
- The inventory is the declared source of truth for Lambdas/triggers.
- Inventory set matches `Makefile LAMBDAS` exactly.
- Every downstream spec references this inventory rather than re-listing Lambdas ad-hoc.

