# Agent 4 Brief — Round 11 (`cmd/api` slice D)

## Goal

Raise statement coverage to **≥ 90% per file** for the files listed below (baseline is in `docs/ai-training/coverage-agent-briefs/2025-12-29-round-11/baseline-cmd-api-scoreboard-file.txt`).

## Coordination constraints

- Other agents are working in parallel in `cmd/api/**`.
- **Do not run coverage** (no `./lesser test coverage`, no `./lesser coverage scoreboard`, no `go test -cover*`).
- Run tests only: `go test -count=1 ./cmd/api/...` (or narrower).
- If you hit compile/test failures **outside** this slice, stop and report the error for coordination.
- No external network/AWS in unit tests. If your code path would call out, **refactor to inject a stubbed dependency** and test it.

## Your slice (17 files, ~2787 statements)

- `cmd/api/lift/admin_federation.go` (2.5%, 9/366)
- `cmd/api/lift/accounts.go` (0.0%, 0/343)
- `cmd/api/lift/websocket_cost_analytics.go` (0.0%, 0/267)
- `cmd/api/lift/setup.go` (0.0%, 0/252)
- `cmd/api/lift/oauth.go` (0.0%, 0/237)
- `cmd/api/lift/reputation.go` (0.0%, 0/194)
- `cmd/api/lift/polls.go` (0.0%, 0/183)
- `cmd/api/lift/notes.go` (0.0%, 0/166)
- `cmd/api/lift/debug.go` (0.0%, 0/145)
- `cmd/api/lift/lists.go` (0.0%, 0/132)
- `cmd/api/lift/translation.go` (0.0%, 0/116)
- `cmd/api/lift/follow_requests.go` (0.0%, 0/104)
- `cmd/api/lift/media.go` (0.0%, 0/87)
- `cmd/api/lift/ai.go` (0.0%, 0/79)
- `cmd/api/lift/mutes.go` (0.0%, 0/61)
- `cmd/api/lift/webfinger.go` (0.0%, 0/41)
- `cmd/api/lift/endorsements.go` (0.0%, 0/14)

## Testing strategy (expected)

- Prefer direct handler invocation with Lift context + `httptest.NewRecorder` (no `httptest.NewServer`).
- Mock repos/services at the handler boundary; avoid AWS/network.
- Refactor as needed for testability (inject HTTP clients/remote-call interfaces, isolate side effects, inject time/randomness), but **do not move production code into new files** or “thin main + impl.go” patterns.

## Done criteria

- All slice files are believed to be ≥ 90% (coordinator will verify with the next coverage report).
- `go test -count=1 ./cmd/api/...` passes.
- Then stop and wait for the coordinator’s next coverage snapshot.
