# Agent 2 Brief — `pkg/storage/repositories` SearchRepository (warm-up → large file)

## Goal

Apply the repository coverage approach (two warm-ups → one large file), in this order:

1) Small file: `pkg/storage/repositories/search_cost_tracking_wrapper.go` (baseline **0.0%**, 0/193 statements)
2) Small file: `pkg/storage/repositories/emoji_repository.go` (baseline **0.0%**, 0/193 statements)
3) Large file: `pkg/storage/repositories/search_repository.go` (baseline **10.9%**, 85/782 statements)

Target: **≥ 90% statement coverage per file** (not package-wide).

Reference: `docs/ai-training/REPOSITORY_TESTING_GUIDE.md`.

## Status

Planned (round 06): not started.

## Constraints (must follow)

- Repository scope only (`pkg/storage/repositories/*`).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Prefer unit tests with DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.
- Tests must be deterministic (no sleeps; no map-order assumptions; avoid `t.Parallel()` if you touch globals).
- Use `./lesser` for validation (`./lesser test coverage --scope pkg` + scoreboard).

## Approach (recommended)

1) Warm-ups: write focused CRUD/lookup tests with mocks; cover not-found, conditional, and query error translation.
2) For `search_repository.go`, start with a “coverage sweep” test to execute most methods once (permissive mock chain + reflection populator), then add targeted tests for:
   - pagination/cursor behavior (empty cursor vs non-empty)
   - branchy filtering and conversion logic
   - error branches from `All/Scan/First/Count`
3) Keep mocks readable: prefer `mock.AnythingOfType("*[]models.X")` for error injections to avoid breaking other calls.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg '(search_repository|emoji_repository|search_cost_tracking_wrapper)'
```
