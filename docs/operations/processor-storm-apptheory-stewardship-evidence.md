# Processor storm AppTheory stewardship evidence packet

Date: 2026-05-18
Issue: [#1008](https://github.com/equaltoai/lesser/issues/1008) — Project 34 M5.3
Parent: [#994](https://github.com/equaltoai/lesser/issues/994) — durable prevention after incident stabilization

This packet is a local evidence and decision artifact. It does **not** file an AppTheory, TableTheory, or
framework issue. It records the current facts so the creator can decide whether any framework stewardship ask is
worth sending after the product incident work is stable.

## Decision status

- Creator approval to file an external/framework issue: **not granted in this packet**.
- External/framework issues filed from this packet: **none**.
- Deploy, source-mapping mutation, EventBridge mutation, processor re-enable/disable, and M4 recovery/backfill:
  **not authorized and not performed**.

## Summary decision table

| Area | Current classification | Evidence threshold | Recommended action now |
| --- | --- | --- | --- |
| Storage bootstrap ambiguity | Lesser product architecture | Product code had ambiguous fallback/default-init behavior and PR #1013 introduced `pkg/lambdastorage` plus audit gates | No framework issue |
| Eager secret resolution in `config.Get()` | Lesser product architecture | Product config loaded unrelated secrets during generic processor cold starts; PR #1013 made resolution lazy at auth/trust call sites | No framework issue |
| TableTheory | No current stewardship need | No PK/SK/GSI, TableTheory tag, query-builder, or versioning limitation is implicated by the current evidence | No TableTheory issue |
| AppTheory CDK stream `OnFailure` destination | Factual AppTheory CDK feature-gap candidate, now strategically optional | AppTheory v1.6.0 stream mapping props expose retry/max-age/bisect/partial-failure knobs but not `OnFailure`; AWS CDK exposes it; Lesser now routes around this locally | Creator decision: file only if framework coherence is worth the cost |
| AppTheory runtime error opacity | Insufficient evidence | Current evidence shows generic `apptheory: event workload failed`, but fuller `federation-aggregator` logs proving handler errors are not surfaced are absent | Do not file now; hold for Ops log evidence |

## Product architecture issues already handled inside Lesser

### Explicit product storage bootstrap

Project 34 M5.1/#1006 was product-owned. PR #1013 added an explicit storage bootstrap package and moved
storage-dependent Lambda startup away from continuing after ambiguous `common.InitializeWithDefaults()` failures.
Relevant paths:

- `pkg/lambdastorage/bootstrap.go`
- `pkg/lambdastorage/bootstrap_test.go`
- `tools/audit_gates/main.go` (`checkNoDefaultInitStorageContinuation`)
- `tools/audit_gates/default_init_storage_test.go`
- migrated `cmd/*/main.go` storage-dependent startup paths

This does not point to AppTheory or TableTheory. The product needed a clearer Lambda storage dependency contract.

### Side-effect-light config and lazy secrets

Project 34 M5.2/#1007 was also product-owned. PR #1013 changed `config.Get()` so generic processor cold starts do
not fetch unrelated JWT / instance / lesser-host secrets, and moved secret resolution to explicit auth/trust call
sites.
Relevant paths:

- `pkg/config/config.go` (`ResolveJWTSecret`, `ResolveInstanceAPIKey`, `ResolveLesserHostInstanceKey`)
- `pkg/config/config_secrets_test.go`
- `pkg/auth/middleware.go`
- `pkg/auth/service.go`
- `pkg/services/authentication.go`
- `cmd/api/handlers/helpers.go` (`createOAuthService` fail-closed lazy JWT resolution)

This does not point to AppTheory or TableTheory. The product needed explicit capability methods and fail-closed
callers.

## TableTheory assessment

Current Project 34 evidence does not show a TableTheory limitation:

- no single-table PK/SK pattern changed;
- no GSI shape changed;
- no TableTheory struct tags changed;
- no query-builder limitation was needed to contain or prevent the storm;
- no optimistic-concurrency/versioning issue was implicated.

Recommendation: **do not file a TableTheory stewardship issue** for this incident unless new evidence ties the storm
or recovery work to a concrete TableTheory tag, query, or concurrency gap.

## AppTheory CDK `OnFailure` candidate

### Observed facts

Lesser's product policy after M2 is that stream processors need finite retry, finite max record age, and a poison
record destination. Current inventory and tests now express that product policy:

- `infra/cdk/inventory/types.go` — `StreamTrigger` includes `PoisonRecordQueue`, `MaxRetryAttempts`,
  `MaxRecordAgeSeconds`, `EnableBisectOnError`, and `ReportBatchItemFailures`.
- `infra/cdk/inventory/lambdas_test.go` — stream triggers must declare finite retry/age and poison destination.
- `infra/cdk/constructs/trigger_wiring_test.go` — synthesized event-source mappings are checked for destination
  config and processor guardrail settings.

AppTheory v1.6.0 exposes a DynamoDB stream mapping construct, but its props do not include an `OnFailure` /
destination field:

```bash
go list -m -f '{{.Version}} {{.Dir}}' github.com/theory-cloud/apptheory
grep -n 'type AppTheoryDynamoDBStreamMappingProps' \
  "$(go list -m -f '{{.Dir}}' github.com/theory-cloud/apptheory)/cdk-go/apptheorycdk/AppTheoryDynamoDBStreamMappingProps.go"
```

The inspected v1.6.0 Go props include:

- `Consumer`
- `Table`
- `BatchSize`
- `BisectBatchOnError`
- `MaxBatchingWindow`
- `MaxRecordAge`
- `ParallelizationFactor`
- `ReportBatchItemFailures`
- `RetryAttempts`
- `StartingPosition`

The v1.6.0 TypeScript source at
`$(go env GOMODCACHE)/github.com/theory-cloud/apptheory@v1.6.0/cdk/lib/dynamodb-stream-mapping.ts` similarly
passes those fields into `lambdaEventSources.DynamoEventSource` and does not expose `onFailure`.

AWS CDK v2.254.0 does expose `OnFailure` on DynamoDB event sources:

```bash
grep -n 'OnFailure' \
  "$(go env GOMODCACHE)/github.com/aws/aws-cdk-go/awscdk/v2@v2.254.0/awslambdaeventsources/DynamoEventSourceProps.go"
```

### Current Lesser workaround

Lesser no longer blocks on this AppTheory gap. It keeps the inventory-driven stream policy but uses the native AWS
CDK event source only where the poison-record destination must be attached:

- `infra/cdk/constructs/stream_processors.go` creates the poison queue with `apptheorycdk.NewAppTheoryQueue`.
- The same file uses `awslambdaeventsources.DynamoEventSourceProps{ OnFailure: awslambdaeventsources.NewSqsDlq(...) }`
  and `handler.AddEventSource(awslambdaeventsources.NewDynamoEventSource(...))`.

### Draft framework-feedback signal, not filed

Target framework: AppTheory CDK (`github.com/theory-cloud/apptheory` v1.6.0)

Concern: Lesser wants to express its stream processor guardrail policy through AppTheory's canonical DynamoDB stream
mapping construct, including poison-record destination support.

Ideal AppTheory support:

```go
apptheorycdk.NewAppTheoryDynamoDBStreamMapping(scope, jsii.String("MetricsProcessorStream"), &apptheorycdk.AppTheoryDynamoDBStreamMappingProps{
    Consumer:                handler,
    Table:                   table,
    StartingPosition:        awslambda.StartingPosition_LATEST,
    RetryAttempts:           jsii.Number(3),
    MaxRecordAge:            awscdk.Duration_Hours(jsii.Number(2)),
    BisectBatchOnError:      jsii.Bool(true),
    ReportBatchItemFailures: jsii.Bool(true),
    OnFailure:               awslambdaeventsources.NewSqsDlq(poisonQueue), // illustrative API shape
})
```

Current workaround:

```go
// AppTheoryDynamoDBStreamMapping currently has no OnFailure destination prop.
// Keep the inventory-driven shape but use the AWS CDK DynamoEventSource so
// poison records are captured instead of being retried until stream expiry.
eventSourceProps := &awslambdaeventsources.DynamoEventSourceProps{
    StartingPosition: startPos,
    OnFailure:        awslambdaeventsources.NewSqsDlq(poisonQueue),
}
handler.AddEventSource(awslambdaeventsources.NewDynamoEventSource(table, eventSourceProps))
```

Cost of the workaround:

- Code complexity: Lesser carries a mixed AppTheory/AWS-CDK stream mapping path.
- Test burden: Lesser must prove destination wiring in its own CDK tests.
- Performance impact: none expected; this is infrastructure expression, not runtime path behavior.
- Maintenance drag: future AppTheory stream mapping improvements must be compared against the local workaround.

Scope of the gap:

- Specific to Lesser? No; any AppTheory consumer that needs DynamoDB stream poison-record destinations would need
  the same escape hatch.
- Incident-blocking? No; Lesser has a working product-side workaround after M2.

Recommendation: file this as an AppTheory CDK feature request only if the creator values restoring canonical
AppTheory construct usage more than the cost of adding framework work during incident stabilization.

## AppTheory runtime observability candidate

### What current evidence shows

Arch's incident analysis noted generic errors such as `apptheory: event workload failed` around
`federation-aggregator`. AppTheory v1.6.0 intentionally sanitizes unsafe event-workload errors to that generic
message unless the error is explicitly marked safe:

- `$(go env GOMODCACHE)/github.com/theory-cloud/apptheory@v1.6.0/runtime/event_workloads.go`
  - `eventWorkloadFailedMessage = "apptheory: event workload failed"`
  - `sanitizeEventWorkloadError` returns the generic error for non-safe errors
- `$(go env GOMODCACHE)/github.com/theory-cloud/apptheory@v1.6.0/runtime/aws_eventsources.go`
  - `ServeEventBridge` and DynamoDB stream helpers record event observability on sanitized failures
- AppTheory also exposes hook-based observability via
  `apptheory.WithObservability(apptheory.ObservabilityHooks{ Log: ..., Metric: ..., Span: ... })`.

### Evidence that is still missing

This packet does **not** include fuller `federation-aggregator` CloudWatch log context showing whether the underlying
handler error was logged before AppTheory returned the generic sanitized error. Without that evidence, the generic
message could be expected safe-error behavior rather than a framework observability bug.

Recommendation: **do not file a runtime observability issue now**. Reconsider only after Ops provides raw log context
for at least one failed `federation-aggregator` scheduled invocation, including handler-level logs immediately before
and after the AppTheory error.

## Creator decision options

1. **No framework issue now** — accept this packet as the M5.3 outcome; keep the AppTheory CDK gap documented locally
   and revisit on a future AppTheory bump.
2. **File AppTheory CDK `OnFailure` feature request only** — justified by current source evidence, but strategic
   rather than incident-blocking because Lesser has a local workaround.
3. **Hold runtime observability for more logs** — recommended unless Ops supplies fuller `federation-aggregator`
   evidence.
4. **File both** — not recommended from current evidence; only reasonable if creator explicitly accepts the thin
   runtime evidence threshold and the distraction cost.

## Recommended decision

Choose option 1 now, or option 2 if the creator wants an AppTheory stewardship ask despite the local workaround.
Do not file a runtime observability issue without fuller logs. Do not file any external/framework issue without an
explicit creator decision recorded on #1008 or in the project thread.
