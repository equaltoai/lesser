# Repository Testing Guide (Coverage-Driven)

This guide describes practical techniques to drive high/complete test coverage in Lesser’s repository layer (`pkg/storage/repositories`) without relying on AWS.

## Context

Repository code tends to be “branchy”: validation, conditional writes, pagination loops, error translation, and glue around other repositories/services. The fastest path to high coverage is usually **unit tests with mocked DynamoORM DB/query chains**, plus **direct tests of pure helpers**.

Some packages (notably `pkg/storage/models`) initialize configuration at import-time; repository tests must run with enough environment to avoid panics.

## Prerequisites

### Environment variables (avoid init-time panics)

Many repository tests require these to be set when running `go test`:

- `DYNAMODB_TABLE` (or `DYNAMO_TABLE_NAME`, or `ENVIRONMENT`/`STAGE`)
- `JWT_SECRET`

Example:

```bash
DYNAMODB_TABLE=test-table JWT_SECRET=dummy go test ./pkg/storage/repositories
```

## Procedures

### 1) Pick a target and baseline the file’s coverage

Find the biggest repository file:

```bash
find pkg/storage/repositories -type f -name '*.go' -print0 | xargs -0 wc -l | sort -n | tail -n 20
```

Generate a coverprofile for the repositories package:

```bash
DYNAMODB_TABLE=test-table JWT_SECRET=dummy \
  go test ./pkg/storage/repositories -count=1 -coverprofile /tmp/repositories_cover.out
```

If you’re using the Lesser CLI tooling, you can also inspect the profile with:

```bash
./lesser coverage scoreboard --profile /tmp/repositories_cover.out --mode file \
  --package github.com/equaltoai/lesser/pkg/storage/repositories --top 200
```

Compute coverage for one file (example uses `user_repository.go`):

```bash
awk 'NR>1{split($1,a,":"); if(a[1]=="github.com/equaltoai/lesser/pkg/storage/repositories/user_repository.go"){t+=$2; if($3>0)c+=$2}} END{printf("%.2f%% (%d/%d)\n",c*100/t,c,t)}' \
  /tmp/repositories_cover.out
```

### 2) Use DynamoORM mocks to unit-test DB-heavy branches

Most repositories build a DynamoORM query chain (`db.Model(...).Where(...).Index(...).Scan(...)`, etc.). The `github.com/pay-theory/dynamorm/pkg/mocks` package lets you stub that chain without AWS.

Common patterns:

- Stub `db.Model(...)` to return a `MockQuery`.
- Stub each chained query method to return the same `MockQuery`.
- For `Scan/All/First`, use `Run(...)` to populate the destination pointer.
- Return DynamoORM sentinel errors to cover “not found” and conditional-write branches:
  - `dynamormerrors.ErrItemNotFound`
  - `dynamormerrors.ErrConditionFailed`

Minimal example:

```go
mockDB := new(mocks.MockDB)
mockQuery := new(mocks.MockQuery)

mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
	dest := args.Get(0).(*[]*models.Vouch)
	*dest = []*models.Vouch{{VouchData: `{"id":"v1","from":"a","to":"b"}`}}
}).Return(nil)
```

#### DynamORM mock gotchas (important)

- `MockQuery.Update(fields ...string)` and `MockQuery.Select(fields ...string)` call `m.Called(fields)` (a single `[]string` argument). Even when the production code calls `Update()` with no args, your expectation must match **one** argument:
  - ✅ `mockQuery.On("Update", mock.Anything).Return(err)`
  - ❌ `mockQuery.On("Update").Return(err)` (won’t match)
- When writing a broad `.Maybe()`-based “sweep” mock (e.g. `mockQuery.On("First", mock.Anything)...Maybe()`), add **more specific** `.Once()` expectations **before** the broad one, otherwise the broad matcher can swallow your targeted case.
  - Example: register `mockQuery.On("First", mock.AnythingOfType("*models.StatusMetadata")).Return(dynamormerrors.ErrItemNotFound).Once()` before wiring the permissive `First` handler.

### 3) Use a “coverage sweep” to jump-start large files

When a repository file is very large and coverage is extremely low, start with a permissive “sweep” test that:

- Sets up a broad, `.Maybe()`-driven mock chain for DynamoORM methods used in the file.
- Uses reflection helpers to populate destination slices/structs for `All/Scan/First` with minimally valid data.
- Calls every exported method (and key private helpers) once to execute the majority of statements.

This is a pragmatic way to get to ~70–90% quickly; then you follow up with targeted, readable branch tests (see next section).

Key implementation tricks:

- Prefer *package tests* (`package repositories`) so you can call unexported helpers directly.
- Populate “shape-dependent” fields (IDs, timestamps, map keys) to avoid panics in post-query processing.
- For time-window logic (e.g., “last 24h”), set a base time relative to `time.Now()` so comparisons like `t.After(since)` actually execute both branches.

### 3) Cover error translation (don’t stop at “happy path”)

Repository code typically wraps/normalizes errors via shared error handlers. To cover those branches:

- Return an error from `Create/Update/Delete/BatchWrite/...` mocks.
- Return `ErrItemNotFound` from `First` to hit “not found” translation.
- Feed invalid JSON or invalid inputs to hit unmarshal/validation failures.

Aim for tests that validate:

- The method returns an error (and sometimes the expected domain error).
- The method returns `nil` for “not found” where intended.
- The method continues/filters invalid rows instead of failing the whole operation.

### 4) Mock “advanced” DynamoORM features when needed

Some repositories use:

- `ConsistentRead()`
- `UpdateBuilder().Set(...).Execute()` (for conditional-update flows)

These are mockable via DynamORM’s `MockUpdateBuilder`. Use this when the “conditional check failed → load current → update” path is a big chunk of uncovered code.

### 4) Test pagination loops deterministically

Many repository methods use `limit` + `cursor` loops.

Techniques:

- Provide a first page with `nextCursor != ""` and a second page with `nextCursor == ""` to cover both “continue” and “break”.
- In mocks, use `.Once()` on sequential calls (`First/Scan/All`) to avoid ambiguous expectations.
- Avoid asserting a specific call order if the production code iterates over maps; instead, sort inputs or craft the test to be order-independent.

If you see a test hang, it’s often because your mock returns the same “page” regardless of cursor or filter conditions; make sure each page progresses or eventually returns an empty page.

### 5) Use dependency injection seams (when present)

Some repositories allow swapping dependencies (e.g. other repos/services) or have internal function hooks for hard-to-mock behavior (like transactions).

Patterns to leverage:

- `SetDependencies(...)` to supply a mock that returns controlled responses.
- `SetBookmarkRepository(...)` (or similar) to inject a repository instance with stubbed internal functions.

This keeps tests unit-scoped while still covering wrapper methods and “glue” logic.

### 6) Drive coverage through “data shape” testing

Repositories often branch on the shape/contents of stored data:

- Empty strings vs. non-empty strings (`ValidateRequiredParam` branches).
- Invalid JSON vs. valid JSON.
- Expired TTL vs. active TTL.
- Different enum states.

Create small, explicit fixtures to cover each branch.

### 7) Iterate with coverage visualization

Use `go tool cover` to find the next best gaps:

```bash
go tool cover -func /tmp/repositories_cover.out | rg 'user_repository.go'
go tool cover -html /tmp/repositories_cover.out
```

Iteration loop:

1. Identify uncovered block(s).
2. Decide whether it’s driven by input validation, DB error, pagination, or dependency behavior.
3. Add the smallest test that hits the branch with mocks (or direct helper calls).
4. Re-run package tests with `-count=1` and re-check file coverage.

## Patterns (What Works Well)

- **One behavior per test**: small tests make it easier to see which branch is uncovered.
- **Table-driven tests** for validation-heavy methods: each row toggles one branch.
- **Direct helper tests**: pull helper logic under test without DB mocks when possible.
- **Avoid network/real DynamoDB** for coverage: integration tests are valuable, but slow and not ideal for branch-completion.
- **Stable time**: use `time.Date(...)` rather than `time.Now()` when comparing values.

## Troubleshooting

### Panic on `go test` about DynamoDB table name

Set required env vars at test run time:

```bash
DYNAMODB_TABLE=test-table JWT_SECRET=dummy go test ./pkg/storage/repositories
```

### Test order flakes / unexpected mock calls

- Add `.Once()` to sequential `First/Scan/All` expectations.
- Avoid relying on Go map iteration order; sort inputs or assert sets rather than sequences.

### “Unreachable” branches block 100% coverage

Sometimes code paths are effectively dead due to error wrapping or early returns.

Options:

- Prefer refactoring to make the branch genuinely meaningful/testable (e.g., preserve error typing, expose a seam, split into a helper).
- If the branch is truly dead/obsolete, remove it rather than writing artificial tests.

### Tests exposed a real bug

If a coverage test reveals a runtime panic or incorrect control flow (e.g., slicing beyond length), prefer fixing the repository code rather than “dodging” it in tests. Coverage work is often an effective way to discover correctness issues in glue code.

## Best Practices

- Treat coverage as a **signal**, not the goal: prioritize tests that exercise real behavior and error handling.
- Keep repository tests **fast** and **pure** (mocks + deterministic data).
- When adding seams to enable testing, prefer **small, explicit injection points** over global state.
