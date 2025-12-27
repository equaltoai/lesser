# Agent 4 Brief — `pkg/storage/repositories` WebSocketCostRepository aggregation helpers

## Goal

Bring `pkg/storage/repositories/websocket_cost_repository.go` off 0% by testing the **pure aggregation/math helpers** (no DB calls).

Primary target:

- `pkg/storage/repositories/websocket_cost_repository.go`

Focus on the aggregation pipeline helpers (initialize → process → finalize) and percentile functions.

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Prefer deterministic tests (no sleeps).
- Prefer table-driven tests + `stretchr/testify`.

## What to cover

### 1) Aggregation initialization and collectors

Targets:

- `initializeAggregation`
- `createMetricCollectors`

Assertions:

- all map fields are non-nil (`CostPercentiles`, `LatencyPercentiles`, `StreamPopularity`, etc.)
- collector maps initialized; slices have expected initial length/capacity semantics

### 2) Per-record processing

Targets:

- `trackUniqueEntities`
- `processOperationMetrics`
- `processConnectOperation`
- `aggregateCostComponents`
- `collectPerformanceMetrics`

Build a few `models.WebSocketCostRecord` fixtures and assert that:

- unique counts and stream popularity update correctly
- operation metrics update:
  - connect → increments `TotalConnections`, sets duration minutes, fills duration values
  - message_in/out → increments message counts + bytes
  - disconnect → increments dropped connections
  - subscribe → increments stream subscriptions
  - error → increments delivery failures
- cost totals roll up into aggregation totals

### 3) Finalization computations

Targets:

- `finalizeAggregation`
- `calculateAverages`
- `calculateMessageMetrics`
- `calculatePercentiles`

Assertions:

- average processing time and memory usage computed when `measurementCount > 0`
- message throughput and average size computed when messages exist and window duration > 0
- percentile maps are populated when values exist

### 4) Percentile helpers

Targets:

- `calculateWebSocketPercentiles`
- `getWebSocketPercentileValue`

Cover:

- empty slice returns all-zero percentiles map
- single-element slice returns that value for all percentiles
- multi-element slice returns expected p50/p90/p95/p99 (assert with exact values for a small known dataset)

## Deliverables

- New tests in `pkg/storage/repositories/`, suggested filename:
  - `websocket_cost_repository_aggregation_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

