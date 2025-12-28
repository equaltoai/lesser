# Agent 4 Brief — `pkg/storage/repositories` Activity/Announcement/Recovery (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/wallet_repository.go` (baseline **0.0%**, 0/136 statements)
2) Small file: `pkg/storage/repositories/moderation_ml_repository.go` (baseline **4.0%**, 5/124 statements)
3) Small file: `pkg/storage/repositories/threat_intel_repository.go` (baseline **0.0%**, 0/113 statements)
4) Small file: `pkg/storage/repositories/search_integration_example.go` (baseline **0.0%**, 0/88 statements)
5) Large file: `pkg/storage/repositories/activity_repository.go` (baseline **5.6%**, 12/214 statements)
6) Large file: `pkg/storage/repositories/announcement_repository.go` (baseline **0.0%**, 0/212 statements)
7) Large file: `pkg/storage/repositories/recovery_repository.go` (baseline **0.0%**, 0/154 statements)
8) Large file: `pkg/storage/repositories/severance_repository.go` (baseline **0.0%**, 0/154 statements)

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

## Approach (recommended)

1) Warm-ups: cover every exported function/method + key error branches in the four small files.
2) For each large file: start with a permissive “coverage sweep” hitting each exported method once, then add targeted tests for:
   - not-found vs other errors
   - time-window filtering + sorting/limit logic
   - conditional write/update/delete branches

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'wallet_repository|moderation_ml_repository|threat_intel_repository|search_integration_example|activity_repository|announcement_repository|recovery_repository|severance_repository'
```

