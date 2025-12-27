# Agent 4 Brief — `pkg/storage/repositories/dlq_repository.go` (DynamORM mocks)

## Goal

Increase `pkg/storage/repositories` coverage by testing the DLQ repository query logic + DLQ-specific in-memory filtering/business rules using DynamORM mocks (no DynamoDB).

Primary target:

- `pkg/storage/repositories/dlq_repository.go`

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Prefer table-driven tests + `stretchr/testify`.
- Use DynamORM mocks: `github.com/pay-theory/dynamorm/pkg/mocks`.

## What to cover

### 1) Clamp helpers

Test:

- `clampDLQPageLimit`, `clampDLQReprocessLimit`, `clampDLQSearchLimit`
- defaulting (`<=0`) and max clamping

### 2) Query-based fetches with pagination

Using a `mocks.MockDB` + `mocks.MockQuery`, test:

- `GetDLQMessagesByErrorType`:
  - query uses `Index("gsi1")`, `Where("gsi1PK","=","DLQ_ERROR#<type>")`, `OrderBy("gsi1SK","DESC")`
  - cursor path adds `Where("gsi1SK","<",cursor)`
  - `Limit(safeLimit+1)`
  - next cursor logic + trimming

- `GetDLQMessagesByStatus`:
  - same idea but `Index("gsi2")` and `gsi2PK` key

- `SearchDLQMessages`:
  - missing `filter.Service` returns error
  - applies filters via `Filter(...)` calls (ErrorType/Status/Priority/IsPermanent/time range)
  - cursor path uses `Where("gsi3SK","<",cursor)`
  - limit+1, next cursor logic
  - text search via `filterByText` (verify it filters down when searchText is provided)

Tip: for `All(&messages)` use `Run(...)` to populate `*[]*models.DLQMessage`.

### 3) In-memory filtering

Test pure helpers without mocks:

- `filterByText`
- `messageMatchesText`

Include a case-insensitive match on `ErrorMessage` and a non-match case.

### 4) Cleanup expired messages (minimal)

Test `CleanupExpiredMessages` query shape (Filter/Limit/All) and the “empty result” early return.

You don’t need to deeply validate `DeleteDLQMessage` internals; it’s fine to keep this test scoped to:

- query is performed
- when no expired messages returned → `(0,nil)`

## Deliverables

- New test file: `pkg/storage/repositories/dlq_repository_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

