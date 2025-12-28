# Agent 1 Brief — `pkg/storage/repositories` Notifications + Conversations (2 small → 2 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/notification_helpers.go` (baseline **0.0%**, 0/67 statements)
2) Small file: `pkg/storage/repositories/push_subscription_repository.go` (baseline **0.0%**, 0/129 statements)
3) Large file: `pkg/storage/repositories/notification_repository.go` (baseline **0.0%**, 0/430 statements)
4) Large file: `pkg/storage/repositories/conversation_repository.go` (baseline **0.0%**, 0/397 statements)

Reference: `docs/ai-training/REPOSITORY_TESTING_GUIDE.md`.

## Status

Planned (round 07): not started.

## Constraints (must follow)

- Scope: only add/modify tests in `pkg/storage/repositories/*_test.go` (and do not edit non-test code without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Prefer unit tests with DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.
- Tests must be deterministic (no sleeps; no map-order assumptions; avoid `t.Parallel()` if you touch globals).
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

1) Warm-ups: for each small file, cover every exported function/method + key error/empty branches.
2) For each large file, start with a permissive “coverage sweep” test (MockDB/MockQuery) to hit all exported methods, then add targeted tests for:
   - not-found vs other error paths
   - conditional write/update paths
   - pagination/cursor branches (if present)
3) Re-run the scoreboard after each iteration; do not guess.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'notification_(helpers|repository)|push_subscription_repository|conversation_repository'
```

