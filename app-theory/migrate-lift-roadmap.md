# lesser: Lift + DynamORM Sunset → AppTheory + TableTheory Replacement Plan

Generated: 2026-01-31

This document defines the complete, codebase-wide replacement of:

- Lift (`github.com/pay-theory/lift`) → AppTheory (`github.com/theory-cloud/apptheory/runtime`)
- DynamORM **and** all direct AWS DynamoDB SDK usage → TableTheory (`github.com/theory-cloud/tabletheory`)

Lift and DynamORM are being sunsetted and must be removed from this repository. AppTheory and TableTheory are permanent
replacements. This migration is **not incremental**: there are no shims, no dual-running, no compatibility wrappers that
preserve Lift/DynamORM call sites, and no “slice” sequencing driven by production risk. The outcome is a repo that builds
and tests cleanly with Lift/DynamORM absent.

## Scope
- In scope:
  - Replace **all** Lift runtime usage (HTTP, WebSockets, DynamoDB streams, SQS, EventBridge, and any Lift-derived helpers).
  - Replace **all** DynamORM usage.
  - Replace **all** direct DynamoDB AWS SDK usage in repo code with TableTheory.
  - Replace Lift CDK constructs usage in `infra/cdk` (AppTheory CDK + native AWS CDK constructs).
  - Rename/remove first-party packages/directories that encode the legacy frameworks (`pkg/lift`, `pkg/storage/dynamorm`, etc.).
- Out of scope: feature refactors unrelated to removing Lift/DynamORM and direct DynamoDB SDK usage.

## Non-negotiable end state (definition of done)
- No `github.com/pay-theory/lift` dependency in:
  - `go.mod`
  - `infra/cdk/go.mod`
- No `github.com/pay-theory/dynamorm` dependency in:
  - `go.mod`
- No Go imports of Lift or DynamORM:
  - `rg -n "github.com/pay-theory/lift" --glob '*.go' -S .` returns 0 results
  - `rg -n "github.com/pay-theory/dynamorm" --glob '*.go' -S .` returns 0 results
- No direct DynamoDB SDK client usage in repo code (TableTheory only):
  - `rg -n "aws-sdk-go-v2/service/dynamodb" --glob '*.go' -S .` returns 0 results
- No first-party legacy framework namespaces:
  - `cmd/api/lift/`, `pkg/lift/`, `pkg/storage/dynamorm/`, `pkg/testing/lift/`, `pkg/testing/dynamorm/` do not exist in
    the final tree (move/rename/delete; do not keep compatibility copies).
- Repo compiles and tests:
  - `go test ./...` passes
  - `./lesser test` passes
  - `make verify` passes

## Destination (pinned)

These are the pinned destination frameworks for this repo. Keep these versions pinned until an intentional upgrade.

### AppTheory (pinned)
- Go module: `github.com/theory-cloud/apptheory@v0.5.0`
- Go runtime import: `github.com/theory-cloud/apptheory/runtime`
- Docs entrypoints:
  - `docs/getting-started.md`
  - `docs/migration/from-lift.md`
- Copy/paste:
  - `go get github.com/theory-cloud/apptheory@v0.5.0`
- Pinned docs:
  - `https://github.com/theory-cloud/AppTheory/blob/v0.5.0/docs/getting-started.md`
  - `https://github.com/theory-cloud/AppTheory/blob/v0.5.0/docs/migration/from-lift.md`

### TableTheory (pinned)
- Go module: `github.com/theory-cloud/tabletheory@v1.3.0`
- Docs entrypoints:
  - `docs/getting-started.md`
  - `docs/api-reference.md`
  - `docs/migration-guide.md`
  - `docs/struct-definition-guide.md` (tag rules)
- Copy/paste:
  - `go get github.com/theory-cloud/tabletheory@v1.3.0`
- Pinned docs:
  - `https://github.com/theory-cloud/TableTheory/blob/v1.3.0/docs/getting-started.md`
  - `https://github.com/theory-cloud/TableTheory/blob/v1.3.0/docs/api-reference.md`
  - `https://github.com/theory-cloud/TableTheory/blob/v1.3.0/docs/migration-guide.md`
  - `https://github.com/theory-cloud/TableTheory/blob/v1.3.0/docs/struct-definition-guide.md`

## Repo inventory (current, not optional)

### Lift usage (must be fully removed)
- Dependency evidence:
  - `go.mod`: `github.com/pay-theory/lift v1.0.82`
  - `infra/cdk/go.mod`: `github.com/pay-theory/lift v1.0.82`

- Lift app creation sites (`lift.New(...)`) that must become `apptheory.New(...)`:
  - `cmd/api/main.go`
  - `cmd/graphql/main.go`
  - `cmd/sse/main.go`
  - `cmd/graphql-ws/main.go` (uses `lift.WithWebSocketSupport()`)
  - `cmd/streaming/main.go` (uses `lift.WithWebSocketSupport()`)
  - `cmd/activity-processor/main.go` (DynamoDB stream)
  - `cmd/ai-processor/main.go` (DynamoDB stream)
  - `cmd/cost-aggregator/main.go` (DynamoDB stream)
  - `cmd/dlq-processor/main.go` (SQS + EventBridge)
  - `cmd/federation-aggregator/main.go` (SQS + EventBridge)
  - `cmd/federation-delivery/main.go` (SQS)
  - `cmd/federation-timeseries/main.go` (DynamoDB stream)
  - `cmd/federation-tracker/main.go` (DynamoDB stream)
  - `cmd/media-processor/main.go` (SQS)
  - `cmd/metrics-aggregator/main.go` (DynamoDB stream)
  - `cmd/metrics-processor/main.go` (DynamoDB stream)
  - `cmd/ml-training-processor/main.go` (DynamoDB stream)
  - `cmd/moderation-processor/main.go` (DynamoDB stream)
  - `cmd/note-processor/main.go` (DynamoDB stream)
  - `cmd/notification-processor/main.go` (SQS)
  - `cmd/objects/main.go` (HTTP routes)
  - `cmd/outbox/main.go` (HTTP routes + SQS)
  - `cmd/report-trust-updater/main.go` (DynamoDB stream)
  - `cmd/search-indexer/main.go` (DynamoDB stream)
  - `cmd/severance-processor/main.go` (DynamoDB stream)
  - `cmd/status-indexer/main.go` (DynamoDB stream)
  - `cmd/stream-router/main.go` (DynamoDB stream)
  - `cmd/trend-aggregator/main.go` (EventBridge schedule)
  - `cmd/websocket-cost-aggregator/main.go` (EventBridge schedule)

- Lift event-source registrations that must become AppTheory registrations:
  - DynamoDB streams (`app.DynamoDB(...)`):
    - `cmd/activity-processor/main.go`
    - `cmd/ai-processor/main.go`
    - `cmd/cost-aggregator/main.go`
    - `cmd/federation-timeseries/main.go`
    - `cmd/federation-tracker/main.go`
    - `cmd/metrics-aggregator/main.go`
    - `cmd/metrics-processor/main.go`
    - `cmd/ml-training-processor/main.go`
    - `cmd/moderation-processor/main.go`
    - `cmd/note-processor/main.go`
    - `cmd/report-trust-updater/main.go`
    - `cmd/search-indexer/main.go`
    - `cmd/severance-processor/main.go`
    - `cmd/status-indexer/main.go`
    - `cmd/stream-router/main.go`
  - SQS (`app.SQS(...)`):
    - `cmd/dlq-processor/main.go`
    - `cmd/federation-aggregator/main.go`
    - `cmd/federation-delivery/main.go`
    - `cmd/media-processor/main.go`
    - `cmd/notification-processor/main.go`
    - `cmd/outbox/main.go`
  - EventBridge (`app.EventBridge(...)`):
    - `cmd/websocket-cost-aggregator/main.go`
    - `cmd/cms-scheduler/main.go`
    - `cmd/dlq-processor/main.go`
    - `cmd/federation-aggregator/main.go`
    - `cmd/trend-aggregator/main.go`
  - WebSockets (`app.WebSocket(...)`):
    - `cmd/graphql-ws/main.go`
    - `cmd/streaming/main.go`
  - SSE:
    - `cmd/sse/main.go` uses `lift.SSEResponse(...)`

- Lift-first helper packages that must be removed or rewritten on AppTheory types:
  - `pkg/lift/` (all files)
  - `pkg/testing/lift/`
  - `pkg/testing/mocks/lift_mock.go`
  - `pkg/deploy/naming/naming.go` must not import `github.com/pay-theory/lift/pkg/naming` (removed; keep it removed)
  - `cmd/api/handlers/` (renamed from `cmd/api/lift/`; still uses Lift types until runtime migration)
  - `cmd/api/routes.go` (renamed from `cmd/api/routes_lift.go`)
  - `cmd/api/handlers.go` (renamed from `cmd/api/lift_handlers.go`)
  - `tools/openapi/*` references `cmd/api/handlers` as the handler package path
  - `scripts/add-panic-recovery.sh` checks for `lift.New()` and injects Lift middleware

### DynamORM usage (must be fully removed)
- Dependency evidence:
  - `go.mod`: `github.com/pay-theory/dynamorm v1.0.39`

- Primary implementation namespaces that must not exist in the final tree:
  - `pkg/storage/dynamorm/`
  - `pkg/testing/dynamorm/`

- Model tags to eliminate:
  - `rg -n "dynamorm:\\\"" -S pkg cmd graph` returns many results across:
    - `pkg/storage/models/*`
    - `pkg/storage/dynamorm/*`
    - other supporting packages (monitoring, services, streaming examples)

- Construction points that must become TableTheory:
  - `cmd/api/main.go` (lambda-optimized DB init)
  - `cmd/activity-processor/main.go` (DB init + repositories)
  - `pkg/storage/dynamorm/*` (all DB init / adapters / repositories)
  - `graph/query_resolvers_cms.go`, `graph/mutation_resolvers_cms.go` (imports DynamORM types/errors)

### Native DynamoDB AWS SDK usage (must be fully removed)

TableTheory is the only DynamoDB access layer in this repo. Any `.go` file importing
`github.com/aws/aws-sdk-go-v2/service/dynamodb` must be rewritten to use TableTheory instead.

Known current import sites:
- `cmd/lesser/bootstrap.go`
- `cmd/lesser/up.go`
- `cmd/owner-bootstrap/main.go`
- `graph/resolver.go`
- `pkg/aws/initialization.go`
- `pkg/config/validator.go`
- `pkg/cost/dynamodb_wrapper.go`
- `pkg/cost/middleware.go`
- `pkg/cost/storage.go`
- `pkg/federation/routing/metrics.go`
- `pkg/monitoring/*` (health/integration helpers)
- `pkg/observability/health.go`
- `pkg/translation/aws_translate.go`
- plus tests for the above files

## Execution steps (single branch / single PR)

### 0) Pre-flight
1. Create a branch dedicated to the replacement.
2. Capture a baseline of current test state (even if failing) so you can see progress:
   - `go test ./...`
   - `./lesser test`
   - `make verify`

### 1) Remove Lift/DynamORM from module dependencies
1. In the root module:
   - Remove `github.com/pay-theory/lift` from `go.mod`.
   - Remove `github.com/pay-theory/dynamorm` from `go.mod`.
   - Ensure pinned destination deps exist:
     - `github.com/theory-cloud/apptheory v0.5.0`
     - `github.com/theory-cloud/tabletheory v1.3.0`
   - Run: `go mod tidy`
2. In the CDK module:
   - Remove `github.com/pay-theory/lift` from `infra/cdk/go.mod`.
   - Add `github.com/theory-cloud/apptheory v0.5.0` to `infra/cdk/go.mod` (for `cdk-go/apptheorycdk`).
   - Run:
     - `cd infra/cdk && go mod tidy`
     - `cd ../..`

### 2) Replace Lift CDK constructs with AppTheory CDK + native AWS CDK
This step removes all `liftcdk` imports from `infra/cdk`.

1. Replace the import root:
   - Lift: `liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"`
   - AppTheory: `apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"`
2. Replace constructs used by `infra/cdk`:
   - `liftcdk.LiftRestAPI` → `apptheorycdk.AppTheoryRestApi`
   - `liftcdk.NewLiftRestAPI(...)` → `apptheorycdk.NewAppTheoryRestApi(...)`
   - `liftcdk.NewLiftFunction(...)` → `apptheorycdk.NewAppTheoryFunction(...)` (use `.Fn()` to get `awslambda.Function`)
   - `liftcdk.NewEventBridgeHandler(...)` → `apptheorycdk.NewAppTheoryEventBridgeHandler(...)`
   - `liftcdk.NewLiftEventSourceMapping(...)` → `apptheorycdk.NewAppTheoryDynamoDBStreamMapping(...)`
   - `liftcdk.NewLiftTable(...)` → `apptheorycdk.NewAppTheoryDynamoTable(...)` (use `.Table()`)
   - `liftcdk.NewLiftSQSQueue(...)` → `apptheorycdk.NewAppTheoryQueueProcessor(...)` + explicit DLQ and grants using `awssqs.QueueProps`
3. Replace Lift-only constructs that have no AppTheory equivalent using AWS CDK directly:
   - `liftcdk.NewLiftKMSKey(...)` → `awskms.NewKey(...)` + `awskms.NewAlias(...)` if needed
   - `liftcdk.NewLiftLambdaRole(...)` → `awsiam.NewRole(...)` + explicit managed policies/inline policies
   - `liftcdk.NewPathRoutedFrontendDistribution(...)` → implement an equivalent construct inside `infra/cdk/constructs/` using `awscloudfront`, `awscloudfrontorigins`, `awss3`, and the existing `localconstructs.NewFrontendStaticResponseHeadersPolicy(...)`
   - `liftcdk.NewMediaCDN(...)` → implement an equivalent construct inside `infra/cdk/constructs/` using `awscloudfront`, `awss3`, and ACM certs already provisioned in `lesser_api_stack.go`

### 3) Replace Lift runtime usage with AppTheory runtime

#### 3.1) Rename/remove legacy framework namespaces
Do these renames/removals up front so new code does not keep the old names:
1. Rename `cmd/api/lift/` → `cmd/api/handlers/` and update all imports accordingly.
2. Rename `cmd/api/routes_lift.go` → `cmd/api/routes.go` and update references/tests accordingly.
3. Rename `cmd/api/lift_handlers.go` → `cmd/api/handlers.go` and update references/tests accordingly.
4. Rename API tests to remove `lift` from filenames:
   - `cmd/api/routes_lift_test.go` → `cmd/api/routes_test.go`
   - `cmd/api/routes_lift_manifest_test.go` → `cmd/api/routes_manifest_test.go`
   - `cmd/api/lift_handlers_test.go` → `cmd/api/handlers_test.go`
   - `cmd/api/lift_handlers_round12_test.go` → `cmd/api/handlers_round12_test.go`
5. Delete the following legacy packages after their replacements exist:
   - `pkg/lift/`
   - `pkg/testing/lift/`
   - `pkg/testing/mocks/lift_mock.go`
6. Remove the Lift naming dependency:
   - Update `pkg/deploy/naming/naming.go` to remove `github.com/pay-theory/lift/pkg/naming`.
   - Replace `liftnaming.SanitizeS3BucketName(...)` with a local S3-bucket sanitization implementation.

#### 3.2) Replace Lift app creation and Lambda entrypoint wiring everywhere
For every Lift-based Lambda entrypoint (see the list under “Lift app creation sites”):
1. Replace imports:
   - `github.com/pay-theory/lift/pkg/lift` → `apptheory "github.com/theory-cloud/apptheory/runtime"`
   - Any Lift middleware imports → remove or replace with AppTheory equivalents (AppTheory includes request-id + recovery in-tier).
2. Replace app construction:
   - Lift: `app := lift.New(...)`
   - AppTheory: `app := apptheory.New(apptheory.WithTier(apptheory.TierP2))`
   - If WebSockets are used: add `apptheory.WithWebSocketSupport()`
3. Replace Lambda startup:
   - Lift: `lambda.Start(app.HandleRequest)`
   - AppTheory: `lambda.Start(app.HandleLambda)`

#### 3.3) Replace HTTP route handlers (Lift Context → AppTheory Context)
1. Replace handler signatures:
   - Lift: `func(ctx *lift.Context) error`
   - AppTheory: `func(ctx *apptheory.Context) (*apptheory.Response, error)`
2. Replace responses:
   - JSON: `return apptheory.JSON(status, value), nil`
   - Text: `return apptheory.Text(status, "text"), nil`
   - Binary: `return apptheory.Binary(status, bytes, contentType), nil`
3. Replace request parsing:
   - JSON body: `v, err := ctx.JSONValue()` then `json.Unmarshal(...)`
   - Params: `ctx.Param("name")` is available in AppTheory
   - Request-scoped state: `ctx.Set(key, value)` / `ctx.Get(key)`

#### 3.4) Replace DynamoDB stream handlers (Lift batch extraction → AppTheory per-record handler)
Lift’s common pattern in this repo:

```go
_ = app.DynamoDB("*", func(ctx *lift.Context) error {
	records, err := ctx.DynamoDBRecords()
	...
	return processor.HandleStream(ctx.Request.Context(), events.DynamoDBEvent{Records: records})
})
```

AppTheory’s contract surface is per-record:
- Register: `app.DynamoDB(tableName, handler)`
- Handler signature: `func(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error`
- Entrypoint: `app.ServeDynamoDBStream(ctx, events.DynamoDBEvent)` (or `app.HandleLambda`)

Required refactor:
1. Refactor each stream processor to a record-level entrypoint:
   - Create `HandleRecord(ctx context.Context, record events.DynamoDBEventRecord) error` and move record processing logic there.
2. Register the AppTheory handler by **exact** table name:
   - AppTheory routes by the table name extracted from `record.EventSourceArn` and matches by string equality.
   - AppTheory does not support `"*"` / glob / prefix matching for DynamoDB Streams.
   - In this repo, the stream processors are wired to the main table stream; register `cfg.DynamoTableName` (env `DYNAMODB_TABLE`).
   - `app.DynamoDB(cfg.DynamoTableName, func(ev *apptheory.EventContext, rec events.DynamoDBEventRecord) error { return processor.HandleRecord(ev.Context(), rec) })`

#### 3.5) Replace SQS handlers (Lift batch extraction → AppTheory per-message handler)
AppTheory SQS surface:
- Register: `app.SQS(queueName, handler)`
- Handler signature: `func(ctx *apptheory.EventContext, msg events.SQSMessage) error`
- Entrypoint: `app.ServeSQS(ctx, events.SQSEvent)` (or `app.HandleLambda`)

Required refactor:
1. Convert each Lift handler that expects an `events.SQSEvent` batch into a per-message handler.
2. Remove any usage of Lift context record extraction (`ctx.SQSRecords()` patterns).
3. Register the AppTheory handler by **exact** queue name:
   - AppTheory routes by the queue name extracted from `record.EventSourceARN` and matches by string equality.
   - AppTheory does not support `"*"` / glob / prefix matching for SQS queue routing.
   - For each SQS-triggered Lambda, compute and register the **deployed queue name**:
     - Queue name = `naming.ResourceNameWithApp(os.Getenv("APP_NAME"), "<logical-queue>", os.Getenv("ENVIRONMENT"))`
     - Canonical logical queue names are the values in `infra/cdk/inventory/lambdas.go` under `SQSTriggers`.
   - For DLQ consumption (`dlq-processor`), register the DLQ queue names (this repo’s CDK defaults to `"<logical>-dlq"`):
     - `enhanced-federation-queue-dlq`
     - `export-processor-queue-dlq`
     - `federation-aggregator-queue-dlq`
     - `federation-delivery-queue-dlq`
     - `import-processor-queue-dlq`
     - `media-processor-queue-dlq`
     - `notification-processor-queue-dlq`
     - `push-delivery-queue-dlq`

#### 3.6) Replace EventBridge handlers
AppTheory EventBridge surface:
- Register by rule: `app.EventBridge(apptheory.EventBridgeRule(ruleName), handler)`
- Register by pattern: `app.EventBridge(apptheory.EventBridgePattern(source, detailType), handler)`
- Entrypoint: `app.ServeEventBridge(ctx, events.EventBridgeEvent)` (or `app.HandleLambda`)

Required refactor:
1. Replace Lift `app.EventBridge("...*", func(ctx *lift.Context) error { ... })` style handlers with AppTheory selectors.
2. Register by **exact** rule name for scheduled Lambdas:
   - AppTheory matches by exact rule name derived from `event.Resources[*]` ARNs (string equality).
   - AppTheory does not support `"*"` / glob / prefix matching for rule names.
   - In this repo, schedule rule names are deterministic and are built as:
     - `naming.ResourceNameWithApp(os.Getenv("APP_NAME"), "<lambda-name>-schedule-0", os.Getenv("ENVIRONMENT"))`
   - Scheduled Lambdas to update:
     - `cms-scheduler` → rule `cms-scheduler-schedule-0`
     - `dlq-processor` → rule `dlq-processor-schedule-0`
     - `federation-aggregator` → rule `federation-aggregator-schedule-0`
     - `trend-aggregator` → rule `trend-aggregator-schedule-0`
     - `websocket-cost-aggregator` → rule `websocket-cost-aggregator-schedule-0`

#### 3.7) Replace WebSocket handlers
AppTheory WebSocket surface:
- Register routes: `app.WebSocket("$connect", handler)` etc.
- Handler signature: `func(ctx *apptheory.Context) (*apptheory.Response, error)`
- Entrypoint: `app.ServeWebSocket(ctx, events.APIGatewayWebsocketProxyRequest)` (or `app.HandleLambda`)

Inside handlers:
- `ws := ctx.AsWebSocket()`
- `ws.SendMessage(...)` / `ws.SendJSONMessage(...)`

#### 3.8) Replace SSE responses
Replace:
- Lift: `lift.SSEResponse(ctx, eventCh)`
- AppTheory: `return apptheory.SSEStreamResponse(ctx.Context(), 200, eventCh)`

### 4) Replace DynamORM + direct DynamoDB SDK usage with TableTheory

#### 4.1) Replace TableTheory model tags
For every struct tag currently using `dynamorm:"..."`, replace with TableTheory’s `theorydb:"..."`.

Mechanical mapping rules (do not change key/index names):
- `dynamorm:"pk"` → `theorydb:"pk"`
- `dynamorm:"sk"` → `theorydb:"sk"`
- `dynamorm:"ttl"` → `theorydb:"ttl"`
- `dynamorm:"version"` → `theorydb:"version"`
- `dynamorm:"attr:<name>"` → `theorydb:"attr:<name>"`
- `dynamorm:"index:<index>,pk"` → `theorydb:"index:<index>,pk"`
- `dynamorm:"index:<index>,sk"` → `theorydb:"index:<index>,sk"`
- `dynamorm:"naming:camelCase"` → `theorydb:"naming:camelCase"`

Reference: TableTheory tag spec is documented in `docs/struct-definition-guide.md` (pinned above).

#### 4.2) Replace DB initialization
1. Remove all DynamORM client construction:
   - `dynamorm.GetClient(...)`
   - `dynamorm.NewLambdaOptimizedClient(...)`
   - `dynamormCore.DB` usage
2. Replace with TableTheory initialization:
   - Lambda: `db, err := tabletheory.NewLambdaOptimized()`
   - Non-Lambda/local: `db, err := tabletheory.New(session.Config{Region: ..., Endpoint: ...})`
3. Update all call sites to use `tabletheory.DB` / `*tabletheory.LambdaDB` instead of DynamORM’s DB interface.

#### 4.3) Replace DynamORM errors with TableTheory errors
1. Replace imports:
   - `github.com/pay-theory/dynamorm/pkg/errors` → `github.com/theory-cloud/tabletheory/pkg/errors`
2. Replace helpers:
   - `dynamormerrors.IsNotFound(err)` → `tabletheoryerrors.IsNotFound(err)`
   - `dynamormerrors.IsConditionFailed(err)` → `tabletheoryerrors.IsConditionFailed(err)`
3. Update any sentinel comparisons to use `errors.Is(err, tabletheoryerrors.ErrItemNotFound)` etc.

#### 4.4) Replace DynamORM repositories and adapters
1. Delete the DynamORM implementation tree after porting:
   - `pkg/storage/dynamorm/` (adapter, client, repositories, migrations, patterns)
2. Create `pkg/storage/tabletheory/` as the TableTheory-only storage implementation namespace:
   - Move/port the necessary client init, adapters, and shared helpers from `pkg/storage/dynamorm/` into `pkg/storage/tabletheory/`.
   - Update any imports that referenced `pkg/storage/dynamorm/...` to `pkg/storage/tabletheory/...`.
3. Update repository factories:
   - `pkg/storage/factory/*` must construct repositories over TableTheory DB.

#### 4.5) Replace raw DynamoDB AWS SDK usage with TableTheory
For every `.go` file importing `github.com/aws/aws-sdk-go-v2/service/dynamodb`:
1. Remove the DynamoDB client.
2. Replace the operation with TableTheory’s equivalents:
   - CRUD/query/update: `db.Model(...).Where(...).First/All/Create/Update/Delete`
   - Schema/table checks: `db.EnsureTable(model)` / `db.AutoMigrate(models...)` / `db.CreateTable(model)`
   - Stream image unmarshal: `tabletheory.UnmarshalStreamImage(...)`

#### 4.6) Replace DynamORM mocks in tests with TableTheory mocks
1. Replace `dynamormmocks.*` usage with `github.com/theory-cloud/tabletheory/pkg/mocks`.
2. Update test harnesses in:
   - `graph/round12_test_harness_test.go`
   - any `pkg/testing/dynamorm/*` helpers

### 5) Tooling + docs cleanup (complete removal)
1. Replace `tools/openapi/*` assumptions about `cmd/api/handlers` package path; keep it aligned as directories move during the migration.
2. Remove or rewrite `tools/migrate_lift_storage_to_repos.go` (it encodes Lift+DynamORM concepts and must not remain in a Lift-free repo).
3. Remove or rewrite `scripts/add-panic-recovery.sh` to target AppTheory (`apptheory.New(...)`) and AppTheory middleware patterns.
4. Update repository documentation that instructs using Lift CDK or Lift runtime:
   - `docs/guides/CLIENT_APP_GUIDE.md` (Lift CDK references)

## Final verification commands (required)
- Dependency hygiene:
  - `go mod tidy`
  - `cd infra/cdk && go mod tidy && cd ../..`
- No legacy imports:
  - `rg -n "github.com/pay-theory/lift" --glob '*.go' -S .`
  - `rg -n "github.com/pay-theory/dynamorm" --glob '*.go' -S .`
  - `rg -n "aws-sdk-go-v2/service/dynamodb" --glob '*.go' -S .`
- Tests:
  - `go test ./...`
  - `./lesser test`
  - `make verify`
