# Agent 1 Brief — `pkg/storage/repositories` Auth + Account (4 small → 4 large)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Small file: `pkg/storage/repositories/oauth_session_repository.go` (baseline **0.0%**, 0/88 statements)
2) Small file: `pkg/storage/repositories/auth_refresh_token_repository.go` (baseline **0.0%**, 0/126 statements)
3) Small file: `pkg/storage/repositories/oauth_repository.go` (baseline **0.0%**, 0/86 statements)
4) Small file: `pkg/storage/repositories/account_repository_refresh_tokens.go` (baseline **0.0%**, 0/148 statements)
5) Large file: `pkg/storage/repositories/account_repository_auth.go` (baseline **0.7%**, 3/417 statements)
6) Large file: `pkg/storage/repositories/auth_repository.go` (baseline **0.0%**, 0/306 statements)
7) Large file: `pkg/storage/repositories/account_repository_social.go` (baseline **2.5%**, 7/279 statements)
8) Large file: `pkg/storage/repositories/rate_limit_repository.go` (baseline **14.8%**, 38/256 statements)

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

1) Warm-ups: cover every exported function/method in each small file + key error branches.
2) For `account_repository_auth.go`, focus on branchy/auth logic:
   - bcrypt success/failure paths (use real `bcrypt.GenerateFromPassword`)
   - suspended account handling
   - token/session ID shape assertions (prefix/length), not exact values
   - not-found vs other errors (via DynamORM mock errors)
3) For `auth_repository.go`, start with a permissive “coverage sweep” (MockDB/MockQuery) to hit every exported method, then add targeted tests for:
   - pagination cursor loops (NextCursor handling)
   - scan/query error mapping
   - create/update/delete error mapping
4) For `account_repository_social.go` + `rate_limit_repository.go`, prioritize:
   - limit clamping branches
   - not-found vs other errors
   - conditional failures (where used)

## Validation

```bash
./lesser test coverage --scope pkg
./lesser coverage scoreboard --profile coverage_pkg.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 2000 | rg 'oauth_session_repository|auth_refresh_token_repository|oauth_repository|account_repository_refresh_tokens|account_repository_auth|auth_repository|account_repository_social|rate_limit_repository'
```
