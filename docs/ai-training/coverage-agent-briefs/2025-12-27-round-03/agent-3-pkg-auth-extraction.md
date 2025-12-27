# Agent 3 Brief — `pkg/auth` extraction + middleware helpers

## Goal

Bring `pkg/auth/extraction.go` off 0% by adding focused unit tests for the consolidated auth-extraction helpers and middleware behaviors:

- `pkg/auth/extraction.go`

These functions are intended to replace ad-hoc handler auth logic; the tests should lock in behavior and error codes.

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Prefer table-driven tests + `stretchr/testify`.
- Use `pkg/testing/lift` helpers (`MockLiftContext`, `WithHeaders`) for Lift context testing.

## What to cover

### 1) Header + bearer token extraction via `GetAccountFromContext`

Cover:

- missing `Authorization` header → `errors.CodeUnauthorized`
- invalid header format (not `Bearer …`) → `errors.CodeUnauthorized`
- `oauthService.ValidateAccessToken` returns error → `errors.CodeUnauthorized`
- valid token → returns `AuthenticatedAccount` with:
  - `Username == claims.GetUsername()`
  - `Claims == claims`

Implementation tip:

- Create a stub `OAuthServiceInterface` that returns claims for known tokens and errors otherwise.

### 2) Scope enforcement helpers

Cover:

- `RequireAuthWithScope`
  - missing scope → `errors.CodeForbidden`
  - has scope → success
- `RequireAuthWithMultipleScopes`
  - any-of scopes succeeds
  - none-of scopes → `errors.CodeForbidden`
- convenience wrappers:
  - `RequireReadScope`, `RequireWriteScope`, `RequireAdminScope`, `RequireReadOrWriteScope`

### 3) Optional auth

Cover `ExtractOptionalAuth`:

- no auth header → `(nil, nil)`
- invalid bearer format → `(nil, nil)`
- invalid token → `(nil, nil)`
- valid token → returns account

### 4) Ownership guards

Cover:

- `ValidateAccountOwnership`
  - equal usernames → nil
  - mismatch → `errors.CodeForbidden`
- `ValidateAccountOwnershipOrAdmin`
  - equal usernames → nil
  - admin scope → nil
  - mismatch without admin → `errors.CodeForbidden`

### 5) Lift middleware wrappers

Cover:

- `NewAuthenticationMiddleware`
- `RequireAuthMiddleware`
  - on missing auth → writes 401 response via `common.RespondUnauthorized`
- `RequireScopeMiddleware`
  - auth failure → 401 response
  - scope failure → 403 response
- `OptionalAuthMiddleware` + `GetAuthenticatedAccountFromContext`
  - valid token sets `"authenticated_account"` and retrieval returns `(account, true)`
  - wrong type stored returns `(nil, false)`

Tip: Prefer asserting `ctx.Response.StatusCode` rather than relying on returned error value from Lift response writers.

### 6) Standard `context.Context` helpers

Cover:

- `SetAccountInStandardContext`
- `GetAccountFromStandardContext` (success + unauthorized when missing/wrong type)
- `GetUsernameFromStandardContext`
- `RequireAuthFromStandardContext`

## Deliverables

- New tests in `pkg/auth/`, suggested filename:
  - `extraction_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

