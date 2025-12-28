# Agent 3 Brief — `pkg/storage/repositories` StatusRepository (warm-up → large file)

## Goal

Apply the repository coverage approach (two warm-ups → one large file), in this order:

1) Small file: `pkg/storage/repositories/thread_repository.go` (baseline **0.0%**, 0/169 statements)
2) Small file: `pkg/storage/repositories/domain_block_repository.go` (baseline **0.0%**, 0/173 statements)
3) Large file: `pkg/storage/repositories/status_repository.go` (baseline **16.8%**, 103/612 statements)

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

1) Warm-ups first (thread + domain blocks): cover exported methods + not-found/condition-failed/query-error branches with mocks.
2) For `status_repository.go`, start with a permissive “coverage sweep” test (MockDB/MockQuery + reflection populators), then iterate with targeted branch tests.
3) After each iteration, re-run the file scoreboard; do not proceed until the file is ≥90%.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg '(status_repository|thread_repository|domain_block_repository)'
```
