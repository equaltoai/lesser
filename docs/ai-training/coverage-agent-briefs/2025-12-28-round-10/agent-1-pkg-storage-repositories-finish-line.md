# Agent 1 Brief — `pkg/storage/repositories` Finish Line (all remaining < 90%)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide) for **every remaining file below 90%**:

1) `pkg/storage/repositories/draft_repository.go` (baseline **0.0%**, 0/54 statements)
2) `pkg/storage/repositories/publication_member_repository.go` (baseline **0.0%**, 0/48 statements)
3) `pkg/storage/repositories/utils.go` (baseline **37.5%**, 18/48 statements)
4) `pkg/storage/repositories/category_repository.go` (baseline **0.0%**, 0/41 statements)
5) `pkg/storage/repositories/revision_repository.go` (baseline **0.0%**, 0/40 statements)
6) `pkg/storage/repositories/dns_cache_repository.go` (baseline **0.0%**, 0/39 statements)
7) `pkg/storage/repositories/series_repository.go` (baseline **0.0%**, 0/39 statements)
8) `pkg/storage/repositories/marker_repository.go` (baseline **0.0%**, 0/34 statements)
9) `pkg/storage/repositories/account_repository_example_refactor.go` (baseline **0.0%**, 0/31 statements)
10) `pkg/storage/repositories/hashtag_trending_calculator.go` (baseline **0.0%**, 0/28 statements)
11) `pkg/storage/repositories/streaming_cloudwatch_repository.go` (baseline **0.0%**, 0/22 statements)
12) `pkg/storage/repositories/publication_repository.go` (baseline **0.0%**, 0/14 statements)
13) `pkg/storage/repositories/hashtag_follow_helpers.go` (baseline **70.8%**, 17/24 statements)

Reference: `docs/ai-training/REPOSITORY_TESTING_GUIDE.md`.

## Status

Planned (round 10): not started.

## Constraints (must follow)

- Scope: only add/modify tests in `pkg/storage/repositories/*_test.go` (do not edit non-test code without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Prefer unit tests with DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.
- Tests must be deterministic (avoid sleeps; prefer passing `time.Time` as inputs; no map-order assumptions; avoid `t.Parallel()` if you touch globals).
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

1) Start with the 11 files at 0.0% (they are all small), aiming for “cover every exported function + core error branches”.
2) For `utils.go` / `hashtag_follow_helpers.go`: cover every branch + edge-case validation paths.
3) If any file is “thin wrapper” code, cover behavior via table-driven tests and stubbed DB/mocks (no real DynamoDB).

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'draft_repository|publication_member_repository|utils\\.go|category_repository|revision_repository|dns_cache_repository|series_repository|marker_repository|account_repository_example_refactor|hashtag_trending_calculator|streaming_cloudwatch_repository|publication_repository|hashtag_follow_helpers'
```

