# Agent 2 Brief — `pkg/storage/repositories` Cost + Metrics (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/routing_metrics_repository.go` (baseline **0.0%**, 0/71 statements)
2) Small file: `pkg/storage/repositories/federation_cost_repository.go` (baseline **0.0%**, 0/199 statements)
3) Small file: `pkg/storage/repositories/import.go` (baseline **0.0%**, 0/195 statements)
4) Small file: `pkg/storage/repositories/export.go` (baseline **0.0%**, 0/118 statements)
5) Large file: `pkg/storage/repositories/websocket_cost_repository.go` (baseline **40.5%**, 177/437 statements)
6) Large file: `pkg/storage/repositories/scheduled_job_cost_repository.go` (baseline **0.0%**, 0/413 statements)
7) Large file: `pkg/storage/repositories/notification_cost_repository.go` (baseline **0.0%**, 0/338 statements)
8) Large file: `pkg/storage/repositories/metrics_repository.go` (baseline **0.0%**, 0/310 statements)

Reference: `docs/ai-training/REPOSITORY_TESTING_GUIDE.md`.

## Status

Planned (round 08): not started.

## Constraints (must follow)

- Scope: only add/modify tests in `pkg/storage/repositories/*_test.go` (do not edit non-test code without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Prefer unit tests with DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.
- Tests must be deterministic (no sleeps; avoid asserting exact timestamps/IDs; no map-order assumptions; avoid `t.Parallel()` if you touch globals).
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

1) Warm-ups:
   - `routing_metrics_repository.go`: cover `Store*`, `Get*`, and error paths (query/create failures).
   - `federation_cost_repository.go`: cover query builders + aggregation math (cost summaries), plus budget not-found path.
2) For `import.go` + `export.go`: cover status/metadata updates, not-found vs other errors, and cost-summary aggregation math.
3) For `websocket_cost_repository.go`, `scheduled_job_cost_repository.go`, `notification_cost_repository.go`, and `metrics_repository.go`:
   - start with a permissive “coverage sweep” test to hit every exported method once
   - add targeted tests for time-range filtering, sorting/limit logic, and error mapping branches
   - if you hit a runtime bug that blocks testing *inside these files* (e.g. recursion/stack overflow), stop and report it for coordination before changing non-test code

## Known gotcha (watch for this)

- `pkg/storage/repositories/websocket_cost_repository.go` has a `BatchCreate` method that appears to call itself (`return r.BatchCreate(ctx, records)`). If you need to cover the non-empty path, pause and report; coordinator will fix production code before you proceed.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'routing_metrics_repository|federation_cost_repository|import\\.go|export\\.go|websocket_cost_repository|scheduled_job_cost_repository|notification_cost_repository|metrics_repository'
```
