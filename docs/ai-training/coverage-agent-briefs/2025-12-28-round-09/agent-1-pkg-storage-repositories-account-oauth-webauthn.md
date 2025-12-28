# Agent 1 Brief — `pkg/storage/repositories` Account OAuth/WebAuthn (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/scheduled_status_repository.go` (baseline **0.0%**, 0/117 statements)
2) Small file: `pkg/storage/repositories/websocket_subscription_manager_repository.go` (baseline **0.0%**, 0/94 statements)
3) Small file: `pkg/storage/repositories/import_export_simple_helpers.go` (baseline **79.6%**, 74/93 statements)
4) Small file: `pkg/storage/repositories/relationship_helpers.go` (baseline **5.1%**, 4/78 statements)
5) Large file: `pkg/storage/repositories/oauth_helpers.go` (baseline **73.4%**, 160/218 statements)
6) Large file: `pkg/storage/repositories/account_repository_webauthn.go` (baseline **0.0%**, 0/204 statements)
7) Large file: `pkg/storage/repositories/account_repository_oauth.go` (baseline **16.1%**, 28/174 statements)
8) Large file: `pkg/storage/repositories/timeline_repository.go` (baseline **79.3%**, 119/150 statements)

Reference: `docs/ai-training/REPOSITORY_TESTING_GUIDE.md`.

## Status

Planned (round 09): not started.

## Constraints (must follow)

- Scope: only add/modify tests in `pkg/storage/repositories/*_test.go` (do not edit non-test code without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Prefer unit tests with DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.
- Tests must be deterministic (no sleeps; avoid asserting exact timestamps/IDs; no map-order assumptions; avoid `t.Parallel()` if you touch globals).
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

1) Warm-ups: cover every exported function/method in each small file + key error branches.
2) For OAuth + WebAuthn helpers: start with a permissive “coverage sweep” to hit every exported method once, then add targeted tests for:
   - not-found vs other errors
   - cursor/pagination edge cases
   - validation failures and conditional failures (where used)
3) Re-run the scoreboard after each iteration; do not guess.

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'scheduled_status_repository|websocket_subscription_manager_repository|import_export_simple_helpers|relationship_helpers|oauth_helpers|account_repository_webauthn|account_repository_oauth|timeline_repository'
```

