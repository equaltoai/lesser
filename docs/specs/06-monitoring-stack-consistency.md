# Spec 06: Monitoring Stack Consistency

## Summary
This spec makes the monitoring layer **complete and consistent** across environments by ensuring the CloudWatch monitoring stack is:
- **Inventory-driven** (no hard-coded Lambda or queue lists).
- **Observability-only** (must not create application wiring like queues, schedules, or event source mappings).
- **Environment-consistent** (names include the canonical environment and do not invent aliases like `prod`).

## Execution Constraints
PAI does not execute commands, tests, or CDK synth. Operators run verification outside PAI.

## Definitions (Environment vs Stage)
- **Environment**: `development | staging | production` (stack naming, resource naming, alarms).
- **DNS stage label**: `dev | staging | live` (domain convention). This is separate from `Environment` and must not leak into alarm names.

## Canonical Inputs (Source of Truth)
- **Lambda inventory**: `infra/cdk/inventory/lambdas.go` (`inventory.LambdaInventory`)
  - Source of truth for: Lambda names, types, stream/SQS/schedule triggers.
- **Resource naming conventions**
  - Lambda physical name: `lesser-<environment>-<lambdaName>` (`infra/cdk/constructs/lambda_functions.go`)
  - SQS queue physical name: `lesser-<queueLogical>-<environment>` (`infra/cdk/stacks/lesser_api_stack.go#createSQSQueues`)
  - DynamoDB tables: `lesser-<environment>` and `lesser-rate-limits-<environment>` (`infra/cdk/stacks/lesser_api_stack.go#createSharedResources`)
  - HTTP API name: `lesser-<environment>-api` (`infra/cdk/constructs/api_routes.go`)

## Goals
1. Monitoring stack creates a dashboard and alarms for **all** inventory Lambdas.
2. Monitoring stack creates alarms for **all** SQS queues implied by inventory SQS triggers (plus the canonical `scheduled-queue`) and their DLQs.
3. Monitoring stack provides baseline DynamoDB alarms for main + rate-limit tables.
4. Monitoring stack does **not** create SQS queues, EventBridge schedules/rules, or Lambda event source mappings.
5. Guardrail tests prevent drift (phantom Lambdas/queues, hard-coded lists, or accidental wiring resources).

## Non-Goals
- Adding or changing application wiring (routes, stream processors, schedules, queue creation).
- Building a full SLO/SLA layer or business-metric dashboards (future spec).
- Managing log group retention here (already owned by:
  - `infra/cdk/constructs/lambda_functions.go` for Lambda log groups
  - `infra/cdk/constructs/api_routes.go` for API Gateway access logs)

## Required Implementation

### A. Environment Canonicalization (CDK defaults)
Ensure defaults use canonical environment names:
- Default CDK context environment must be `development` (not `dev`).
  - Update `infra/cdk/cdk.json` default `context.environment`.
  - Update `infra/cdk/main.go` default when context is missing.

### B. Inventory-Driven CloudWatch (MonitoringStack)
Implement monitoring population directly in `infra/cdk/stacks/monitoring_stack.go` constructor (`NewMonitoringStack`):

#### B1. Lambda monitoring (inventory-driven)
For every `inventory.LambdaInventory.Lambdas` entry:
- Compute physical function name: `lesser-<environment>-<lambdaName>`.
- Add dashboard widgets for:
  - `Invocations` (Sum) vs `Errors` (Sum)
  - `Duration` (Average) vs `Throttles` (Sum)
  - For stream/hybrid Lambdas with stream triggers: `IteratorAge` (Maximum)
- Create alarms (per Lambda):
  - Error rate (%): Math expression `(errors / invocations) * 100`
  - Duration (ms) threshold (environment-tuned defaults are fine)
  - Throttles >= 1
  - Iterator age for stream processors

Constraints:
- Do not hard-code function-name lists.
- Alarm names must include `lesser-<environment>-<lambdaName>` (or the full physical function name).

#### B2. SQS monitoring (derived from inventory triggers)
Derive a unique set of queues from all `LambdaSpec.SQSTriggers`:
- Canonical primary queue name: `lesser-<queueLogical>-<environment>`
- Canonical DLQ logical name:
  - If `SQSTrigger.DeadLetterQueue` is set, use that.
  - Else default to `<queueLogical>-dlq`.
  - DLQ physical name: `lesser-<dlqLogical>-<environment>`
- Always include the canonical `scheduled-queue` (+ its DLQ) even if no consumer exists yet (Spec 05).

Create dashboard widgets + alarms:
- Primary queue: `ApproximateAgeOfOldestMessage` (alarm if above threshold).
- DLQ: `ApproximateNumberOfMessagesVisible` (alarm if > 0 for sustained period).

Constraints:
- Monitoring must not create `AWS::SQS::Queue` resources (queues are owned by the application stack).

#### B3. DynamoDB monitoring (baseline)
Create at least:
- Dashboard widgets for consumed read/write capacity.
- Alarms for read/write throttles for:
  - `lesser-<environment>`
  - `lesser-rate-limits-<environment>`

### C. Guardrails (Tests)
Add/extend tests under `infra/cdk/stacks/` to enforce:
- Monitoring stack creates Lambda alarms for every inventory Lambda (no missing/phantom).
- Monitoring stack does not create application wiring resources:
  - no `AWS::SQS::Queue`
  - no `AWS::Lambda::EventSourceMapping`
  - no `AWS::Events::Rule`

## Operator Verification (Manual Only; PAI Must Not Run)
- `make verify-inventory`
- `cd infra/cdk && go test ./...`
- `cd infra/cdk && cdk synth --context environment=development`

## Acceptance Criteria
- Monitoring stack is populated and inventory-driven (no stub dashboards).
- Alarm/dashboard naming is consistent and environment-correct (`development|staging|production`).
- Inventory changes deterministically update monitoring coverage.
- Guardrail tests prevent monitoring drift and prevent accidental wiring resources from entering the monitoring stack.
