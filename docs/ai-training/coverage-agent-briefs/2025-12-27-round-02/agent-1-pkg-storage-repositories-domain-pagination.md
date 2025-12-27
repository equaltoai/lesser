# Agent 1 Brief — `pkg/storage/repositories` domain + keyset pagination helpers

## Goal

Boost `pkg/storage/repositories` coverage by adding unit tests for small-but-central pagination helper code that many repositories rely on.

Primary targets:

- `pkg/storage/repositories/pagination_helpers.go`
- `pkg/storage/repositories/domain_pagination_helpers.go`

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Prefer table-driven tests + `stretchr/testify`.
- Use DynamORM mocks (`github.com/pay-theory/dynamorm/pkg/mocks`) for query chaining.

## What to cover

### 1) `listByPKSKPrefixPaginated` (keyset pagination)

File: `pkg/storage/repositories/pagination_helpers.go`

Test behavior:

- Default limit: `limit <= 0` becomes `25`
- Cursor trimming: cursor is `strings.TrimSpace(cursor)` and only applied when non-empty
- Query shape:
  - `Where("PK","=",pk)`
  - `Where("SK","BEGINS_WITH",skPrefix)`
  - `OrderBy("SK","ASC")`
  - optional `Where("SK",">",cursor)`
  - `Limit(limit+1)`
  - `All(&items)`
- Next cursor logic:
  - if `len(items) <= limit` → `nextCursor == ""`
  - if `len(items) > limit` → `nextCursor == items[limit-1].GetSK()` and results are trimmed to `limit`

Tip: define a small test type implementing `GetSK()` and populate `All` via `Run(...)`.

### 2) Domain pagination (GSI1-based)

File: `pkg/storage/repositories/domain_pagination_helpers.go`

Cover these helpers:

- `clampDomainLimit` (default, clamp max)
- `buildPaginationQuery` (cursor/no cursor, `Limit(safeLimit+1)`, `Index("gsi1")`, `OrderBy("gsi1SK","DESC")`)
- `generateNextCursor` (only returns cursor when `resultsLen > requestedLimit && requestedLimit > 0`)

Cover at least one concrete paginated function end-to-end using DynamORM mocks:

- `getPaginatedEmailDomainBlocks` OR `getPaginatedDomainAllows`

Assertions:

- query chain is invoked with expected parameters (GSIPK from config, cursor comparison operator, limit+1)
- returned results are trimmed correctly
- next cursor is produced from the last retained element
- conversion to storage type preserves `ID`, `Domain`, `CreatedBy`, `CreatedAt`

### 3) Domain deletion helper

File: `pkg/storage/repositories/domain_pagination_helpers.go`

Test `genericDeleteByID`:

- When GSI query returns no matching ID → returns `storage.ErrNotFound`
- When matching ID exists → performs delete query:
  - `Where("PK","=",item.GetPK())`, `Where("SK","=",item.GetSK())`, `Delete()`
- Error paths:
  - query `All` error → wrapped via `ErrorHandler.HandleQueryError` (just assert `require.Error`)
  - delete error → wrapped via `ErrorHandler.HandleDeleteError` (just assert `require.Error`)

## Deliverables

- New tests in `pkg/storage/repositories/`:
  - Suggested: `pagination_helpers_test.go`, `domain_pagination_helpers_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

