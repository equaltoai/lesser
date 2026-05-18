# Inventory Schema and Defaults

This package defines the machine-readable Lambda inventory used by specs 02–07 to eliminate drift across code, packaging, CDK wiring, and monitoring.

## Schema Overview
- `Inventory` — root document containing `Defaults` and the full `Lambdas` slice.
- `LambdaSpec`
  - `Name` — canonical Lambda name (matches `Makefile LAMBDAS` and `bin/<name>.zip`).
  - `Type` — one of `api-http`, `api-ws`, `processor-stream`, `processor-sqs`, `processor-scheduled`, `hybrid`.
  - `Role` — `basic` or `encryption` IAM class.
  - `HTTPRoutes` — list of `{Method, Path}` owned by the Lambda.
  - `SQSTriggers` — `{queue, deadLetterQueue, batchSize, maxBatchingWindowSeconds, enablePartialFailure}`.
  - `StreamTriggers` — `{sourceTable|streamArn, poisonRecordQueue, batchSize, maxBatchingWindowSeconds, parallelizationFactor, startingPosition, maxRetryAttempts, maxRecordAgeSeconds, enableBisectOnError, reportBatchItemFailures}`.
  - `ScheduleTriggers` — `{expression (cron|rate), input}`.
  - `Overrides` — per-Lambda overrides for memory/timeout/log retention.
  - `RequiredEnvVars` — non-baseline env var slots that must be provided by configuration or secrets.
- `OperationalDefaults` — baseline operational settings applied unless overridden.

All structs carry `json` and `yaml` tags for machine consumption.

## Defaults
- `BaselineDefaults` (non-production Lift-aligned): `memoryMb=512`, `timeoutSeconds=30`, `logRetentionDays=7`.
- `ProductionDefaults` (production Lift-aligned): `memoryMb=3008`, `timeoutSeconds=30`, `logRetentionDays=30`.
Use environment-specific config to override as needed; per-Lambda overrides in `LambdaSpec.Overrides` take precedence.

## Downstream Consumption (Specs 02–07)
- **Spec 02 (CDK Lambda Definitions)**: Generate one `awslambda.Function` per `LambdaSpec`, apply `Role`, map assets `bin/<name>.zip`, and apply `Overrides` over `Defaults`.
- **Spec 03 (API Gateway + Federation Routing)**: Emit HTTP routes from `HTTPRoutes` for API Gateway wiring; enforce ownership for federation endpoints.
- **Spec 04 (Event Sources Wiring)**: Emit SQS, stream, and schedule event sources from `SQSTriggers`, `StreamTriggers`, and `ScheduleTriggers` with batch/window/position settings.
- **Spec 05 (Env Var Contract Alignment)**: Reconcile `RequiredEnvVars` with runtime configuration/secrets; surface gaps before deploy.
- **Spec 06 (Monitoring Stack Consistency)**: Use `Type` and `Role` to attach monitoring/alarms and retention policies consistently.
- **Spec 07 (Drift Prevention and Proof)**: Compare inventory against `Makefile LAMBDAS`, CDK outputs, and monitoring definitions to prove set equality.

## Usage Notes
- Keep the inventory exhaustive and exact for the product Lambdas defined in Spec 01 (the `Makefile` `LAMBDAS` set).
- Prefer explicit trigger declarations over implicit catch-alls to avoid routing and wiring drift.
- Avoid CDK-specific types in the schema; this package stays pure-data so other tools (linting, validation, codegen) can consume it without AWS dependencies.
