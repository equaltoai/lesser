# Agent 4 Brief — `pkg/storage/repositories` Federation + Streaming (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/relay_repository.go` (baseline **0.0%**, 0/139 statements)
2) Small file: `pkg/storage/repositories/streaming_repository.go` (baseline **0.0%**, 0/146 statements)
3) Small file: `pkg/storage/repositories/route_optimizer_repository.go` (baseline **0.0%**, 0/140 statements)
4) Small file: `pkg/storage/repositories/query_cache_repository.go` (baseline **0.0%**, 0/125 statements)
5) Large file: `pkg/storage/repositories/federation_instance_repository.go` (baseline **0.0%**, 0/367 statements)
6) Large file: `pkg/storage/repositories/streaming_connection_repository.go` (baseline **0.0%**, 0/339 statements)
7) Large file: `pkg/storage/repositories/social_repository.go` (baseline **13.4%**, 57/424 statements)
8) Large file: `pkg/storage/repositories/list_repository.go` (baseline **0.0%**, 0/289 statements)

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
   - `relay_repository.go` and `streaming_repository.go`: cover every exported method + key error branches.
2) Add the two warm-ups:
   - `route_optimizer_repository.go`: cover query builders + analytics helpers + error branches.
   - `query_cache_repository.go`: cover cache hit/miss, expiry, and json (un)marshal error paths.
3) For each large file:
   - start with a permissive “coverage sweep” test using MockDB/MockQuery to hit all exported methods
   - add targeted tests for pagination cursor behavior, not-found handling, and query error mapping

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'relay_repository|streaming_repository|route_optimizer_repository|query_cache_repository|federation_instance_repository|streaming_connection_repository|social_repository|list_repository'
```
