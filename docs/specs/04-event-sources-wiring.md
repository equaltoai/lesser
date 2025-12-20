# Spec 04: Event Sources Wiring (Streams, SQS, Schedules)

## Summary
Lesser relies on multiple non-HTTP triggers (DynamoDB streams, SQS, EventBridge schedules). CDK must wire each Lambda to the correct event source(s) based on what its handler supports.

## Goals
- Make event wiring correct and inventory-driven.
- Ensure processors are invoked by the right event types (stream vs SQS vs schedule).
- Standardize queue/schedule naming and DLQ patterns.

## Non-Goals
- Rewriting business behavior beyond what’s needed to align trigger contracts.
- Adding new async systems beyond DynamoDB + SQS + EventBridge.

## Requirements
### R1 — DynamoDB stream mappings match stream handlers
Only Lambdas that consume `events.DynamoDBEvent` (or Lift stream equivalents) may be attached to DynamoDB streams.

### R2 — SQS event source mappings match SQS handlers
Only Lambdas that consume `events.SQSEvent` may be attached to SQS queues.

### R3 — Scheduled rules only target schedule handlers
Only Lambdas that implement the scheduled handler pattern (EventBridge/CloudWatch events) may be invoked by schedules.

Infra must not “schedule-invoke” a stream-only handler.

### R4 — Each queue has a DLQ and sane redrive
For SQS-triggered processors:
- Create per-queue DLQs (or a justified shared DLQ) with explicit `maxReceiveCount`.
- Ensure processors handle partial batch failures (`ReportBatchItemFailures`) where appropriate.

### R5 — Event filtering strategy is consistent
Where possible:
- Prefer in-code filtering when event source filters are unstable or brittle.
- If using event source filters, define them in the inventory and test `cdk synth` stability.

## Key Design Questions (Must Decide)
### Q1 — Import/export canonical queue model
There appear to be multiple queueing paths:
- a single “import/export” queue path (e.g., `IMPORT_EXPORT_QUEUE_URL`)
- per-job queues (e.g., `IMPORT_QUEUE_URL`, `EXPORT_QUEUE_URL`, `MEDIA_QUEUE_URL`, `SCHEDULED_QUEUE_URL`, `FEDERATION_DELIVERY_QUEUE_URL`)

Decision needed:
- **Canonical** should be per-job queues (most explicit, avoids mixed schemas), or
- Canonical should be a single queue with a unified message schema and router.

This decision drives:
- CDK queue creation
- env var contracts (Spec 05)
- processor event schemas

### Q2 — `cost-aggregator` trigger model
Options:
- **stream-only** (DynamoDB stream)
- **schedule-only** (EventBridge cadence)
- **hybrid** (both)

Decision needed:
- If stream-only, infra must attach a stream mapping and must not schedule-invoke it.
- If schedule-only, the code must implement the scheduled handler path.
- If hybrid, define the split of responsibility and dedupe risks.

## Current Drift Findings (Examples)
- `infra/cdk/constructs/stream_processors.go` attaches DynamoDB stream event sources to Lambdas that are not stream handlers (e.g., routing streams into `OutboxFunction` and “timeline” fields that are not stream processors).
- The same file attaches an SQS event source to `NotificationProcessor`, but CDK currently constructs that field as `"push-delivery"` (so semantics/monitoring/wiring drift).
- Stream event source filters were removed in CDK to “fix deployment issues”; if filters are reintroduced, they must be inventory-driven and validated with `cdk synth`.

## Proposed Implementation
1. Extend the machine inventory (Spec 01) to include event sources:
   - `stream`: table/stream ARN, starting position, batch size, retry settings
   - `sqs`: queue + DLQ, batch size, max batching window, partial failure reporting
   - `schedule`: cron/rate, input payload, enabled environments
2. Generate event source mappings/rules from inventory in CDK.
3. Add `cdk synth` validations (Spec 07) that fail if:
   - a stream handler has no stream mapping
   - an SQS handler has no queue mapping
   - a schedule handler has no EventBridge rule

## Acceptance Criteria
- Every processor Lambda has exactly the event sources it expects (no missing, no incorrect extras).
- SQS queues have DLQs and redrive policies.
- Open questions Q1 and Q2 are resolved and reflected in inventory + CDK.
