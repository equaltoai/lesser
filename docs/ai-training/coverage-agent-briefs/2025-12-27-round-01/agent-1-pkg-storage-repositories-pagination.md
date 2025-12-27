# Agent 1 Brief — `pkg/storage/repositories` pagination helpers

## Goal

Increase `pkg/` unit test coverage by adding deterministic tests for the search/repository pagination helpers. Target a clean, low-risk win by covering pure functions and boundary behavior (no AWS, no network).

Primary target: `pkg/storage/repositories/pagination.go`

## Constraints (must follow)

- Run tests via the CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- Unit tests must be AWS/network independent.
- Do not use `httptest.NewServer` (the environment cannot bind/listen to ports).
- Prefer table-driven tests + `stretchr/testify`.
- Don’t refactor production code unless a testability issue blocks you.

## What to cover (must)

### 1) Option validation

File: `pkg/storage/repositories/pagination.go`

Test `(*PaginationOptions).Validate()`:

- `Limit <= 0` defaults to `20`
- `Limit > 50` clamps to `50`
- Empty/invalid `SortOrder` defaults to `relevance`
- Invalid sort order returns an error wrapping `ErrPaginationParametersInvalid`

### 2) Cursor encode/decode

Test:

- `EncodeCursor(nil) == ""`
- `EncodeCursor(&CursorData{}) == ""` (empty cursor data is intentionally not encoded)
- Round-trip: `DecodeCursor(EncodeCursor(data))` preserves key fields
- `DecodeCursor("")` returns `&CursorData{}` with `nil` error
- Invalid cursor strings return errors wrapped by:
  - `ErrPaginationCursorInvalid` (fails repository cursor validation)
  - or `ErrPaginationCursorFormat` / `ErrPaginationCursorData` (bad base64 / bad JSON)

Tip: you can create a clearly-invalid cursor by passing non-base64 like `"%%%"`.

### 3) Page-state helpers

Test:

- `CreateNextCursor(...)` returns empty when the data is “empty” (e.g. all zero values), and non-empty when at least one of the cursor fields is set.
- `CreatePaginationResult(...)` returns the provided fields.
- `ShouldContinuePagination(...)` truth table:
  - `resultCount > requestedLimit` → `true`
  - `totalProcessed < maxScan && resultCount == requestedLimit` → `true`
  - otherwise → `false`
- `ApplyPaginationLimits`:
  - `len(results) <= limit` returns original slice, `hasMore=false`
  - `len(results) > limit` returns truncated slice, `hasMore=true`

### 4) Sorting

Cover `SortResults` and the tie-breaking behavior:

- `SearchSortRelevance`: score desc; ties broken by timestamp desc
- `SearchSortTimeAsc`: timestamp asc; ties broken by ID asc (stable)
- `SearchSortTimeDesc`: timestamp desc; ties broken by ID asc (stable)

Tip: use a tiny struct like:

```go
type item struct {
  score float64
  ts    time.Time
  id    string
}
```

Provide `getScore/getTimestamp/getID` closures.

## Stretch (optional, only if time remains)

Also cover: `pkg/storage/repositories/shared_helpers_simple.go`

- `NormalizePaginationLimit(limit)` for invalid values returning default `20`, and valid values returning `limit`.
- `AuditLogQueryHelper(...)` using `github.com/pay-theory/dynamorm/pkg/mocks` to assert the expected query chain is invoked (Index/Where/Limit/All). Keep it minimal; do not require real DynamoDB.

## Deliverables

- New test file(s) in `pkg/storage/repositories/`:
  - Suggested: `pagination_test.go` (and `shared_helpers_simple_test.go` if you do the stretch)
- All tests pass:
  - `./lesser test unit`
  - `./lesser lint`
- Coverage improved (not required to hit a number, but should move):
  - `./lesser test coverage --scope pkg`
  - `./lesser coverage scoreboard --profile coverage_pkg.out --top 25`

