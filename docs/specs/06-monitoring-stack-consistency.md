# Spec 06: Monitoring Stack Consistency (Dashboards, Alarms, Logs)

## Summary
Monitoring must be consistent with the deployed product set. Today, monitoring code can drift (stale function lists, environment naming mismatch, unused helper methods). This spec aligns monitoring with the inventory and makes it hard to regress.

## Goals
- Ensure dashboards/alarms cover the full product set (or a documented subset by tier).
- Eliminate stale/phantom monitored functions.
- Standardize log group naming and retention, aligned with Spec 02.

## Non-Goals
- Perfect production SLO design; this is a baseline for “all 9’s”.

## Requirements
### R1 — Monitoring targets derive from inventory
Monitoring should not maintain a separate hand-written list of Lambda names. It must be generated from the inventory (Spec 01) and naming (Spec 02).

### R2 — Environment naming is consistent
If the product uses `production`, monitoring must not special-case `prod` (or vice versa). One canonical environment string must be used everywhere.

### R3 — Core signals exist per trigger type
At minimum:
- Lambda: invocations, errors, duration, throttles
- Streams: iterator age (for stream processors)
- SQS: queue depth/age, DLQ depth
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
- Monitoring helper methods exist but are not consistently invoked from the main stack (monitoring appears “present” but not fully wired).

## Proposed Implementation
1. Create a monitoring “tier” model in inventory:
   - `tier=critical|important|best-effort`
2. Generate dashboard widgets and alarms from inventory tiers.
3. Ensure log group naming and retention follow Spec 02 conventions.

## Acceptance Criteria
- Monitoring stack references only inventory Lambdas.
- No phantom functions appear in dashboards/alarms.
- Environment naming is consistent across stacks and alarms.
