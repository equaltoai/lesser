# Spec 04: Event Sources Wiring (Streams, SQS, Schedules)

## Summary
The Phase 1–4 implementation is **fully inventory-driven** for DynamoDB streams, SQS, and EventBridge schedules. This document records the live wiring model, queue naming/DLQ conventions, trigger validation rules (R1–R6), schedule expressions, and operational notes. All resources below are derived from `infra/cdk/inventory/lambdas.go` and materialized by CDK constructs.

## Canonical Sources (Do Not Fork)
1. Inventory: `infra/cdk/inventory/lambdas.go` (authoritative for `StreamTriggers`, `SQSTriggers`, `ScheduleTriggers`).
2. Generated doc: `docs/specs/01-lambda-inventory-matrix.md` (run `make generate-inventory`).
3. Wiring implementation: `infra/cdk/constructs/stream_processors.go`, `infra/cdk/constructs/schedule_wiring.go`.
4. Queue provisioning: `infra/cdk/stacks/lesser_api_stack.go:createSQSQueues` (inventory-driven).
5. Guardrail test: `infra/cdk/constructs/trigger_wiring_test.go` (asserts every inventory trigger produces the corresponding mapping/queue/rule).

## Wiring Implementation Snapshot (Phase 1–4)
- **Streams & SQS wiring:** Iterates inventory; validates Lambda type (R1/R2) before attaching (`validateStreamCapable`, `validateSQSCapable`); panics if a declared queue is missing.
- **DLQ consumption (SQS):** If `SQSTrigger.ConsumeDeadLetterQueue=true`, the event source mapping targets the queue’s DLQ (`<queue>-dlq`) instead of the primary queue.
- **Schedules:** Iterates inventory; validates Lambda type (R3); creates **enabled** EventBridge rules in all environments with rule name `lesser-<env>-<lambda>-schedule-<idx>`.
- **Queues:** One queue per unique `SQSTrigger.Queue`; DLQ per queue (default `<queue>-dlq`); aliases created for compatibility (`federation-queue`, `push-notification-queue`, `import-export-queue` point to the same underlying queue pairs). Additionally, `scheduled-queue` is provisioned as a standalone queue to satisfy the canonical env-var contract (Spec 05), even though it is not currently wired as an event source mapping.
- **Naming:** Physical queue name `lesser-<logical>-<environment>`; DLQ `lesser-<logical-dlq>-<environment>`.
- **Defaults:** Long polling 20s, visibility 2m, retention 4d, DLQ retention 14d, maxReceiveCount 5, partial batch failure **off** unless inventory enables it.

## Queue/DLQ Model (Inventory-Driven)
- **Per-job queues** only; no unified ImportExport queue path.
- Each queue gets its own DLQ; redrive policy maxReceiveCount=5.
- Tags: `app=lesser`, `environment=<env>`.
- Aliases: `federation-delivery-queue` → also exposed as `federation-queue`; `push-delivery-queue` → `push-notification-queue`; `import-processor-queue` → `import-export-queue`.

### SQS Event Sources (per Lambda)
| Lambda | Inventory Queue Key | Consumes | Physical Queue Name (pattern) | Redrive DLQ Name (pattern) | Partial Failure | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| dlq-processor | enhanced-federation-queue | DLQ | lesser-enhanced-federation-queue-dlq-<env> | — | false | Consumes DLQ for `enhanced-federation-queue` |
| dlq-processor | export-processor-queue | DLQ | lesser-export-processor-queue-dlq-<env> | — | false | Consumes DLQ for `export-processor-queue` |
| dlq-processor | federation-aggregator-queue | DLQ | lesser-federation-aggregator-queue-dlq-<env> | — | false | Consumes DLQ for `federation-aggregator-queue` |
| dlq-processor | federation-delivery-queue | DLQ | lesser-federation-delivery-queue-dlq-<env> | — | false | Consumes DLQ for `federation-delivery-queue` |
| dlq-processor | import-processor-queue | DLQ | lesser-import-processor-queue-dlq-<env> | — | false | Consumes DLQ for `import-processor-queue` |
| dlq-processor | media-processor-queue | DLQ | lesser-media-processor-queue-dlq-<env> | — | false | Consumes DLQ for `media-processor-queue` |
| dlq-processor | notification-processor-queue | DLQ | lesser-notification-processor-queue-dlq-<env> | — | false | Consumes DLQ for `notification-processor-queue` |
| dlq-processor | push-delivery-queue | DLQ | lesser-push-delivery-queue-dlq-<env> | — | false | Consumes DLQ for `push-delivery-queue` |
| enhanced-federation-processor | enhanced-federation-queue | Primary | lesser-enhanced-federation-queue-<env> | lesser-enhanced-federation-queue-dlq-<env> | false |  |
| export-generator | export-processor-queue | Primary | lesser-export-processor-queue-<env> | lesser-export-processor-queue-dlq-<env> | false | Env var `EXPORT_PROCESSOR_QUEUE_URL` populated |
| federation-aggregator | federation-aggregator-queue | Primary | lesser-federation-aggregator-queue-<env> | lesser-federation-aggregator-queue-dlq-<env> | false | Also scheduled (hourly) |
| federation-delivery | federation-delivery-queue | Primary | lesser-federation-delivery-queue-<env> | lesser-federation-delivery-queue-dlq-<env> | false | Alias `federation-queue` shares the same pair |
| import-processor | import-processor-queue | Primary | lesser-import-processor-queue-<env> | lesser-import-processor-queue-dlq-<env> | false | Alias `import-export-queue`; env var `IMPORT_PROCESSOR_QUEUE_URL` populated |
| media-processor | media-processor-queue | Primary | lesser-media-processor-queue-<env> | lesser-media-processor-queue-dlq-<env> | false | Env var `MEDIA_PROCESSOR_QUEUE_URL` populated |
| notification-processor | notification-processor-queue | Primary | lesser-notification-processor-queue-<env> | lesser-notification-processor-queue-dlq-<env> | false |  |
| push-delivery | push-delivery-queue | Primary | lesser-push-delivery-queue-<env> | lesser-push-delivery-queue-dlq-<env> | false | Alias `push-notification-queue`; populates push queue URLs |

*(All partial failure flags are currently false; `ReportBatchItemFailures` is only set when `EnablePartialFailure=true`.)*

### Stream Event Sources (DynamoDB `main-table` stream)
| Lambda | Starting Position | Batch Size | Report Batch Item Failures | Notes |
| --- | --- | --- | --- | --- |
| activity-processor | TRIM_HORIZON | 25 | true | Parallelization 5, bisect on error |
| ai-processor | TRIM_HORIZON | 25 | true | Parallelization 2, bisect on error |
| cost-aggregator | LATEST | 10 | true | Stream-only (Q2) |
| federation-timeseries | LATEST | 25 | true |  |
| federation-tracker | LATEST | 25 | true |  |
| metrics-aggregator | LATEST | 25 | true |  |
| metrics-processor | LATEST | 25 | true |  |
| ml-training-processor | LATEST | 5 | true | Bisect on error |
| moderation-processor | LATEST | 10 | true | Bisect on error |
| note-processor | LATEST | 25 | true |  |
| report-trust-updater | LATEST | 25 | true |  |
| search-indexer | LATEST | 100 | true | Parallelization 5, bisect on error |
| severance-processor | LATEST | 10 | true | Parallelization 2, bisect on error |
| status-indexer | LATEST | 25 | true |  |
| stream-router | LATEST | 50 | true | Parallelization 5, bisect on error |

*(All stream triggers target the deployed `main-table` stream; non-`main-table` sources are rejected.)*

### Schedule Triggers
| Lambda | Expression | Rule Name Pattern | Enabled | Notes |
| --- | --- | --- | --- | --- |
| dlq-processor | rate(15 minutes) | lesser-<env>-dlq-processor-schedule-0 | true | DLQ sweeps all envs |
| federation-aggregator | rate(1 hour) | lesser-<env>-federation-aggregator-schedule-0 | true | All envs |
| trend-aggregator | cron(0 2 * * ? *) | lesser-<env>-trend-aggregator-schedule-0 | true | Daily 02:00 UTC |
| websocket-cost-aggregator | rate(1 hour) | lesser-<env>-websocket-cost-aggregator-schedule-0 | true | All envs |

## Trigger Validation Rules (Aligned to R1–R6)
- **R1 (streams):** Only `LambdaTypeProcessorStream` or `LambdaTypeHybrid` may declare streams; enforced by `validateStreamCapable`.
- **R2 (SQS):** Only `LambdaTypeProcessorSQS` or `LambdaTypeHybrid` may declare SQS; enforced by `validateSQSCapable`. Missing queues panic at synth time. If `ConsumeDeadLetterQueue=true`, the mapping targets the queue’s DLQ (`<queue>-dlq`) and the DLQ must exist.
- **R3 (schedules):** Only `LambdaTypeProcessorScheduled` or `LambdaTypeHybrid` may declare schedules; enforced by `validateScheduleCapable`.
- **R4 (DLQ):** Every SQS queue has a DLQ with maxReceiveCount=5 and DLQ retention=14d; main queue retention=4d; long polling=20s.
- **R5 (filtering):** No event source filters are defined in inventory; filtering remains in-handler (consistent with current code).
- **R6 (partial failures):** `ReportBatchItemFailures` is set only when `EnablePartialFailure=true` in inventory. Current inventory sets all SQS partial failures to `false`; stream partial failures are `true` where handlers support it.

## Operational Notes
- **Redrive:** Messages exceeding maxReceiveCount are sent to the queue’s DLQ; monitor DLQ age/count (see Spec 06 monitoring).
- **Batch failure behavior:** With partial failures off (all SQS triggers), failures are whole-batch; DLQ redrive applies after retries.
- **Schedule scope:** Rules are created and enabled in **all environments** (dev/staging/prod) using the name pattern above. `dlq-processor` treats EventBridge’s default scheduled `detail-type` (`"Scheduled Event"`) as a scheduled reprocessing sweep; no custom rule input is required.
- **Env vars:** Queue URLs are injected per trigger; aliases ensure legacy names (`federation-queue`, `push-notification-queue`, `import-export-queue`) resolve to the same physical queues.

## Cross-Links and Regeneration
- Regenerate inventory docs: `make generate-inventory` → `docs/specs/01-lambda-inventory-matrix.md`.
- Guardrail: `cd infra/cdk && go test ./constructs -run TestInventoryTriggersMaterializeResources` validates every mapping/queue/rule.
- Inventory source of truth: `infra/cdk/inventory/lambdas.go` must be updated first for any wiring change.

## Acceptance Criteria (Documentation Alignment)
- Tables above enumerate every inventory-declared stream/SQS/schedule trigger with the live naming scheme.
- R1–R6 and queue/DLQ defaults match `stream_processors.go`, `schedule_wiring.go`, and `createSQSQueues`.
- Alias queues and per-job queue model are explicitly documented (no unified ImportExport path).
- Schedule expressions match deployed values for dlq-processor, federation-aggregator, trend-aggregator, websocket-cost-aggregator.
