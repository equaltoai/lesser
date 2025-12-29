# Agent 1 Brief — Round 11 (`cmd/api` slice A)

## Goal

Raise statement coverage to **≥ 90% per file** for the files listed below (baseline is in `docs/ai-training/coverage-agent-briefs/2025-12-29-round-11/baseline-cmd-api-scoreboard-file.txt`).

## Coordination constraints

- Other agents are working in parallel in `cmd/api/**`.
- **Do not run coverage** (no `./lesser test coverage`, no `./lesser coverage scoreboard`, no `go test -cover*`).
- Run tests only: `go test -count=1 ./cmd/api/...` (or narrower).
- If you hit compile/test failures **outside** this slice, stop and report the error for coordination.
- No external network/AWS in unit tests. If your code path would call out, **refactor to inject a stubbed dependency** and test it.

## Your slice (17 files, ~2780 statements)

- `cmd/api/lift/statuses.go` (0.0%, 0/519)
- `cmd/api/lift/helpers.go` (0.6%, 2/309)
- `cmd/api/routes_lift.go` (0.0%, 0/256)
- `cmd/api/lift/tags.go` (0.0%, 0/241)
- `cmd/api/lift/status_pins.go` (0.0%, 0/211)
- `cmd/api/lift/announcements.go` (0.0%, 0/192)
- `cmd/api/lift/recovery_emailfree.go` (0.0%, 0/172)
- `cmd/api/lift/wallet.go` (0.0%, 0/160)
- `cmd/api/lift/quotes.go` (0.0%, 0/143)
- `cmd/api/lift/timelines.go` (0.0%, 0/122)
- `cmd/api/lift/statuses_full.go` (0.0%, 0/105)
- `cmd/api/lift/trends.go` (0.0%, 0/100)
- `cmd/api/lift/preferences.go` (0.0%, 0/84)
- `cmd/api/lift/domain_blocks.go` (0.0%, 0/66)
- `cmd/api/lift/bookmarks.go` (0.0%, 0/60)
- `cmd/api/lift_handlers.go` (0.0%, 0/25)
- `cmd/api/lift/admin_users.go` (0.0%, 0/15)

## Testing strategy (expected)

- Prefer direct handler invocation with Lift context + `httptest.NewRecorder` (no `httptest.NewServer`).
- Mock repos/services at the handler boundary; avoid AWS/network.
- Refactor as needed for testability (inject HTTP clients/remote-call interfaces, isolate side effects, inject time/randomness), but **do not move production code into new files** or “thin main + impl.go” patterns.

## Done criteria

- All slice files are believed to be ≥ 90% (coordinator will verify with the next coverage report).
- `go test -count=1 ./cmd/api/...` passes.
- Then stop and wait for the coordinator’s next coverage snapshot.
