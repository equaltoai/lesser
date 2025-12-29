# Agent 3 Brief — Round 11 (`cmd/api` slice C)

## Goal

Raise statement coverage to **≥ 90% per file** for the files listed below (baseline is in `docs/ai-training/coverage-agent-briefs/2025-12-29-round-11/baseline-cmd-api-scoreboard-file.txt`).

## Coordination constraints

- Other agents are working in parallel in `cmd/api/**`.
- **Do not run coverage** (no `./lesser test coverage`, no `./lesser coverage scoreboard`, no `go test -cover*`).
- Run tests only: `go test -count=1 ./cmd/api/...` (or narrower).
- If you hit compile/test failures **outside** this slice, stop and report the error for coordination.
- No external network/AWS in unit tests. If your code path would call out, **refactor to inject a stubbed dependency** and test it.

## Your slice (17 files, ~2785 statements)

- `cmd/api/lift/filters.go` (0.0%, 0/377)
- `cmd/api/main.go` (0.0%, 0/342)
- `cmd/api/lift/imports.go` (0.0%, 0/262)
- `cmd/api/lift/oembed.go` (0.0%, 0/251)
- `cmd/api/lift/test_mocks.go` (13.5%, 30/223)
- `cmd/api/lift/health.go` (0.0%, 0/198)
- `cmd/api/lift/metrics.go` (0.0%, 0/190)
- `cmd/api/lift/webauthn.go` (0.0%, 0/161)
- `cmd/api/lift/apps.go` (0.0%, 0/146)
- `cmd/api/lift/push_subscriptions.go` (0.0%, 0/132)
- `cmd/api/lift/status_info.go` (0.0%, 0/118)
- `cmd/api/lift/markers.go` (0.0%, 0/104)
- `cmd/api/lift/reports.go` (0.0%, 0/89)
- `cmd/api/lift/relationships_full.go` (0.0%, 0/77)
- `cmd/api/lift/handler.go` (10.8%, 7/65)
- `cmd/api/lift/favorites.go` (0.0%, 0/26)
- `cmd/api/lift/relationships.go` (0.0%, 0/24)

## Testing strategy (expected)

- Prefer direct handler invocation with Lift context + `httptest.NewRecorder` (no `httptest.NewServer`).
- Mock repos/services at the handler boundary; avoid AWS/network.
- Refactor as needed for testability (inject HTTP clients/remote-call interfaces, isolate side effects, inject time/randomness), but **do not move production code into new files** or “thin main + impl.go” patterns.

## Done criteria

- All slice files are believed to be ≥ 90% (coordinator will verify with the next coverage report).
- `go test -count=1 ./cmd/api/...` passes.
- Then stop and wait for the coordinator’s next coverage snapshot.
