# Testing

<!-- AI Training: Testing workflows and conventions for Lesser -->

This doc is for contributors running and writing tests in this repo. It covers what the `./lesser` CLI runs, what it
doesn’t, and how to extend coverage safely.

## Quick start

Run the same checks we expect contributors to use locally:

```bash
# Fast unit sweep (used by `./lesser verify`)
./lesser test unit

# Full Go test sweep (all packages)
./lesser test

# Coverage report (writes coverage.out + coverage.html)
./lesser test coverage

# Race detector (slow)
./lesser test race
```

## What `./lesser verify` checks

`./lesser verify` is the repo’s drift + quality guardrail. It currently runs:

- Lambda inventory drift checks (lambda set + CDK inventory alignment)
- Docs drift checks (`scripts/verify_docs.sh`)
- AI-training doc verification (`scripts/verify_ai_training.sh`)
- GraphQL schema verification (`scripts/verify_schema.sh`)
- GraphQL coverage contract verification (`./tools/graphql_coverage`)
- OpenAPI verification (`./tools/openapi`)
- Go unit test sweep (`go test -short ./...`)

Run it before opening a PR:

```bash
./lesser verify
```

## Test types in this repo

### 1) Go unit tests (most common)

✅ CORRECT: prefer deterministic unit tests in `*_test.go` next to the code being tested.

Run:

```bash
./lesser test unit
```

Notes:

- Uses `go test -short ./...`
- The CLI sets safe defaults for `ENVIRONMENT`, `STAGE`, and secrets so tests don’t rely on a local `.env`.

### 2) Go integration harness (longer, AWS-shaped)

The integration harness lives under `pkg/testing/harness/` and is run with build tag `integration`.

Run:

```bash
./lesser test integration
```

When adding harness coverage:

- Prefer explicit, environment-driven configuration (AWS-style env vars).
- Avoid hidden dependencies on local developer state.

### 3) Smoke suites (HTTP-only checks against a deployment)

Smoke scripts are intentionally non-destructive. Use them to sanity check a deployed instance.

Run directly:

```bash
./lesser smoke core --base-url=https://dev.example.com --token="Bearer <token>"
./lesser smoke federation --base-url=https://dev.example.com --username=alice --object-id=<id>
```

Or run through `verify`:

```bash
./lesser verify --smoke --smoke-base-url=https://dev.example.com --smoke-token="Bearer <token>"
```

### 4) Python “system” validation scripts (repo helpers)

There are Python scripts under `tests/system/` and `scripts/` that validate broader flows (often GraphQL-focused).

✅ CORRECT: treat these as workflow validators (end-to-end-ish), not unit tests.

⚠️ WARNING: `./lesser dev seed-and-validate` is destructive to the target DynamoDB table. Only use it on disposable
environments (never live).

```bash
./lesser dev seed-and-validate --aws-profile <profile> --dynamodb-table <table> --base-url https://dev.example.com
```

## Writing tests (repo conventions)

### Prefer table-driven tests

✅ CORRECT:

```go
func TestThing(t *testing.T) {
  tests := []struct{
    name string
    in   string
    want string
  }{
    {name: "empty", in: "", want: ""},
  }
  for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
      require.Equal(t, tt.want, Thing(tt.in))
    })
  }
}
```

### Prefer `stretchr/testify` for assertions

Most packages use `require`/`assert` to reduce boilerplate and improve failure output.

### Avoid network calls in unit tests

✅ CORRECT: stub repositories/services.

❌ INCORRECT: make real AWS calls or require a deployed environment for `./lesser test unit`.

## Coverage workflow

Generate an HTML report:

```bash
./lesser test coverage
```

Artifacts:

- `coverage.out` (machine-readable)
- `coverage.html` (human readable)

### Coverage KPI (pkg-first)

For the “90% coverage” program, use `pkg/` as the primary KPI scope (core domain logic).

Run pkg-only coverage:

```bash
./lesser test coverage --scope pkg
```

Artifacts:

- `coverage_pkg.out` (machine-readable)
- `coverage_pkg.html` (human readable)

Print a package-level scoreboard (largest uncovered first):

```bash
./lesser coverage scoreboard --profile coverage_pkg.out --top 30
```

When pushing coverage up:

- Start with packages that have business logic but few dependencies (pure functions, converters, validators).
- Add tests around error handling and boundary conditions (this tends to increase coverage quickly and safely).
