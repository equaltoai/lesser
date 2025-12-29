# Agent 2 Brief — Round 11 (`cmd/api` slice B)

## Goal

Raise statement coverage to **≥ 90% per file** for the files listed below (baseline is in `docs/ai-training/coverage-agent-briefs/2025-12-29-round-11/baseline-cmd-api-scoreboard-file.txt`).

## Coordination constraints

- Other agents are working in parallel in `cmd/api/**`.
- **Do not run coverage** (no `./lesser test coverage`, no `./lesser coverage scoreboard`, no `go test -cover*`).
- Run tests only: `go test -count=1 ./cmd/api/...` (or narrower).
- If you hit compile/test failures **outside** this slice, stop and report the error for coordination.
- No external network/AWS in unit tests. If your code path would call out (e.g., “remote search”), **refactor to inject a stubbed dependency** and test it.

## Your slice (17 files, ~2786 statements)

- `cmd/api/lift/misc.go` (0.0%, 0/478)
- `cmd/api/lift/moderation.go` (0.0%, 0/330)
- `cmd/api/lift/exports.go` (0.0%, 0/262)
- `cmd/api/lift/accounts_full.go` (0.0%, 0/250)
- `cmd/api/lift/scheduled_statuses.go` (0.0%, 0/212)
- `cmd/api/lift/search.go` (0.0%, 0/193)
- `cmd/api/lift/statuses_unified_boost.go` (0.0%, 0/182)
- `cmd/api/lift/discovery.go` (0.0%, 0/155)
- `cmd/api/lift/instance.go` (0.0%, 0/140)
- `cmd/api/middleware.go` (0.0%, 0/122)
- `cmd/api/lift/custom_emojis.go` (0.0%, 0/114)
- `cmd/api/lift/interactions.go` (0.0%, 0/93)
- `cmd/api/lift/conversations.go` (0.0%, 0/79)
- `cmd/api/lift/oauth_consent.go` (0.0%, 0/76)
- `cmd/api/lift/status_interactions.go` (0.0%, 0/44)
- `cmd/api/lift/errors.go` (0.0%, 0/42)
- `cmd/api/lift/nodeinfo.go` (0.0%, 0/14)

## Testing strategy (expected)

- Prefer direct handler invocation with Lift context + `httptest.NewRecorder` (no `httptest.NewServer`).
- Mock repos/services at the handler boundary; avoid AWS/network.
- Refactor as needed for testability (inject HTTP clients/remote-call interfaces, isolate side effects, inject time/randomness), but **do not move production code into new files** or “thin main + impl.go” patterns.

## Done criteria

- All slice files are believed to be ≥ 90% (coordinator will verify with the next coverage report).
- `go test -count=1 ./cmd/api/...` passes.
- Then stop and wait for the coordinator’s next coverage snapshot.
