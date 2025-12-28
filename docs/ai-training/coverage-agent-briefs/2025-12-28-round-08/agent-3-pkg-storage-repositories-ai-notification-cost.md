# Agent 3 Brief — `pkg/storage/repositories` AI + Analytics (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/ai_repository.go` (baseline **0.0%**, 0/109 statements)
2) Small file: `pkg/storage/repositories/errors.go` (baseline **88.4%**, 175/198 statements)
3) Small file: `pkg/storage/repositories/url_utils.go` (baseline **85.4%**, 158/185 statements)
4) Small file: `pkg/storage/repositories/feature_repository.go` (baseline **0.0%**, 0/72 statements)
5) Large file: `pkg/storage/repositories/ai_cost_repository.go` (baseline **0.0%**, 0/337 statements)
6) Large file: `pkg/storage/repositories/media_analytics_repository.go` (baseline **0.0%**, 0/305 statements)
7) Large file: `pkg/storage/repositories/bookmark_repository.go` (baseline **41.4%**, 128/309 statements)
8) Large file: `pkg/storage/repositories/dlq_repository.go` (baseline **27.0%**, 88/326 statements)

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
   - `ai_repository.go`: cover conversion helpers + stats calculation branches.
   - `errors.go`: close remaining gaps by directly calling uncovered error wrapper constructors.
2) For `url_utils.go`: close remaining branches (invalid formats, platform matches, shorteners, activitypub username extraction).
3) For `feature_repository.go`: cover create/get/update/list/delete + enabled checks (no real DB; use mocks).
4) For `ai_cost_repository.go`:
   - cover Create/Get and all query/filter/sort branches (time-window filtering, tier defaulting, limit behavior).
5) For `media_analytics_repository.go`, `bookmark_repository.go`, and `dlq_repository.go`:
   - start with a “coverage sweep” test hitting every exported method at least once
   - add targeted tests for not-found vs other errors, pagination/cursor branches, and aggregation/sort behavior
6) Use a “coverage sweep” test first, then add targeted error-branch tests until file coverage is ≥90%.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'ai_repository|errors\\.go|url_utils\\.go|feature_repository|ai_cost_repository|media_analytics_repository|bookmark_repository|dlq_repository'
```
