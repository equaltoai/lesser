# Agent 3 Brief — `pkg/storage/models/federation_route_metrics.go`

## Goal

Increase `pkg/storage/models` coverage by adding deterministic tests for federation route metrics keying and derived metric calculations.

Primary target:

- `pkg/storage/models/federation_route_metrics.go`

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Prefer table-driven tests + `stretchr/testify`.
- Avoid `time.Sleep`; use `assert.WithinDuration` for timestamp checks.

## What to cover

### 1) `determinePerformanceTier`

Table-driven test for latency thresholds:

- `<= 100` → `excellent`
- `<= 300` → `good`
- `<= 1000` → `fair`
- `> 1000` → `poor`

### 2) `UpdateKeys`

Test that `UpdateKeys()` sets:

- `PK` includes route id + compact date
- `SK` uses `METRICS#<periodType>`
- `GSI1PK`/`GSI1SK` are date/time-based and include route id
- `GSI2PK` uses destination domain
- `GSI3PK` uses tier from `AvgLatencyMs`
- `GSI3SK` is `LATENCY#%06d#<route_id>` formatting

### 3) `BeforeCreate` defaults + derived fields

Test `BeforeCreate()`:

- Initializes `CreatedAt`, `UpdatedAt`, `FirstUsed` (when zero)
- Initializes `ErrorBreakdown` map and `HealthHistory` slice when nil
- Sets `TTL` to ~90 days in the future
- Calls `calculateDerivedMetrics()`:
  - success rate, timeout rate, avg cost per delivery, etc.
  - spot-check 2–3 derived fields with known inputs

### 4) `BeforeUpdate`

Test `BeforeUpdate()` updates `UpdatedAt`, recomputes derived metrics, and updates keys.

## Deliverables

- New test file: `pkg/storage/models/federation_route_metrics_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

