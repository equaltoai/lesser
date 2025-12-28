# Agent 1 Brief — `pkg/storage/repositories` AccountRepository (warm-up → large file)

## Goal

Apply the repository coverage approach (two warm-ups → one large file), in this order:

1) Small file: `pkg/storage/repositories/account_repository_search.go` (baseline **0.0%**, 0/177 statements)
2) Small file: `pkg/storage/repositories/account_repository_timeline.go` (baseline **0.0%**, 0/186 statements)
3) Large file: `pkg/storage/repositories/account_repository.go` (baseline **19.6%**, 178/909 statements)

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

1) Start with the warm-ups: cover every exported method + 1–2 key error branches (not found, conditional failure, query error).
2) For `account_repository.go`, begin with a permissive “coverage sweep” test (MockDB/MockQuery + reflection populators), then add targeted branch tests for:
   - pagination cursor logic
   - not-found vs other errors
   - conditional write paths
   - internal helpers with heavy branching
3) Re-run `./lesser coverage scoreboard --mode file` after each iteration; do not guess.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'account_repository'
```
