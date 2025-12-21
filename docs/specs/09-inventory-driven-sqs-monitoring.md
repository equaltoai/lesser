Phase: Inventory-Driven SQS & DLQ Monitoring

Objective
Add SQS queue depth/age and DLQ depth alarms/widgets for every inventory-declared SQS trigger. Use the same physical queue naming convention as queue creation (lesser-<queue>-<environment>) and include DLQs when ConsumeDeadLetterQueue is true.

Canonical Inputs
- Inventory: infra/cdk/inventory/lambdas.go (LambdaInventory.Lambdas[].SQSTriggers).
- Queue naming: infra/cdk/stacks/lesser_api_stack.go:createSQSQueues() uses QueueName fmt.Sprintf("lesser-%s-%s", logicalQueue, environment); DLQ logical defaults to <queue>-dlq with the same naming pattern.
- Queue aliases (for compatibility): federation-delivery-queue -> federation-queue; push-delivery-queue -> push-notification-queue; import-processor-queue -> import-export-queue; scheduled-queue is always created even without an inventory consumer.
- Alarm thresholds: infra/cdk/constructs/alarm_config.go (AlarmConfigBuilder().GetConfiguration(env).SQS).
- Monitoring stack entrypoint: infra/cdk/stacks/monitoring_stack.go.

Scope
- Derive monitoring targets from inventory (no hand-written queue lists).
- Resolve physical queue names/URLs using the same logic/aliases used when queues are created.
- Add CloudWatch metrics/widgets and alarms for primary queues and DLQs.
- Integrate into MonitoringStack without creating/wiring application resources.

Non-Goals
- Do not create queues, DLQs, event source mappings, or schedules in monitoring.
- Do not hard-code queue names; never diverge from createSQSQueues naming/aliasing.
- Do not change queue properties (visibility, retention, redrive) from monitoring.

Requirements
1) Target derivation
- Iterate inventory.LambdaInventory.Lambdas and collect every SQSTrigger.
- For each unique logical queue, resolve:
  - primary queue name: lesser-<queue>-<environment>
  - DLQ name: lesser-<queue>-dlq-<environment> unless trigger.DeadLetterQueue is set, then lesser-<override>-<environment>
  - Apply aliases exactly as createSQSQueues does (federation-delivery-queue -> federation-queue, push-delivery-queue -> push-notification-queue, import-processor-queue -> import-export-queue); scheduled-queue must be included.
- If ConsumeDeadLetterQueue is true, monitoring must surface the DLQ directly as a monitored queue (depth widgets and alarms still use the DLQ).

2) Metrics to emit (all 5m period)
- Primary queue:
  - ApproximateNumberOfMessagesVisible (Sum)
  - ApproximateNumberOfMessagesNotVisible (Sum)
  - ApproximateAgeOfOldestMessage (Maximum)
- DLQ (when it exists):
  - ApproximateNumberOfMessagesVisible (Sum) for DLQ depth
  - ApproximateAgeOfOldestMessage (Maximum) for DLQ age (optional widget; depth alarm is mandatory).

3) Widgets
- Per primary queue:
  - Graph 12x6: Visible (left) + NotVisible (right)
  - Graph 12x6: OldestMessageAge (left)
- Per DLQ (when present):
  - Graph 12x6: DLQ Visible (depth)
  - Graph 12x6: DLQ OldestMessageAge (optional but recommended)
- Place widgets on the existing MonitoringStack dashboard; ordering should follow inventory queue discovery to avoid drift.

4) Alarms
- DLQ depth: threshold >= 1 message, evaluationPeriods=1, treatMissingData=NOT_BREACHING, action=AlertTopic.
- Optional primary queue depth/age alarms: use AlarmConfigBuilder().GetConfiguration(env).SQS thresholds:
  - QueueDepthThreshold -> ApproximateNumberOfMessagesVisible
  - MessageAgeThresholdSeconds -> ApproximateAgeOfOldestMessage
  - EvaluationPeriods/DatapointsToAlarm from SQS config; TreatMissingData=NOT_BREACHING.
- Alarm names must include the logical queue and environment (e.g., lesser-<queue>-<env>-dlq-depth).

5) Integration points
- Add a helper in MonitoringStack to:
  - Build a resolved queue target struct {Logical, PrimaryName, DLQName, HasDLQ, MonitorDLQDirect bool (ConsumeDeadLetterQueue)} derived from inventory + createSQSQueues naming rules/aliases.
  - Construct metrics with DimensionsMap QueueName: <physical name>.
  - Add widgets to the existing dashboard instance.
  - Create alarms using AlarmConfigBuilder SQS thresholds; wire actions to AlertTopic.
- Ensure environment strings are passed through verbatim (dev, staging, production); do not emit prod when environment is production.

6) Validation guardrails (implementation guidance)
- Add a unit test under infra/cdk/stacks or constructs that:
  - Asserts every inventory SQSTrigger yields a monitored primary queue metric.
  - Asserts DLQ alarms exist when ConsumeDeadLetterQueue is true.
  - Asserts no hard-coded queue names outside the resolved map.

Acceptance Criteria
- MonitoringStack produces SQS metrics/widgets and DLQ depth alarms for every inventory-declared SQS trigger using the exact queue names created in stacks.
- No hard-coded or stale queue names; aliases and scheduled-queue are honored.
- DLQ depth alarms fire on >=1 message and publish to the AlertTopic.
- Optional depth/age alarms use AlarmConfigBuilder SQS thresholds and TreatMissingData=NOT_BREACHING.
- No new application wiring is introduced by monitoring.