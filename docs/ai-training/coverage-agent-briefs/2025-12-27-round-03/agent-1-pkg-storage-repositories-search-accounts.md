# Agent 1 Brief — `pkg/storage/repositories` SearchRepository (account search + privacy filters)

## Goal

Raise `pkg/storage/repositories` coverage by adding unit tests for the **account search** surface area in:

- `pkg/storage/repositories/search_repository.go`

Focus on logic that is widely reused and mostly deterministic (sorting, pagination, privacy filtering, and the 3 account search strategies).

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Prefer table-driven tests + `stretchr/testify`.
- Use DynamORM mocks (`github.com/pay-theory/dynamorm/pkg/mocks`) for DB/query chaining.
- Use simple stubs for `SearchRepositoryDeps` (don’t pull in real storage adapters).

## What to cover

### 1) Query normalization + relevance sorting

File: `pkg/storage/repositories/search_repository.go`

Cover:

- `normalizeSearchQuery`
  - trims whitespace
  - lowercases
  - strips leading `@`
- `compareRelevance` + `sortAccountResults`
  - exact username match outranks prefix match
  - prefix match outranks others
  - then shorter `PreferredUsername` wins
- `paginateResults`
  - `offset >= len(results)` returns empty slice
  - offset applies first, then limit trims

Tip: you can construct actors with only:

- `ID` (in `BaseObject`)
- `PreferredUsername`

### 2) Following-only filtering

Cover `applyFollowingFilter`:

- when `r.deps == nil` → returns original results and doesn’t panic
- when `deps.GetFollowing` returns a list → only includes actors whose `PreferredUsername` is present
- when `deps.GetFollowing` returns an error → returns original results (fail open)

### 3) Privacy filtering (block rules)

Cover `filterAccountsByPrivacy` with a stubbed `SearchRepositoryDeps`:

- Searcher’s own actor is always included (ID equality short-circuit)
- When `IsBlockedBidirectional` returns `true` → exclude target
- When `IsBlockedBidirectional` returns an error → include target (fail open)
- Nil actor in results is skipped

### 4) The three account search strategies (DB query shape + dedupe)

Cover these internal methods with DynamORM mocks:

- `searchExactUsername`
  - validates query length (`ValidateRepositorySearchQuery(query, 3)`)
  - query shape:
    - `Where("PK","=", fmt.Sprintf("ACTOR#%s", query))`
    - `Where("SK","=", "PROFILE")`
    - `First(&models.Actor{})`
  - adds result only when `exactMatch.Actor != nil` and not already seen by actor ID
- `searchUsernamePrefix`
  - query shape:
    - `Index("gsi1")`
    - `Where("gsi1PK","=", fmt.Sprintf("USERNAME_SEARCH#%s", query[:2]))`
    - `Filter("gsi1SK","BEGINS_WITH", query)`
    - `Limit(limit+offset)`
    - `All(&[]models.Actor{})`
  - dedupe by `Actor.ID`
  - error path: `All` error should not panic; returns no additions
- `searchDisplayName`
  - query shape:
    - `Index("gsi2")`
    - `Where("gsi2PK","=", fmt.Sprintf("NAME_SEARCH#%s", query[:2]))`
    - `Filter("gsi2SK","BEGINS_WITH", query)`
    - `Limit(limit)`
    - `All(&[]models.Actor{})`
  - dedupe by `Actor.ID`
  - error path: `All` error should not panic; returns no additions

Tip: Use **one MockQuery per strategy** so expectations are simple:

- `mockDB.On("Model", ...).Return(mockQueryExact).Once()`
- `mockDB.On("Model", ...).Return(mockQueryPrefix).Once()`
- `mockDB.On("Model", ...).Return(mockQueryName).Once()`

## Deliverables

- New tests in `pkg/storage/repositories/`, suggested filenames:
  - `search_repository_accounts_test.go` (pure logic + deps filtering)
  - `search_repository_accounts_queries_test.go` (DB strategy shapes with DynamORM mocks)
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

