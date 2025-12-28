# Agent 2 Brief — `pkg/storage/repositories` Trust/Health/CloudWatch (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/poll_repository.go` (baseline **10.2%**, 14/137 statements)
2) Small file: `pkg/storage/repositories/audit_repository.go` (baseline **0.0%**, 0/131 statements)
3) Small file: `pkg/storage/repositories/block_repository.go` (baseline **65.1%**, 54/83 statements)
4) Small file: `pkg/storage/repositories/circuit_breaker_repository.go` (baseline **0.0%**, 0/73 statements)
5) Large file: `pkg/storage/repositories/trust_repository.go` (baseline **0.8%**, 2/248 statements)
6) Large file: `pkg/storage/repositories/instance_health_repository.go` (baseline **0.0%**, 0/223 statements)
7) Large file: `pkg/storage/repositories/cloudwatch_metrics_repository.go` (baseline **0.0%**, 0/198 statements)
8) Large file: `pkg/storage/repositories/filter_repository.go` (baseline **0.0%**, 0/195 statements)

Reference: `docs/ai-training/REPOSITORY_TESTING_GUIDE.md`.

## Status

Planned (round 09): not started.

## Constraints (must follow)

- Scope: only add/modify tests in `pkg/storage/repositories/*_test.go` (do not edit non-test code without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Prefer unit tests with DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.
- Tests must be deterministic (no sleeps; avoid asserting exact timestamps/IDs; no map-order assumptions; avoid `t.Parallel()` if you touch globals).
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## CloudWatch-specific guidance (no network)

- Do not call real AWS endpoints.
- Prefer testing pure logic first (cost math, breakdown calculations).
- For code paths that require `r.client.GetMetricStatistics`, build a CloudWatch client with a stubbed `http.Client` transport that returns canned responses (no outbound network).
- If you hit a runtime/panic that requires non-test code changes, pause and report for coordination.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'poll_repository|audit_repository|block_repository|circuit_breaker_repository|trust_repository|instance_health_repository|cloudwatch_metrics_repository|filter_repository'
```

