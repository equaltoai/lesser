# Agent 2 Brief — `pkg/storage/repositories` Hashtags + Instances (2 small → 2 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/hashtag_batch_helpers.go` (baseline **0.0%**, 0/54 statements)
2) Small file: `pkg/storage/repositories/featured_tag_repository.go` (baseline **0.0%**, 0/110 statements)
3) Large file: `pkg/storage/repositories/hashtag_repository.go` (baseline **16.4%**, 81/493 statements)
4) Large file: `pkg/storage/repositories/instance_repository.go` (baseline **8.1%**, 36/445 statements)

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

1) Warm-ups: cover every exported function/method in the two small files (and empty/error branches).
2) For each large file:
   - start with a permissive “coverage sweep” (MockDB/MockQuery) to hit every exported method at least once
   - add targeted branch tests for query errors, conditional failures, and pagination branches
3) Re-run the scoreboard after each iteration; do not guess.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'hashtag_(batch_helpers|repository)\\.go|featured_tag_repository|instance_repository'
```

