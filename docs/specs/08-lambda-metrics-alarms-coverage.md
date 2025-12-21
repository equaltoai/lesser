# Phase: Lambda Metrics & Alarms Coverage

## Objective
Create inventory-driven Lambda widgets and alarms for every product Lambda, including stream-specific iterator age where applicable.

## Scope
- Monitoring stack: `infra/cdk/stacks/monitoring_stack.go`
- Alarm defaults: `infra/cdk/constructs/alarm_config.go`
- Inventory source: `infra/cdk/inventory/lambdas.go`
- Naming: deployed Lambda functions are `lesser-<environment>-<lambda>`

## Canonical Inputs
- Inventory (`inventory.LambdaInventory.Lambdas`) defines the complete Lambda set and stream-capable functions (`StreamTriggers` non-empty).
- Alarm thresholds come from `AlarmConfigBuilder` (keep environment-sensitive defaults; reuse iterator age/duration/error/throttle thresholds).
- Environment string is the stack `Environment` value (use `production`, not `prod`, in emitted resource names).

## Requirements
1) Inventory-driven targets
- Iterate all `inventory.LambdaInventory.Lambdas`; derive metric dimensions from deployed name `lesser-${Environment}-${lambda.Name}`.
- No hard-coded Lambda name conditionals; remove special-case lists (e.g., prior stream name checks).

2) Core Lambda metrics & widgets
- Emit metrics: Invocations (Sum), Errors (Sum), Duration (Average), Throttles (Sum), ConcurrentExecutions (Maximum), period 5m.
- Widget grouping per function:
  - Graph 1 (12x6): Invocations (left) + Errors (right)
  - Graph 2 (12x6): Duration (left) + Throttles (right)
  - If stream-capable: Graph 3 (24x6): IteratorAge (left, Maximum) + ConcurrentExecutions (right)

3) Alarms (attach to existing AlertTopic)
- Error rate: math expression `(errors / invocations) * 100`, threshold from `config.Lambda.ErrorRateThreshold`, evaluation/DP to alarm from config; TreatMissingData=NOT_BREACHING.
- Duration: threshold from `config.Lambda.DurationThresholdMs`, evaluation/DP to alarm from config; TreatMissingData=NOT_BREACHING.
- Throttles: threshold from `config.Lambda.ThrottleThreshold` (default any>0), evaluation periods 1; TreatMissingData=NOT_BREACHING.
- IteratorAge (stream-capable only): threshold from `config.Lambda.IteratorAgeThresholdMs`, evaluation periods 2; TreatMissingData=NOT_BREACHING.
- Reuse `AlarmManager`/`AlarmConfigBuilder` defaults; do not introduce new per-function hard-codes.

4) Stream detection
- A function is stream-capable when `len(lambda.StreamTriggers) > 0`.
- Apply iterator age widgets/alarms only to stream-capable Lambdas.

5) Dashboard wiring
- Use the existing `Dashboard` instance in MonitoringStack; add widgets per Lambda in order of inventory.
- Keep dashboard start/time range as-is.

6) Log/metric dimensions
- Function dimension key: `FunctionName: lesser-${Environment}-${lambda.Name}` (matches deployed logical name).
- Period: 5 minutes for all metrics; statistics as above.

7) Missing data behavior
- All Lambda alarms treat missing data as NOT_BREACHING (explicitly set).

8) Non-goals / exclusions
- Do not create or change event source mappings, schedules, or queues in monitoring.
- Do not alter alarm thresholds beyond reusing `AlarmConfigBuilder` defaults.

## Implementation Notes (for code changes)
- In `MonitoringStack`, iterate inventory and invoke a refactored helper that builds metrics/widgets/alarms using `AlarmManager` thresholds and AlertTopic actions.
- Remove prior hard-coded stream conditional (`if functionName == "activity-processor" ...`) and any phantom names.
- Ensure function name passed to metrics/alarms includes `lesser-${Environment}-` prefix.

## Acceptance Criteria
- Every inventory Lambda appears in dashboard widgets and has error/duration/throttle alarms; stream Lambdas also have iterator age widgets/alarms.
- No hard-coded Lambda name lists remain; monitoring is fully inventory-driven.
- All alarms use existing defaults from `AlarmConfigBuilder` and treat missing data as not breaching.
- Widgets are added to the existing monitoring dashboard instance with logical grouping as specified.