# Spec 06: Monitoring Stack Consistency (Dashboards, Alarms, Logs)

## Summary
Monitoring must be consistent with the deployed product set. Today, monitoring code can drift (stale function lists, environment naming mismatch, unused helper methods, and phantom resources). This spec makes monitoring inventory-driven so dashboards/alarms stay aligned with:
- what exists in code (`cmd/*`)
- what is deployed (`infra/cdk/constructs` + `infra/cdk/stacks`)
- what is declared as product (`infra/cdk/inventory/lambdas.go`)

This spec is written to be directly actionable by PAI: it defines canonical inputs, explicit decisions, the required file touch points, and operator-run verification.

## Canonical Inputs (Source of Truth)
PAI must treat these as canonical, in this priority order:
1. **Inventory:** `infra/cdk/inventory/lambdas.go` (product Lambda set + types + triggers).
2. **Naming conventions:** Spec 02 (Lambda names and log group names).
3. **Event-source contracts:** Spec 04 (streams/SQS/schedules exist and are wired from inventory).

## Execution Constraints
PAI must not execute commands, run tests, or synthesize CDK. PAI may add/modify code and scripts, but all verification steps are run by the operator (Codex CLI) outside PAI.

## Goals
- Ensure dashboards/alarms cover the full product set (all inventory Lambdas).
- Eliminate stale/phantom monitored functions.
- Standardize log group naming and retention, aligned with Spec 02 (no “prod” special cases that drift from `production`).

## Non-Goals
- Perfect production SLO design; this is a baseline for “all 9’s”.
- Designing a multi-tier monitoring coverage model (monitor all product Lambdas; tiering can be added later if needed).

## Decisions (Lock These For Implementation)
### D1 — Monitoring targets are inventory-driven
Monitoring must derive its Lambda target set from `infra/cdk/inventory/lambdas.go` and must not maintain a separate hand-written list of Lambda names.

### D2 — Use canonical environment strings
Use the stack `Environment` value verbatim for naming and tagging (e.g., `development`, `staging`, `production`). Treat `prod` as an alias only for backwards compatibility if it still exists in inputs, but do not emit `prod` in names/labels when the environment is `production`.

### D3 — Monitoring must not create application wiring
Monitoring must not create or own application event wiring resources:
- No EventBridge schedule rules for application jobs (Spec 04 owns schedules).
- No SQS queues / DLQs for processors (Spec 04 owns queues).
- No Lambda event source mappings (Spec 04 owns stream/SQS wiring).

Monitoring may create dashboards, alarms, and log/metric resources needed for observability.

### D4 — Log group naming and retention match Spec 02
Monitoring must assume Lambda log groups are named:
- `/aws/lambda/lesser-<environment>-<lambda>`

Retention must align with Spec 02 defaults:
- non-production: 7 days
- production: 30 days

If monitoring creates any log groups (e.g., API Gateway), it must apply the same retention policy logic.

## Requirements
### R1 — Monitoring targets derive from inventory
Monitoring should not maintain a separate hand-written list of Lambda names. It must be generated from the inventory (Spec 01) and naming (Spec 02).

### R2 — Environment naming is consistent
If the product uses `production`, monitoring must not special-case `prod` (or vice versa). One canonical environment string must be used everywhere.

### R3 — Core signals exist per trigger type
At minimum:
- Lambda: invocations, errors, duration, throttles
- Streams: iterator age (for stream processors)
- SQS: queue depth/age, DLQ depth (where queue names/metrics are available)
- API Gateway: 4xx/5xx and latency (for HTTP APIs)

### R4 — Alarms exist for high-severity conditions
At minimum:
- sustained Lambda errors
- throttles
- stream iterator age above threshold
- DLQ messages present

## Current Drift Findings (Examples)
- `infra/cdk/stacks/monitoring_stack.go` hard-codes a Lambda log group list that includes `push-notification` (not a product Lambda; the product Lambda is `push-delivery`).
- The same file uses `if environment == "prod"` to select retention, while the repo uses `production` as the canonical live environment string elsewhere.
- Monitoring helper methods exist but are not invoked anywhere (monitoring appears “present” but is not fully wired).
- The monitoring stack currently defines an application schedule rule creator (cost/trend); schedule wiring belongs to Spec 04, not monitoring.

## Implementation (PAI Execution Checklist)
PAI should implement these steps in order.

### Step 1 — Remove hard-coded function lists and phantom names
Update `infra/cdk/stacks/monitoring_stack.go`:
- Import `cdk/inventory` and derive the Lambda list from `inventory.LambdaInventory.Lambdas`.
- Remove any hand-written `[]string{...}` Lambda name lists.
- Ensure no phantom names remain (e.g., `push-notification`).

### Step 2 — Wire Lambda metrics/alarms for every inventory Lambda
Update `infra/cdk/stacks/monitoring_stack.go` so `NewMonitoringStack(...)` actually creates widgets/alarms:
- For each inventory Lambda, emit Lambda metrics (invocations/errors/duration/throttles).
- Use the *deployed* function name in CloudWatch dimensions: `lesser-<environment>-<lambda>`.
- For stream processors (inventory `StreamTriggers` non-empty), also emit IteratorAge widget + alarm.

### Step 3 — Add SQS + DLQ monitoring (inventory-driven)
For every inventory `SQSTrigger`:
- Emit SQS queue metrics and DLQ metrics (depth + oldest message age).
- Create an alarm on DLQ depth > 0.

Queue identification must follow the same physical naming convention used by Spec 04’s queue builder. Do not hard-code queue physical names in monitoring.

### Step 4 — Remove application wiring from monitoring
Delete or stop using any monitoring helpers that create application wiring (e.g., scheduled rules). Schedules are created in the application stack as part of Spec 04.

### Step 5 — Add a guardrail test (optional but recommended)
Add a CDK unit test (in `infra/cdk/stacks`) that fails if monitoring:
- contains a hand-written Lambda list, or
- references a Lambda name not present in the inventory.

## Operator Verification (Run Outside PAI)
- `make verify-inventory`
- `cd infra/cdk && go test ./...`
- `cd infra/cdk && cdk synth` (if toolchain is available)

## Acceptance Criteria
- Monitoring stack references only inventory Lambdas.
- No phantom functions appear in dashboards/alarms.
- Environment naming is consistent across stacks and alarms.
