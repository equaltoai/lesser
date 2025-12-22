# Cost Tracking Findings (Lesser)

Status: notes (for later implementation work)  
Last updated: 2025-12-22  
Scope: current cost-tracking implementation review (no changes proposed here)

## Executive Summary

Lesser currently has multiple cost-tracking layers:

1. **Request-scoped counters** (in-memory) that can estimate “per request” cost signals (Dynamo read/write units, Lambda duration/memory, S3 ops, bytes out).
2. **DynamORM operation tracking wrappers** that can increment those counters and capture richer operation metadata (filters, projections, consistent reads, etc.).
3. **Durable cost records + analytics** built around `models.DynamoDBCostRecord` and `models.DynamoDBCostAggregation`, with a **stream-driven aggregator** (`cmd/cost-aggregator`) computing rollups.
4. A separate **CostHistory table** persistence path (`pkg/cost/storage.go` + `pkg/cost/middleware.go`) that appears more “legacy/side-path” than the primary system.

For the upcoming pilot, the durable record/aggregation system is the best foundation for “flesh this out”; the main gaps are around **API Gateway (REST/WebSocket/streaming), observability, security, and egress/NAT** costs and consistent unit/currency handling.

## 1) Core Building Blocks

### 1.1 Request-scoped cost counters (lightweight)

- `pkg/cost/tracker.go`
  - Tracks: Dynamo reads/writes/storage, Lambda invocations/duration/memory, S3 GET/PUT/storage, bytes out.
  - Calculates an estimated total cost (`Tracker.CalculateCost()`), and supports merging/cloning.
- `pkg/cost/context.go`
  - Context attachment helpers: `cost.WithTracker(ctx, tracker)` + convenience functions like `cost.TrackDynamoReadContext(ctx, items)`.
- `pkg/cost/circuit_breaker.go`
  - Circuit breaker intended to cap spend per-hour/per-request (used by `Tracker.TrackDynamoRead/Write`).

Notes:
- These counters are useful for *relative comparisons and hotspots* during trials, but should be treated as **approximate** until unit/pricing math is validated against Cost Explorer.

### 1.2 DynamORM operation tracking

- `pkg/cost/dynamorm_tracker.go`
  - `NewTrackingDB(db, tracker, logger)` wraps `core.DB` and returns a `core.Query` wrapper that tracks read/write operations and (optionally) captures rich metadata.
  - `EnhancedOperationTracker` is currently **in-memory** and logs metadata; it does not persist it.
- `pkg/storage/dynamorm/lambda_init.go`
  - `LambdaInitWithOptions(... EnableCostTracking: true ...)` can wrap the DB with `NewTrackingDB`.

Practical takeaway:
- This layer is a “low friction” way to standardize cost counter increments (read/write) across repositories that already use DynamORM.

## 2) Durable Cost Records + Analytics (primary foundation)

### 2.1 Durable record model

- `pkg/storage/models/cost_tracking.go`
  - `models.DynamoDBCostRecord` (detailed cost tracking record)
  - `models.DynamoDBCostAggregation` (rollup by period/operation/table)

The record model already supports:
- Operation type, table, timestamp/period.
- RCUs/WCUs + microcents cost fields.
- Function/service identity (`ServiceName`, `FunctionName`, `RequestID`, version).
- `Tags` and `Properties` maps for app-specific attribution (user/tenant/job IDs, streaming route, federation target domain, etc.).

### 2.2 Repository and analytics API

- `pkg/storage/repositories/cost_tracking_repository.go`
  - Persistence + higher-level analytics: list by table, trends, anomaly detection/forecast scaffolding, etc.

### 2.3 Aggregation pipeline

- `cmd/cost-aggregator/main.go`
  - Processes DynamoDB stream events and writes:
    - Raw `models.DynamoDBCostRecord` (per operation)
    - Aggregated `models.DynamoDBCostAggregation` (period rollups)

### 2.4 Where records are emitted today (examples)

Some jobs emit `models.DynamoDBCostRecord` directly:
- `cmd/export-generator/main.go` (export job costs)
- `cmd/import-processor/main.go` (import job costs)
- `cmd/notification-processor/main.go` (notification delivery costs)
- `pkg/dlq/processor.go` (DLQ reprocessing costs)
- `pkg/storage/cost/dynamorm_storage.go` (bridges `cost.OperationCost` → `models.DynamoDBCostRecord`)

This record-centric approach is the best place to add pilot-driven attribution fields (user, tenant, “stream name”, federation remote domain, etc.).

## 3) CostHistory Table Path (separate / likely legacy)

There is a parallel persistence path:

- `pkg/cost/middleware.go` initializes a global `pkg/cost/storage.go` and attempts to persist a per-request `cost.OperationCost` into a DynamoDB table named by `COST_HISTORY_TABLE_NAME`.
- `pkg/cost/storage.go` uses the native DynamoDB SDK client (not DynamORM) to write cost events and aggregates keyed like `COST#YYYY-MM-DD`, `COST_DAILY#...`, `COST_MONTHLY#...`.

Notes:
- This path is APIGatewayV2-shaped (`events.APIGatewayV2HTTPRequest`) and is not obviously wired into the primary Lift-based API path.
- If the policy remains “DynamORM only”, this path likely needs either migration to DynamORM or clear “intentional exception” documentation.

## 4) Unit / Pricing Consistency (important for trials)

There are multiple “cost math” implementations:

- `pkg/cost/tracker.go` (microcents calculation for request counters)
- `pkg/cost/tracking_types.go` / `pkg/cost/tracking_service.go` (CloudWatch metric-oriented tracker)
- `pkg/storage/models/cost_tracking.go` includes its own `CalculateCost(...)` helper
- `pkg/storage/cost/dynamorm_storage.go` maps `cost.OperationCost` to `models.DynamoDBCostRecord`

Before you rely on totals for budgeting, it’s worth a focused “units + price constants” calibration pass so that:
- “microcents” means the same thing everywhere,
- totals roughly match Cost Explorer for a controlled load test,
- CloudWatch/EMF metrics are explicitly “signals”, while durable records are “accounting-grade”.

## 5) Pilot Gaps vs. What You’ll Be Watching

Your pilot cost hotspots were: enhanced security, observability, and the serverless tax around usage (egress, long-lived streaming sessions).

The durable record system is Dynamo/Lambda-centric today; it does not yet have first-class tracking for:

- **API Gateway costs**
  - REST (request count, payload sizes) is partially inferable, but streaming/SSE costs need explicit instrumentation.
  - WebSocket pricing drivers (connection-minutes, messages) need first-class capture.
- **CloudWatch logs**
  - Ingestion volume and retention/exports are likely to dominate unless controlled.
- **Tracing (X-Ray)**
  - Trace volume and sampling rate should be tracked as a cost driver.
- **WAF/Shield**
  - Usually billed per request/rule/capacity; needs explicit attribution.
- **NAT + egress**
  - For ActivityPub, outbound delivery/retries can dominate; bytes + request counts need tracking by remote domain / queue / job type.

## 6) Conceptual Approach (recommended framing)

For trials, treat cost tracking as three distinct products:

1. **Signals (fast, cheap, low-cardinality)**
   - EMF/CloudWatch metrics for dashboards and alerts (invocations, latency, bytes out, websocket messages, streaming connection-minutes).
2. **Attribution (durable, queryable, higher-cardinality)**
   - `models.DynamoDBCostRecord` for “who/what caused spend”, with tags/properties to support per-user/per-tenant/per-feature analysis.
3. **Accounting / calibration**
   - Periodic reconciliation against Cost Explorer/cur reports to adjust estimators and allocate shared overhead (WAF, logs, NAT) to workloads.

## 7) Suggested Next Steps (when you circle back)

When you’re ready to expand cost tracking during trials:

- Decide which path is canonical:
  - Prefer `models.DynamoDBCostRecord` + `models.DynamoDBCostAggregation` as the single durable system.
  - Either deprecate or migrate `pkg/cost/storage.go`/`pkg/cost/middleware.go`.
- Add explicit instrumentation for:
  - WebSocket connection-minutes + message counts + bytes
  - SSE connection duration + bytes
  - ActivityPub delivery: outbound requests/bytes by remote domain + retry counts
  - CloudWatch log volume and sampling levels for tracing
- Add a calibration workflow:
  - Run a controlled load for a fixed period, compare estimates to Cost Explorer, and tune constants/allocators.

