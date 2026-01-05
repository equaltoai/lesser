# Lift Adoption Inventory (Lesser)

> Archived: moved from `docs/planning/` on 2025-12-27. Not actively maintained.

Status: tracked (living doc)  
Release phase: pre-release (prototype deployment; planned tear-down/rebuild)  
Last updated: 2025-12-22  
Lift version: `github.com/pay-theory/lift v1.0.81` (pinned in `go.mod`)  
Local dev override: root `go.mod` uses `replace github.com/pay-theory/lift => ../../lift` (adds DynamoDB stream runtime support; pending upstream release)  
CDK dev override: `infra/cdk/go.mod` uses `replace github.com/pay-theory/lift => ../../../../lift` (required for newer CDK constructs like `StaticSite`/`MediaCDN`)

## Policy / Principles

- **Use Lift for every AWS resource/trigger it supports** (CDK + runtime).
- **Use DynamORM for DynamoDB access** (no direct DynamoDB API calls in app code).
- **Pre-release**: prefer clean Lift-first replacements over in-place migrations; prototype stacks can be destroyed/recreated.
- If we intentionally keep a native AWS implementation, document the **reason and gap** here.

## Lift Capability Surface (v1.0.81)

### Runtime (Go)

- **HTTP routing** behind API Gateway (REST API v1 / HTTP API v2) via `lift.New()` + `app.Handle` / `app.GET` etc.
- **API Gateway REST API v1 response streaming (SSE)** via `lift.SSEResponse` (requires `-tags lambda.norpc`).
- **Trigger helpers**: `app.SQS`, `app.S3`, `app.EventBridge`.
- **DynamoDB streams**: `app.DynamoDB` + `(*lift.Context).DynamoDBRecords()` (local patch; pending upstream release).
- **WebSockets**: `lift.WebSocketContext` and Lift’s `pkg/streamer` wrapper around API Gateway Management API.
- **Durable EventBus**: `services.DynamoDBEventBus` (DynamoDB-backed; TTL; optional metrics; not AWS EventBridge).

### CDK Constructs (high-signal list)

Lift CDK provides constructs that cover most of Lesser’s “core serverless” primitives:

- **APIs**: `LiftRestAPI` (REST v1; supports per-method response streaming), `LiftAPI` (HTTP API v2), `WebSocketAPI`.
- **Lambda**: `LiftFunction` (+ wrappers like monitored/secure/ratelimited) and Lambda role helpers.
- **DynamoDB**: `LiftTable`, `StreamingTable`, `RateLimitTable`, `ConnectionTable`, `IdempotencyTable`, `EventBusTable`, etc.
- **Event wiring**: `SQSProcessor`, `DynamoStreamProcessor`, `EventBridgeHandler`, `LiftEventSourceMapping`.
- **CloudFront**: `StaticSite`, `FrontendDistribution`, `MediaCDN`, `HostRedirect`.
- **Security/monitoring**: `EnhancedSecurity`, `EnhancedMonitoring`, alarms helpers.
- **DNS/certs/domains**: `LiftHostedZone`, `LiftCertificate`, `LiftApiDomain`.

### Known Gaps / Caveats (important)

- **WebSocket custom domain mapping**: Lift’s `WebSocketAPI` construct does not manage `DomainName`/`ApiMapping`/Route53 alias records, so domain mapping stays native (`awsapigatewayv2.DomainName` + `awsapigatewayv2.ApiMapping` + Route53 records).
- **CloudFront publish/invalidation**: Lift provides infra constructs but does not ship an asset publish pipeline or invalidation worker (still native tooling).
- Lift is not a replacement for everything (MediaConvert, Bedrock, Rekognition, etc. remain native AWS SDK/CDK).

## Current Lesser Adoption Snapshot

### Infra (CDK)

- **Lift in use**: `LiftRestAPI` (`infra/cdk/constructs/api_routes.go`), `LiftFunction` (`infra/cdk/constructs/lambda_functions.go`), `LiftTable` (`infra/cdk/stacks/lesser_api_stack.go`), `LiftKMSKey`/`LiftLambdaRole` (`infra/cdk/stacks/shared_stack.go`), `LiftSQSQueue` + `LiftEventSourceMapping` (`infra/cdk/stacks/lesser_api_stack.go`, `infra/cdk/constructs/stream_processors.go`), and CloudFront constructs (`StaticSite`, `MediaCDN`).
- **Native CDK** still used for: WebSocket APIs + custom domain mapping, monitoring, and remaining Route53/ACM glue where Lift does not yet cover the full behavior (see caveats).

### Runtime (cmd/*)

- **Lift imported** in `36/40` Lambda `main.go` files (`cmd/cms-scheduler` has no `main.go`; not using Lift: `cmd/cloudfront-keygen`, `cmd/configure-instance`, `cmd/enhanced-federation-processor`, `cmd/init-deploy`).
- **Lift trigger helpers** are used by only a small subset:
  - `app.SQS`: `cmd/dlq-processor/main.go`, `cmd/federation-aggregator/main.go`, `cmd/media-processor/main.go`, `cmd/notification-processor/main.go`, `cmd/outbox/main.go`
  - `app.EventBridge`: `cmd/dlq-processor/main.go`
  - `app.DynamoDB`: `cmd/activity-processor/main.go`, `cmd/ai-processor/main.go`, `cmd/cost-aggregator/main.go`, `cmd/federation-timeseries/main.go`, `cmd/federation-tracker/main.go`, `cmd/metrics-aggregator/main.go`, `cmd/metrics-processor/main.go`, `cmd/ml-training-processor/main.go`, `cmd/moderation-processor/main.go`, `cmd/note-processor/main.go`, `cmd/report-trust-updater/main.go`, `cmd/search-indexer/main.go`, `cmd/severance-processor/main.go`, `cmd/status-indexer/main.go`, `cmd/stream-router/main.go`
- **WebSocket message delivery**:
  - GraphQL WS uses `lift.WebSocketContext` (`cmd/graphql-ws/main.go`).
  - Streaming/WebSocket broadcasting uses Lift `pkg/streamer` (no direct `apigatewaymanagementapi` usage).

## Inventory: Native AWS → Lift (Where Supported)

This is the actionable inventory of “we’re doing it native today, but Lift supports it”.

### Infra / CDK

| Area | Current (native) | Lift alternative | Where | Notes |
|---|---|---|---|---|
| REST API v1 + per-method SSE streaming | ✅ already Lift | `LiftRestAPI` | `infra/cdk/constructs/api_routes.go` | Keep as-is; this is the preferred pattern. |
| WebSocket APIs (streaming + graphql-ws) | `awsapigatewayv2.NewWebSocketApi` | `constructs.NewWebSocketAPI` | `infra/cdk/constructs/api_routes.go` | Lift can build the WS API + connection table, but domain mapping must remain native (see caveat). |
| Lambda creation | ✅ already Lift | `constructs.NewLiftFunction` (+ monitored/secure wrappers) | `infra/cdk/constructs/lambda_functions.go` | Creates Lambdas via Lift for consistent defaults and wiring. |
| DynamoDB tables (main, stream events, rate limit) | ✅ now Lift | `NewLiftTable` | `infra/cdk/stacks/lesser_api_stack.go` | Migrated to `NewLiftTable` to retain PITR/deletion-protection/removal-policy controls; GSIs still added via `Table.AddGlobalSecondaryIndex`. |
| SQS queue creation | ✅ now Lift | `constructs.NewLiftSQSQueue` | `infra/cdk/stacks/lesser_api_stack.go` | Primary consumers are wired by the Lift queue construct; DLQs still exist per queue. |
| SQS event source mappings | partially Lift | `constructs.NewLiftSQSQueue` (primary), `constructs.NewSQSProcessor` / `NewLiftEventSourceMapping` (optional) | `infra/cdk/stacks/lesser_api_stack.go`, `infra/cdk/constructs/stream_processors.go` | Primary mappings are created by `LiftSQSQueue`; DLQ consumption remains native `NewSqsEventSource` for now. |
| DynamoDB stream event source mappings | ✅ now Lift | `NewLiftEventSourceMapping` | `infra/cdk/constructs/stream_processors.go` | Runtime now routes stream events via Lift `app.DynamoDB` + `ctx.DynamoDBRecords()` (local patch; pending upstream release). |
| Scheduled EventBridge rules | ✅ now Lift | `constructs.NewEventBridgeHandler` | `infra/cdk/constructs/lambda_functions.go` | Inventory schedules are now created via Lift when functions are synthesized (rule names use `<repo>-<lambda>-schedule-<n>-<stage>`). |
| IAM roles + KMS key | ✅ now Lift | `NewLiftLambdaRole`, `NewLiftKMSKey` | `infra/cdk/stacks/shared_stack.go` | Shared stack now uses Lift helpers; app-specific wildcard policies are still attached in-repo. |
| Monitoring + alarms | `awscloudwatch` + `awssns` | `EnhancedMonitoring` | `infra/cdk/stacks/monitoring_stack.go`, `infra/cdk/constructs/alarm_config.go` | Adopt if we want a single monitoring posture across envs. |
| CloudFront: Auth UI static site | ✅ now Lift | `constructs.NewStaticSite` | `infra/cdk/constructs/auth_ui.go` | Pre-release: OK to replace resources; validate SPA routing behavior. |
| CloudFront: Media CDN | ✅ now Lift | `constructs.NewMediaCDN` | `infra/cdk/stacks/lesser_api_stack.go` | Lift uses OAC and can optionally enforce signed URLs via Key Groups. |

### Runtime / Lambda handlers

| Area | Current (native) | Lift alternative | Where | Notes |
|---|---|---|---|---|
| DynamoDB stream Lambda entrypoints | ✅ now Lift | `app.DynamoDB` + `ctx.DynamoDBRecords()` | `cmd/*-processor/main.go` (15 functions) | Removes the wrapper approach and standardizes trigger routing in Lift. |
| SQS-triggered Lambda entrypoints | direct `lambda.Start` + `events.SQSEvent` | `app.SQS(...)` (Lift runtime) | e.g. `cmd/enhanced-federation-processor/main.go`, `cmd/export-generator/main.go`, `cmd/import-processor/main.go`, `cmd/push-delivery/main.go` | Many SQS Lambdas already use Lift, but not necessarily the native `app.SQS` helper. |
| WebSocket message delivery | ✅ now Lift | Lift `pkg/streamer` or `lift.WebSocketContext` | `cmd/streaming/main.go`, `cmd/stream-router/main.go`, `pkg/streaming/*`, `pkg/websocket/subscriptions.go` | Migrated to Lift `pkg/streamer` (no direct `apigatewaymanagementapi` usage). |

## Migration Backlog (track status here)

P0 (high leverage / policy-sensitive):

- [x] CDK: migrate Lambda creation to `constructs.NewLiftFunction` (`infra/cdk/constructs/lambda_functions.go`).
- [x] Runtime: migrate WebSocket send/broadcast to Lift `pkg/streamer` / `lift.WebSocketContext` (`pkg/streaming/*`, `cmd/streaming/main.go`).
- [x] Runtime: migrate DynamoDB stream Lambdas to Lift runtime `app.DynamoDB` + `ctx.DynamoDBRecords()` (wrapper removed).

P1 (reduces boilerplate / improves consistency):

- [x] CDK: migrate SQS queues to `constructs.NewLiftSQSQueue` (`infra/cdk/stacks/lesser_api_stack.go`).
- [x] CDK: migrate event source mappings to Lift processors (`infra/cdk/constructs/stream_processors.go`).
- [x] CDK: migrate schedules to `constructs.NewEventBridgeHandler` (now created during Lambda synthesis in `infra/cdk/constructs/lambda_functions.go`).
- [x] CDK: migrate CloudFront distributions to Lift constructs (`infra/cdk/constructs/auth_ui.go`, `infra/cdk/stacks/lesser_api_stack.go`).

P2 (bigger schema/stack refactors; do after confidence is high):

- [x] CDK: migrate DynamoDB tables to `NewLiftTable` (`infra/cdk/stacks/lesser_api_stack.go`).
- [x] CDK: migrate IAM/KMS to `NewLiftKMSKey` + `NewLiftLambdaRole` (`infra/cdk/stacks/shared_stack.go`).
- [x] CDK: evaluate migrating monitoring to `NewEnhancedMonitoring` (kept inventory-driven monitoring; `EnhancedMonitoring` currently assumes Lift-owned resources + fixed alarm names) (`infra/cdk/stacks/monitoring_stack.go`).

## DynamoDB SDK Usage (DynamORM Policy Check)

This is not “Lift vs native”, but it is a recurring policy risk: files importing `github.com/aws/aws-sdk-go-v2/service/dynamodb` should be reviewed to ensure they **only** bootstrap clients for DynamORM, not call DynamoDB APIs directly.

Start list (not exhaustive, regenerate with commands below):

- `pkg/aws/initialization.go`
- `pkg/config/validator.go`
- `pkg/cost/dynamodb_wrapper.go`
- `pkg/cost/middleware.go`
- `pkg/cost/storage.go`
- `pkg/federation/routing/metrics.go`
- `pkg/moderation/advanced/reputation.go`
- `pkg/monitoring/health.go`
- `pkg/monitoring/dynamorm_integration.go`
- `pkg/monitoring/xray_integration.go`
- `pkg/observability/health.go`
- `pkg/translation/aws_translate.go`

## Regenerate / Update This Inventory

Useful repo-local commands (run from repo root):

- Lift usage in Lambda mains: `for f in cmd/*/main.go; do rg -q \"pay-theory/lift/pkg/lift\" \"$f\" && echo \"$f\"; done | wc -l`
- Lambdas not using Lift: `for f in cmd/*/main.go; do rg -q \"pay-theory/lift/pkg/lift\" \"$f\" || echo \"$f\"; done | sort`
- Native WebSocket management API usage: `rg -n \"aws-sdk-go-v2/service/apigatewaymanagementapi\" cmd pkg -S`
- DynamoDB SDK client imports: `rg -n \"aws-sdk-go-v2/service/dynamodb\\\"\" cmd pkg -S`
- Native CDK service usage: `rg -n \"aws-cdk-go/awscdk/v2/aws\" infra/cdk -S | sed -E 's/.*\\\"(github.com\\/aws\\/aws-cdk-go\\/awscdk\\/v2\\/aws[^\\\"]+)\\\".*/\\1/' | sort -u`
