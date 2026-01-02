# Unit Test Coverage Roadmap (pkg → cmd → graph)

This document defines milestones to reach **≥90% statement coverage** (unit tests) in this order:

1. `pkg/` → **90%**
2. `cmd/` → **90%**
3. `graph/` → **90%**

## Coverage Measurement Contract

**Source of truth:** `go test` statement coverage via `./lesser test coverage --scope all`.

**Exclusions (for “application coverage”):**
- Test code: `*_test.go` (implicitly excluded by Go coverage tooling)
- Test harness packages: `github.com/equaltoai/lesser/pkg/testing/**`, `github.com/equaltoai/lesser/pkg/lift/testing/**`
- Tooling/non-app code: `github.com/equaltoai/lesser/tools/**`, `github.com/equaltoai/lesser/scripts/**`
- Mocks (by path/filename convention): any file path containing `/mocks/`, any file ending in `_mock.go`
- `cmd/api/lift/test_mocks.go` (a testify mock compiled into non-test code)

**Standard commands**
- Generate full raw coverage profile: `./lesser test coverage --scope all`
- Create filtered “application” profile (example):
  - `coverage.out` → `coverage.app.out` by removing excluded paths/files
- Report totals:
  - App total: `go tool cover -func=coverage.app.out | tail -n 1`
  - `pkg` total: `go tool cover -func=coverage.app.pkg.out | tail -n 1`
  - `cmd` total: `go tool cover -func=coverage.app.cmd.out | tail -n 1`
  - `graph` total: `go tool cover -func=coverage.app.graph.out | tail -n 1`
- Find lowest packages/files:
  - `./lesser coverage scoreboard --profile coverage.app.out --mode package --top 50`
  - `./lesser coverage scoreboard --profile coverage.app.out --mode file --top 50`

## Current Baseline (Example)

This roadmap assumes a baseline roughly like:
- `pkg`: ~80%
- `cmd`: ~45%
- `graph`: ~1%

Always re-check at the start of each milestone using the Coverage Measurement Contract.

---

# Milestone 1 — `pkg/` to **≥90%**

## Acceptance Criteria
- `go tool cover -func=coverage.app.pkg.out | tail -n 1` reports **≥90.0%**.
- No coverage achieved by “hiding” behavior (no moving production logic into separate files solely to dodge coverage, no deleting behavior, no build-tagging away code that ships).

## Strategy (Order of Operations)
1. **Triage by uncovered statements (not by percent).**
   - Use `./lesser coverage scoreboard --package github.com/equaltoai/lesser/pkg/` and target the biggest “uncovered statement” buckets first.
2. **Make dependencies injectable via interfaces (inside the package).**
   - Prefer existing repository/service interfaces; if missing, add minimal interfaces around external clients (AWS/HTTP/etc) and inject them.
3. **Test behavior, then error branches.**
   - First, ensure the main happy-path behavior is covered.
   - Second, add table-driven tests for error classification/translation and edge cases.
4. **Use deterministic inputs.**
   - Inject clock/time, UUID/random sources, and network clients; avoid sleeps and real network calls in unit tests.

## Sub-milestones (Repeatable Loop)

### M1.1 — Eliminate “>1000 uncovered statements” buckets in `pkg/`
- Pick the single largest uncovered `pkg` package and two smaller ones.
- Bring each selected package to **≥90%** before moving on.
- Repeat until no `pkg` package has >1000 uncovered statements.

### M1.2 — Reduce the largest remaining uncovered packages (2-at-a-time)
- Pick the two largest remaining uncovered `pkg` packages.
- Raise each to **≥90%**.
- Repeat until `pkg/` total reaches **≥90%**.

### M1.3 — Stabilize test infrastructure (only if needed)
- Consolidate shared test helpers per domain (e.g., storage harnesses, fake clocks) to reduce flaky/duplicated tests.
- Guardrail: helpers must *reduce* test complexity, not introduce parallel mock stacks.

---

# Milestone 2 — `cmd/` to **≥90%**

## Acceptance Criteria
- `go tool cover -func=coverage.app.cmd.out | tail -n 1` reports **≥90.0%**.
- Unit tests stay package-level (`go test ./cmd/<name>`), no integration dependencies required.

## Strategy
1. **Test the handler/entry behavior without running Lambda.**
   - Prefer calling the package’s handler function(s) directly.
   - If the code is currently “all in `main()`”, refactor *within the same file* to a `run(...)` function with injected deps (no file splitting to avoid coverage).
2. **Standardize dependency seams across commands.**
   - Common seams: repository storage, AWS clients, HTTP clients, clock, env/config parsing.
3. **Prioritize the largest `cmd` packages with the most uncovered statements.**
   - Many `cmd/*` packages are currently at 0%; raising the biggest first moves the overall needle fastest.

## Sub-milestones

### M2.1 — Establish a shared `cmd` testing pattern
- Define a repeatable pattern for:
  - Building config for tests
  - Injecting repositories/services
  - Running a handler with a fake event/context
  - Asserting structured outputs and standardized error responses

### M2.2 — “Largest + two smaller” until no `cmd` package has >1000 uncovered statements
- Same loop as Milestone 1, but scoped to `cmd/`.

### M2.3 — Two-largest iteration until `cmd/` total is ≥90%
- Keep iterating by uncovered statements until the `cmd` aggregate crosses 90%.

---

# Milestone 3 — `graph/` to **≥90%**

## Acceptance Criteria
- `go tool cover -func=coverage.app.graph.out | tail -n 1` reports **≥90.0%**.

## Strategy
1. **Treat `graph/` like a public API surface.**
   - Coverage should come primarily from executing GraphQL operations that traverse generated execution code and resolvers.
2. **Build a GraphQL test harness.**
   - Construct the gqlgen server in-process.
   - Inject mocked repositories/services (no DynamoDB/AWS required).
3. **Drive coverage by executing operations, not by unit-testing generated functions.**
   - A small set of high-value operations can execute large portions of generated code.
   - Expand to cover every root field and representative nested selections.

## Sub-milestones

### M3.1 — In-process GraphQL execution harness
- Add a helper to run GraphQL queries/mutations against the gqlgen server without network.
- Provide a single mocked dependency bundle that can satisfy most resolvers.

### M3.2 — Schema-wide operation suite
- Add tests that:
  - Run introspection (sanity)
  - Run at least one query/mutation per root field
  - Exercise error paths (auth required, validation failures, not found)

### M3.3 — Coverage closure loop
- Use per-file scoreboard (`--mode file`) to identify remaining uncovered regions and add targeted operations to hit them.
- Continue until `graph/` total is ≥90%.

---

## Guardrails (Non-Negotiable)
- No moving production logic into new files to claim “testability” if it breaks behavior or hides coverage.
- No weakening compile-time contracts (interfaces, dependency wiring) to make tests pass.
- No real network calls in unit tests.
- If a test requires a dependency seam, add the seam (interface/DI), don’t mock by global patching unless already established in the codebase.

