# Spec 01: Lambda Inventory Matrix (Product Source of Truth)

## Summary
Lesser’s “all 9’s” work requires eliminating drift between:
- `cmd/*` (what exists in code)
- the repo’s Lambda packaging set (what is packaged to `bin/*.zip`)
- CDK (what is deployed and wired)
- monitoring (what is observed)

A lightweight verifier (`scripts/verify_lambda_set.sh`) checks the packaging set against `cmd/*` and any built
`bin/<name>.zip` artifacts to flag set drift early.

Run `./lesser verify inventory` to assert the packaging set matches `infra/cdk/inventory/LambdaInventory` and to ensure
this doc is regenerated from the inventory generator.

This spec defines a single, canonical **Lambda inventory matrix** that becomes the source of truth for Specs 02–07.

## Goals
- Establish an authoritative list of product Lambdas and their responsibilities.
- Capture each Lambda’s trigger(s): HTTP routes, WebSockets, SQS queues, DynamoDB streams, schedules.
- Enable inventory-driven CDK generation and drift checks.

## Non-Goals
- Removing Lambdas or “core-only” deployment modes (entire product is required).
- Redesigning domain behavior beyond wiring/contract alignment.

## Canonical Product Set (Must Match the Repo Lambda Set)
The inventory set is enforced by:

```bash
./lesser verify lambda-set
./lesser verify inventory
```

<!-- INVENTORY_TABLE_START -->

> This table is generated from `infra/cdk/inventory/LambdaInventory` via `./lesser generate inventory`. Do not edit the table manually; update the inventory and re-run the generator.

| Name | Type | Triggers | Required Env Vars | Role | Operational Defaults |
| --- | --- | --- | --- | --- | --- |
| activity-processor | processor-stream | Stream: table=main-table; start=TRIM_HORIZON; batch=25; window=5s; parallel=5; maxRetry=3; bisect=true; reportBatchItemFailures=true | — | basic | memory=1024MB; timeout=300s; logs=7d |
| actor | api-http | HTTP: GET /users/{username} | — | encryption | memory=512MB; timeout=30s; logs=7d |
| ai-processor | processor-stream | Stream: table=main-table; start=TRIM_HORIZON; batch=25; window=5s; parallel=2; maxRetry=3; bisect=true; reportBatchItemFailures=true | — | basic | memory=1024MB; timeout=300s; logs=7d |
| api | api-http | HTTP: ANY /api/v1/{proxy+}<br>HTTP: ANY /api/v2/{proxy+}<br>HTTP: GET /.well-known/nodeinfo | — | encryption | memory=512MB; timeout=30s; logs=7d |
| cms-scheduler | processor-scheduled | Schedule: expression=rate(1 minute) | — | basic | memory=512MB; timeout=300s; logs=7d |
| collections | api-http | HTTP: GET /users/{username}/followers<br>HTTP: GET /users/{username}/following<br>HTTP: GET /users/{username}/liked | — | encryption | memory=512MB; timeout=30s; logs=7d |
| cost-aggregator | processor-stream | Stream: table=main-table; start=LATEST; batch=10; window=2s; parallel=1; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| dlq-processor | hybrid | SQS: queue=enhanced-federation-queue; consume=dlq; batch=10; window=1s<br>SQS: queue=export-processor-queue; consume=dlq; batch=10; window=1s<br>SQS: queue=federation-aggregator-queue; consume=dlq; batch=10; window=1s<br>SQS: queue=federation-delivery-queue; consume=dlq; batch=10; window=1s<br>SQS: queue=import-processor-queue; consume=dlq; batch=10; window=1s<br>SQS: queue=media-processor-queue; consume=dlq; batch=10; window=1s<br>SQS: queue=notification-processor-queue; consume=dlq; batch=10; window=1s<br>SQS: queue=push-delivery-queue; consume=dlq; batch=10; window=1s<br>Schedule: expression=rate(15 minutes) | — | basic | memory=512MB; timeout=30s; logs=7d |
| enhanced-federation-processor | processor-sqs | SQS: queue=enhanced-federation-queue; partialFailure=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| export-generator | processor-sqs | SQS: queue=export-processor-queue; partialFailure=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| federation-aggregator | hybrid | SQS: queue=federation-aggregator-queue; partialFailure=true<br>Schedule: expression=rate(1 hour) | — | basic | memory=512MB; timeout=30s; logs=7d |
| federation-delivery | processor-sqs | SQS: queue=federation-delivery-queue; partialFailure=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| federation-timeseries | processor-stream | Stream: table=main-table; start=LATEST; batch=25; window=5s; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| federation-tracker | processor-stream | Stream: table=main-table; start=LATEST; batch=25; window=5s; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| graphql | api-http | HTTP: GET /api/graphql<br>HTTP: POST /api/graphql | — | encryption | memory=512MB; timeout=30s; logs=7d |
| graphql-ws | api-ws | — | — | encryption | memory=512MB; timeout=30s; logs=7d |
| import-processor | processor-sqs | SQS: queue=import-processor-queue; partialFailure=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| inbox | api-http | HTTP: GET /users/{username}/inbox<br>HTTP: POST /users/{username}/inbox | — | encryption | memory=512MB; timeout=30s; logs=7d |
| media-processor | processor-sqs | SQS: queue=media-processor-queue; partialFailure=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| metrics-aggregator | processor-stream | Stream: table=main-table; start=LATEST; batch=25; window=5s; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| metrics-processor | processor-stream | Stream: table=main-table; start=LATEST; batch=25; window=5s; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| ml-training-processor | processor-stream | Stream: table=main-table; start=LATEST; batch=5; window=1s; parallel=1; maxRetry=3; bisect=true; reportBatchItemFailures=true | — | basic | memory=1024MB; timeout=900s; logs=7d |
| moderation-processor | processor-stream | Stream: table=main-table; start=LATEST; batch=10; window=5s; parallel=2; maxRetry=3; bisect=true; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| note-processor | processor-stream | Stream: table=main-table; start=LATEST; batch=25; window=5s; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| notification-processor | processor-sqs | SQS: queue=notification-processor-queue; partialFailure=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| objects | api-http | HTTP: GET /objects/{id} | — | encryption | memory=512MB; timeout=30s; logs=7d |
| outbox | api-http | HTTP: GET /users/{username}/outbox<br>HTTP: POST /users/{username}/outbox | — | encryption | memory=512MB; timeout=30s; logs=7d |
| push-delivery | processor-sqs | SQS: queue=push-delivery-queue; partialFailure=true | VAPID_PUBLIC_KEY<br>VAPID_SUBJECT<br>VAPID_SECRET_ARN | basic | memory=512MB; timeout=30s; logs=7d |
| report-trust-updater | processor-stream | Stream: table=main-table; start=LATEST; batch=25; window=5s; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| search-indexer | processor-stream | Stream: table=main-table; start=LATEST; batch=100; window=30s; parallel=5; maxRetry=3; bisect=true; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| severance-processor | processor-stream | Stream: table=main-table; start=LATEST; batch=10; window=5s; parallel=2; maxRetry=3; bisect=true; reportBatchItemFailures=true | — | basic | memory=1024MB; timeout=30s; logs=7d |
| sse | api-http | — | — | encryption | memory=512MB; timeout=900s; logs=7d |
| status-indexer | processor-stream | Stream: table=main-table; start=LATEST; batch=25; window=5s; reportBatchItemFailures=true | — | basic | memory=512MB; timeout=30s; logs=7d |
| stream-router | processor-stream | Stream: table=main-table; start=LATEST; batch=50; window=2s; parallel=5; maxRetry=3; bisect=true; reportBatchItemFailures=true | — | encryption | memory=512MB; timeout=30s; logs=7d |
| streaming | api-ws | — | — | encryption | memory=512MB; timeout=30s; logs=7d |
| trend-aggregator | processor-scheduled | Schedule: expression=cron(0 2 * * ? *) | — | basic | memory=512MB; timeout=30s; logs=7d |
| webfinger | api-http | HTTP: GET /.well-known/webfinger | — | basic | memory=512MB; timeout=30s; logs=7d |
| websocket-cost-aggregator | processor-scheduled | Schedule: expression=rate(1 hour) | — | basic | memory=512MB; timeout=30s; logs=7d |

<!-- INVENTORY_TABLE_END -->

This table is a starting point. The final inventory must be precise enough to generate CDK wiring without inference.

## Requirements
### R1 — Inventory is exhaustive and exact
- The inventory includes all product Lambdas above (no missing, no extra).

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
1. Add machine-readable inventory in `infra/cdk` (canonical source: `infra/cdk/inventory/lambdas.go`).
2. Generate CDK functions and wiring from the inventory (Specs 02–06).
3. Add a drift check that enforces set equality between `Makefile LAMBDAS`, inventory, CDK, and monitoring (Spec 07).

## Acceptance Criteria
- The inventory is the declared source of truth for Lambdas/triggers.
- Inventory set matches `Makefile LAMBDAS` exactly.
- Every downstream spec references this inventory rather than re-listing Lambdas ad-hoc.
