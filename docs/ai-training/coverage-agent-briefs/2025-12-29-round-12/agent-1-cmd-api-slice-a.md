# Agent 1 Brief — Round 12 (`cmd/api` slice A)

## Goal

Bring every file in this slice to **≥ 90% statement coverage** (coordinator will verify via the next snapshot).

## Coordination constraints

- Other agents are working in parallel in `cmd/api/**`.
- **Do not run coverage** (no `./lesser test coverage`, no `./lesser coverage scoreboard`, no `go test -cover*`).
- Run tests only: `go test -count=1 ./cmd/api/...` (or narrower).
- Optional slower sanity check: `./lesser test unit`.
- If you hit compile/test failures **outside** this slice, **stop** and report the error for coordination.
- No external network/AWS in unit tests; inject stubs and keep logic in the same file.
- Follow the error contract: `docs/ai-training/ERROR_HANDLING_CONTRACT.md` (assert `error_code` in error responses).

## Your slice (17 files, ~2725 statements below 90%)

- `cmd/api/lift/statuses.go` (40.3%, 214/531)
- `cmd/api/lift/helpers.go` (71.2%, 220/309)
- `cmd/api/lift/setup.go` (0.0%, 0/252)
- `cmd/api/lift/oauth.go` (0.0%, 0/237)
- `cmd/api/lift/search.go` (68.2%, 135/198)
- `cmd/api/lift/polls.go` (0.0%, 0/183)
- `cmd/api/lift/webauthn.go` (59.0%, 95/161)
- `cmd/api/lift/apps.go` (52.1%, 76/146)
- `cmd/api/lift/instance.go` (73.6%, 103/140)
- `cmd/api/lift/status_info.go` (65.3%, 77/118)
- `cmd/api/lift/statuses_full.go` (59.0%, 62/105)
- `cmd/api/lift/reports.go` (19.1%, 17/89)
- `cmd/api/lift/ai.go` (48.1%, 38/79)
- `cmd/api/lift/relationships_full.go` (71.4%, 55/77)
- `cmd/api/lift/bookmarks.go` (66.7%, 40/60)
- `cmd/api/lift_handlers.go` (84.0%, 21/25)
- `cmd/api/lift/admin_users.go` (60.0%, 9/15)

## Testing strategy (use the harness)

- Create handlers via `round11NewHandler(t, cfg, state, reg)` from `cmd/api/lift/round11_test_helpers_test.go`.
- Build requests via `round10NewLiftContext(...)` helpers and drive error-producing handlers through `common.CreateAPIErrorMiddleware(zap.NewNop())`.
- Prefer `RegistryStub` + service stubs from `cmd/api/lift/service_registry_stubs_test.go` (do not create new mock service types).
- For error-path assertions, ensure responses include `error_code` and the expected HTTP status.

## Done criteria

- `go test -count=1 ./cmd/api/...` passes.
- You believe every file in your slice is ≥ 90%.
- Post: files touched + tests run + any blockers, then stop and wait for the coordinator’s next coverage snapshot.
