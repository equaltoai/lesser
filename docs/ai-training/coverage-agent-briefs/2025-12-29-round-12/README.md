# Round 12 — `cmd/api/**` to ≥90% per file (4 agents, coordinator-run coverage)

## Goal

- Raise **statement coverage to ≥ 90% per file** across `cmd/api/**` (all files with measurable statements).
- Use the **standard error contract** and the **shared Lift test harness** so we don’t reinvent mocks or rely on stringly error behavior.

## Baseline (start of Round 12)

- File-level scoreboard: `docs/ai-training/coverage-agent-briefs/2025-12-29-round-12/baseline-cmd-api-scoreboard-file.txt`
- Current total (cmd/api): **52.1%** (6277/12047 statements)

Files already ≥90% (do not touch unless required by your slice work):
- `cmd/api/routes_lift.go`
- `cmd/api/lift/admin.go`
- `cmd/api/lift/vapid_check.go`
- `cmd/api/lift/nodeinfo.go`
- `cmd/api/models/mastodon.go`

## Coordination rules (collision avoidance)

- **Only the coordinator runs coverage.** Agents must not run:
  - `./lesser test coverage ...`
  - `./lesser coverage scoreboard ...`
  - any `go test -cover*` commands
- Agents may run **tests only** (package level):
  - `go test -count=1 ./cmd/api/...`
  - `go test -count=1 ./cmd/api/lift -run TestName`
  - Optional slower sanity check: `./lesser test unit`
- If you hit a compile/test failure **outside your slice**, **stop immediately** and report it for coordination. Do not “fix drive-by” outside scope.

## Non-negotiables

- No coverage gaming: **do not move production logic into new files** or split “thin main + impl.go”.
- No external network or AWS in unit tests. If code would call out, **refactor to inject a stub** (keep logic in the same file).
- No per-file ad-hoc mocks for the same services: use the shared stubs/harness below.

## Shared harness (use this; do not reinvent)

**Lift request + handler harness**
- `cmd/api/lift/round11_test_helpers_test.go`: `round11NewHandler`, `round11TestConfig`
- `cmd/api/lift/round10_test_helpers_test.go`: `round10NewLiftContext` helpers
- Auth tokens: use `round11SignAccessToken(...)` (already used across Round 11 tests)

**API error middleware (standard envelope)**
- Prefer testing handlers through `common.CreateAPIErrorMiddleware(zap.NewNop())` when the handler returns an error.
- Golden assertions must include:
  - HTTP status code
  - JSON error envelope includes `error` and `error_code`
  - `error_code` matches the expected domain code
- Reference contract: `docs/ai-training/ERROR_HANDLING_CONTRACT.md`

**Service mocks (stop duplicating stubs)**
- Use the shared registry stubs:
  - Interfaces/adapters: `cmd/api/lift/service_registry.go`
  - Test stubs: `cmd/api/lift/service_registry_stubs_test.go`
- If you truly need to change shared stub files, **pause and coordinate first** (this is a merge-conflict magnet).

## Coordinator snapshot commands

The coordinator will publish periodic snapshots using:

- `go test -coverprofile=cover_cmd_api.out ./cmd/api/...`
- `./lesser coverage scoreboard -profile cover_cmd_api.out -prefix github.com/equaltoai/lesser/cmd/api/ -mode file -top 120`

Agents should stop and wait once their slice tests pass until the next published snapshot.
