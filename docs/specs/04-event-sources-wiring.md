# Spec 04: Event Sources Wiring (Streams, SQS, Schedules)

## Summary
Lesser relies on multiple non-HTTP triggers (DynamoDB streams, SQS, EventBridge schedules). CDK must wire each Lambda to the correct event source(s) based on what its handler supports, without drift between:
- handler implementations in `cmd/*/main.go`
- the canonical inventory in `infra/cdk/inventory/lambdas.go`
- CDK wiring (event source mappings, schedules, and queues)

This spec is written to be directly actionable by PAI: it defines canonical inputs, concrete outputs, the required file touch points, and acceptance gates.

## Canonical Inputs (Source of Truth)
PAI must treat these as canonical, in this priority order:
1. **Inventory:** `infra/cdk/inventory/lambdas.go` (fields: `StreamTriggers`, `SQSTriggers`, `ScheduleTriggers`).
2. **Handler truth (to correct inventory):** `cmd/*/main.go` (actual handler signatures and Lift registrations).

Do **not** introduce a second “events inventory” file (e.g., `events.yaml`) for this spec. Inventory lives in `infra/cdk/inventory/lambdas.go` so downstream specs (01/02/03/05/07) stay consistent.

## Goals
- Make event wiring correct and inventory-driven.
- Ensure processors are invoked by the right event types (stream vs SQS vs schedule).
- Standardize queue/schedule naming and DLQ patterns.

## Non-Goals
- Rewriting business behavior beyond what’s needed to align trigger contracts.
- Adding new async systems beyond DynamoDB + SQS + EventBridge.

## Decisions (Lock These For Implementation)
### Q1 — Import/export canonical queue model (RESOLVED)
Canonical model is **per-job queues**. CDK must provision and wire distinct queues for:
- import processing
- export processing
- media processing
- scheduled publishing
- federation delivery
- push delivery
- any other SQS-triggered processor declared in inventory

CDK must **not** provision or wire a unified `ImportExportQueue` path going forward (legacy `IMPORT_EXPORT_QUEUE_URL` style contracts are considered deprecated).

### Q2 — `cost-aggregator` trigger model (RESOLVED)
**Stream-only.**
- Inventory must declare only `StreamTriggers` for `cost-aggregator` (no `ScheduleTriggers`).
- CDK must not create an EventBridge schedule for it.

### Schedule cadences (RESOLVED)
Use these schedule expressions (EventBridge expressions are UTC):
- `dlq-processor`: `rate(15 minutes)`
- `federation-aggregator`: `rate(1 hour)`
- `trend-aggregator`: `cron(0 2 * * ? *)` (daily 02:00 UTC)
- `websocket-cost-aggregator`: `rate(1 hour)`

### Schedule enablement by environment (RESOLVED)
Create EventBridge rules in **all** environments/stages, including `development`, `staging`, and `production`.

## Requirements
### R0 — Inventory is the single wiring contract
CDK event wiring must be generated from `infra/cdk/inventory/lambdas.go`. Any drift discovered against `cmd/*/main.go` must be fixed by updating the inventory first.

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
- Do not enable partial batch failure reporting unless the handler returns the correct response type (see R6).

### R5 — Event filtering strategy is consistent
Where possible:
- Prefer in-code filtering when event source filters are unstable or brittle.
- If using event source filters, define them in the inventory and test `cdk synth` stability.

### R6 — Partial batch failure reporting is capability-gated
Inventory flags must reflect handler capability:
- For SQS, only set `SQSTrigger.EnablePartialFailure=true` if the Lambda handler returns `events.SQSEventResponse` (or an equivalent wrapper that produces the batch-item-failures payload).
- For streams, only set `StreamTrigger.ReportBatchItemFailures=true` if the Lambda handler returns the DynamoDB partial failure response type (if/when implemented).

If capability is unknown or absent, default the flags to **false** so runtime behavior stays whole-batch and predictable.

### R7 — Deprecated/hard-coded wiring is removed
`infra/cdk/constructs/stream_processors.go` (or its successor) must not contain hard-coded Lambda→event-source mappings. All attachments must be derived by iterating the inventory.

### R8 — Inventory-declared triggers are fully realized
For every trigger declared in inventory, CDK must create the corresponding AWS resources:
- `StreamTriggers` → `AWS::Lambda::EventSourceMapping` (DynamoDB stream)
- `SQSTriggers` → `AWS::SQS::Queue` + `AWS::SQS::Queue` (DLQ) + `AWS::Lambda::EventSourceMapping`
- `ScheduleTriggers` → `AWS::Events::Rule` + Lambda target

## Implementation (PAI Execution Checklist)
PAI should implement these steps in order.

### Step 1 — Reconcile inventory against handlers
Update `infra/cdk/inventory/lambdas.go` so it matches handler reality in `cmd/*/main.go`:
- Remove any triggers that don’t match the handler’s supported event types.
- Add missing triggers where the handler clearly supports them (e.g., Lift `app.SQS(...)`, `patterns.RegisterEventBridge(...)`, etc.).
- Apply R6 (partial failure flags match handler signature).
- Replace any placeholder schedule expressions with the defaults above (or your chosen values).

Then re-run the inventory doc generator to keep Spec 01 current:
- `make generate-inventory`

### Step 2 — Build queues from inventory (per-job queues)
Update CDK to provision queues from inventory (and delete the unified import/export queue path):
- Replace `LesserApiStack.createSQSQueues`’s current hard-coded queues (`ImportExportQueue`, etc.) with an inventory-driven queue builder.
- Create one queue per unique `SQSTrigger.Queue` in the inventory.
- Create one DLQ per queue (use `SQSTrigger.DeadLetterQueue` if provided; otherwise default to `<queue>-dlq`).
- Apply a single, consistent naming scheme for deployed queue names, e.g. `lesser-<logical-queue-name>-<environment>`.

Minimum queue defaults (unless explicitly overridden later):
- `ReceiveMessageWaitTime`: 20 seconds (long polling)
- `DLQ retention`: 14 days
- `maxReceiveCount`: 5

### Step 3 — Attach stream and SQS event sources from inventory
Replace hard-coded wiring in `infra/cdk/constructs/stream_processors.go` with inventory iteration:
- For each Lambda with `StreamTriggers`, attach a DynamoDB stream event source mapping with the configured batching/position/retry settings.
- For each Lambda with `SQSTriggers`, attach an SQS event source mapping with configured batching/window settings and `ReportBatchItemFailures` derived from `EnablePartialFailure`.
- Enforce R1/R2 via validation: fail synth/test if an HTTP-only/WS-only Lambda declares a non-HTTP trigger.

### Step 4 — Attach schedules from inventory
Create EventBridge rules for each `ScheduleTrigger`:
- Do not gate schedules by environment; schedules must exist in all stages.
- Use `ScheduleTrigger.Expression` verbatim as the EventBridge schedule expression.
- If `ScheduleTrigger.Input` is set, attach it as the rule target input (text or JSON).

### Step 5 — Add guardrails (tests or synth-time validation)
Add a CDK-level guardrail so PAI can prove Spec 04 is satisfied:
- A `go test` in `infra/cdk/constructs` that synthesizes a stack and asserts:
  - every inventory `StreamTrigger` yields an `AWS::Lambda::EventSourceMapping` for the right function
  - every inventory `SQSTrigger` yields the queue+DLQ and an event source mapping
  - every inventory `ScheduleTrigger` yields an `AWS::Events::Rule` with the expected expression and target

## Current Drift Findings (Examples)
- `infra/cdk/constructs/stream_processors.go` attaches DynamoDB stream event sources to Lambdas that are not stream handlers.
- Some SQS sources exist in infra but are not represented in the inventory, and vice versa.
- A unified `ImportExportQueue` exists in CDK but the product model is per-job queues (Q1).

## Acceptance Gates (What PAI Must Make Green)
- `make verify-inventory` (set equality + Spec 01 freshness + Spec 02/03 tests)
- `cd infra/cdk && go test ./...` (or at minimum, the Spec 04 guardrail test added above)

## Acceptance Criteria
- Every processor Lambda has exactly the event sources it expects (no missing, no incorrect extras).
- SQS queues have DLQs and redrive policies.
- Schedule rules exist for every inventory `ScheduleTrigger`.
