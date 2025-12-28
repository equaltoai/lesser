# Agent 4 Brief — `pkg/storage/repositories` Base + Enhanced Pattern (2 small → 2 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/base_repository_helpers.go` (baseline **32.7%**, 16/49 statements)
2) Small file: `pkg/storage/repositories/pattern_repository.go` (baseline **0.0%**, 0/93 statements)
3) Large file: `pkg/storage/repositories/base_repository.go` (baseline **36.7%**, 230/626 statements)
4) Large file: `pkg/storage/repositories/enhanced_pattern_repository.go` (baseline **21.0%**, 92/439 statements)

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

1) Warm-ups: get `base_repository_helpers.go` to ≥90% first; use it to establish shared mock patterns you can reuse.
2) For each large file:
   - start with a permissive “coverage sweep” test using MockDB/MockQuery to hit all exported methods
   - add targeted tests for: query errors, not-found handling, conditional write paths, and any helper-heavy branching
3) Re-run the scoreboard after each iteration; do not guess.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'base_repository(_helpers)?\\.go|pattern_repository|enhanced_pattern_repository'
```

