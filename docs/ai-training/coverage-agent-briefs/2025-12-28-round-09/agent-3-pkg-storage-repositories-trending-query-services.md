# Agent 3 Brief — `pkg/storage/repositories` Trending/Query/Services (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/quote_repository.go` (baseline **0.0%**, 0/128 statements)
2) Small file: `pkg/storage/repositories/like_repository.go` (baseline **0.0%**, 0/122 statements)
3) Small file: `pkg/storage/repositories/article_repository.go` (baseline **0.0%**, 0/120 statements)
4) Small file: `pkg/storage/repositories/community_note_repository.go` (baseline **0.0%**, 0/115 statements)
5) Large file: `pkg/storage/repositories/hashtag_trending_engine.go` (baseline **14.4%**, 46/320 statements)
6) Large file: `pkg/storage/repositories/query_utils.go` (baseline **37.6%**, 96/255 statements)
7) Large file: `pkg/storage/repositories/enhanced_base_repository.go` (baseline **63.0%**, 92/146 statements)
8) Large file: `pkg/storage/repositories/services.go` (baseline **38.9%**, 56/144 statements)

Reference: `docs/ai-training/REPOSITORY_TESTING_GUIDE.md`.

## Status

Planned (round 09): not started.

## Constraints (must follow)

- Scope: only add/modify tests in `pkg/storage/repositories/*_test.go` (do not edit non-test code without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Prefer unit tests with DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.
- Tests must be deterministic (avoid sleeps; prefer passing `time.Time` as inputs; no map-order assumptions; avoid `t.Parallel()` if you touch globals).
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

1) Warm-ups: cover every exported method + key error branches in the four small files.
2) For `hashtag_trending_engine.go`: prioritize pure scoring/math + caching behavior; avoid flaky time-based assertions (use fixed times).
3) For `query_utils.go` and other large files: start with a permissive “coverage sweep”, then add targeted tests for pagination, cursor encoding/decoding, and error mapping.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'quote_repository|like_repository|article_repository|community_note_repository|hashtag_trending_engine|query_utils|enhanced_base_repository|services'
```

