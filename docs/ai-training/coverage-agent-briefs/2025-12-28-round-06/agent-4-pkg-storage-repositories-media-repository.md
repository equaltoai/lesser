# Agent 4 Brief — `pkg/storage/repositories` MediaRepository (warm-up → large file)

## Goal

Apply the repository coverage approach (two warm-ups → one large file), in this order:

1) Small file: `pkg/storage/repositories/media_session_repository.go` (baseline **0.0%**, 0/183 statements)
2) Small file: `pkg/storage/repositories/alert_repository.go` (baseline **0.0%**, 0/182 statements)
3) Large file: `pkg/storage/repositories/media_repository.go` (baseline **0.0%**, 0/505 statements)

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

1) Warm-ups: implement CRUD/lookup tests first; cover both not-found and non-notfound errors.
2) For `media_repository.go`, use a sweep test to execute all exported methods once, then add small “error branch” tests to cover:
   - conditional failures (already exists)
   - not-found behavior (returns nil vs returns error, as intended)
   - pagination cursor logic
3) Keep mocks readable and type-safe (`mock.AnythingOfType("*[]models.X")` for `All/Scan` error injection).

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg '(media_repository|media_session_repository|alert_repository)'
```
