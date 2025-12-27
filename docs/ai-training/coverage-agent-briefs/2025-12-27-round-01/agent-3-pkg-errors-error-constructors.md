# Agent 3 Brief — `pkg/errors` constructors + metadata

## Goal

Increase `pkg/` unit test coverage by validating the behavior of the error constructor helpers across error domains (auth, storage, federation, validation, lambda, services).

Primary target: `pkg/errors/` (multiple files)

## Constraints (must follow)

- Run tests via the CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- Unit tests must be AWS/network independent.
- Prefer table-driven tests + `stretchr/testify`.
- Do not refactor production code unless necessary for testability.

## Suggested approach

Most constructors are one-liners; maximize coverage with table-driven tests that:

- call each constructor
- assert non-nil
- assert `Code`, `Category`, and `Message`
- for metadata helpers, assert metadata keys exist (don’t over-assert exact formatting unless stable)
- for wrappers, assert `errors.Is` and/or `InternalMessage` behavior

Avoid huge brittle snapshots; focus on invariants.

## What to cover (must)

### 1) Domain constructors set correct category + code

Files to target:

- `pkg/errors/auth.go`
- `pkg/errors/storage.go`
- `pkg/errors/federation.go`
- `pkg/errors/lambda.go`
- `pkg/errors/validation.go`
- `pkg/errors/services.go`
- `pkg/errors/common.go` (if applicable)

Write tests that cover a representative set of constructors from each file; include at least one metadata-bearing constructor per domain, e.g.:

- `AuthFailed("reason")` has `CategoryAuth`, `CodeAuthFailed`, metadata includes `reason`
- `UserNotFound("alice")` has metadata includes `username`
- `AccessDenied("resource")` has metadata includes `resource`

### 2) Internal wrapping helpers preserve and expose underlying errors

Test a constructor that calls `WrapError` (e.g., `PasswordHashingFailed(err)`):

- `errors.Is(appErr, err)` is true
- `InternalMessage` contains underlying error text
- retryable behavior (if any) matches expectations

### 3) Category/code helpers behave consistently

If there are helper functions like `HasCode`, `HasCategory`, `GetErrorCode`, etc. not already covered sufficiently by `pkg/errors/app_error_test.go`, add a couple more cases that include non-`AppError` errors.

## Deliverables

- New/updated test file(s) in `pkg/errors/`:
  - Suggested: `auth_test.go`, `storage_test.go`, or one consolidated `constructors_test.go`
- All tests pass:
  - `./lesser test unit`
  - `./lesser lint`
- Coverage improved:
  - `./lesser test coverage --scope pkg`
  - `./lesser coverage scoreboard --profile coverage_pkg.out --top 25`

