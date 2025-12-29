# Agent 3 Brief — Round 12 (`cmd/api` slice C)

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

## Your slice (17 files, ~2721 statements below 90%)

- `cmd/api/lift/filters.go` (56.0%, 211/377)
- `cmd/api/main.go` (24.6%, 84/342)
- `cmd/api/lift/imports.go` (25.2%, 66/262)
- `cmd/api/lift/accounts_full.go` (62.4%, 156/250)
- `cmd/api/lift/scheduled_statuses.go` (58.6%, 126/215)
- `cmd/api/lift/reputation.go` (0.0%, 0/194)
- `cmd/api/lift/statuses_unified_boost.go` (65.9%, 120/182)
- `cmd/api/lift/status_pins.go` (69.4%, 109/157)
- `cmd/api/lift/quotes.go` (58.7%, 84/143)
- `cmd/api/middleware.go` (69.7%, 85/122)
- `cmd/api/lift/translation.go` (71.8%, 84/117)
- `cmd/api/lift/follow_requests.go` (61.5%, 64/104)
- `cmd/api/lift/media.go` (0.0%, 0/87)
- `cmd/api/lift/domain_blocks.go` (60.6%, 40/66)
- `cmd/api/lift/handler.go` (42.6%, 26/61)
- `cmd/api/lift/webfinger.go` (68.3%, 28/41)
- `cmd/api/lift/remote_search_deps.go` (0.0%, 0/1)

## Special note: `cmd/api/main.go`

Unit tests skip `init()` when `common.RunningUnitTests()` is true; to reach ≥90% you will likely need in-file refactors:
- Extract side-effectful init logic into test-callable functions **in the same file** (no new files).
- Inject/stub AWS/Lambda dependencies via function variables or small interfaces so unit tests don’t call AWS.

## Done criteria

- `go test -count=1 ./cmd/api/...` passes.
- You believe every file in your slice is ≥ 90%.
- Post: files touched + tests run + any blockers, then stop and wait for the coordinator’s next coverage snapshot.
