# Agent 3 Brief — `pkg/storage/repositories` Actors + Relationships (2 small → 2 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/relationship_base.go` (baseline **0.0%**, 0/117 statements)
2) Small file: `pkg/storage/repositories/relationship_pagination_helpers.go` (baseline **56.5%**, 39/69 statements)
3) Large file: `pkg/storage/repositories/relationship_repository.go` (baseline **4.7%**, 21/443 statements)
4) Large file: `pkg/storage/repositories/actor_repository.go` (baseline **17.3%**, 88/510 statements)

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

1) Warm-ups: cover every exported helper/type behavior in `relationship_base.go`, then fill gaps in `relationship_pagination_helpers.go` (edge cases + invalid inputs).
2) For each large file:
   - begin with a “coverage sweep” test using MockDB/MockQuery to hit all exported methods
   - add targeted tests for not-found vs other errors, conditional failures, and cursor/pagination behavior
3) Re-run the scoreboard after each iteration; do not guess.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'relationship_(base|pagination_helpers|repository)\\.go|actor_repository'
```

