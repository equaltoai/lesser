# Agent 2 Brief — Round 12 (`cmd/api` slice B)

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

## Your slice (17 files, ~2731 statements below 90%)

- `cmd/api/lift/misc.go` (60.0%, 287/478)
- `cmd/api/lift/moderation.go` (58.5%, 193/330)
- `cmd/api/lift/exports.go` (58.4%, 153/262)
- `cmd/api/lift/tags.go` (41.1%, 99/241)
- `cmd/api/lift/health.go` (74.9%, 149/199)
- `cmd/api/lift/metrics.go` (66.8%, 127/190)
- `cmd/api/lift/notes.go` (0.0%, 0/166)
- `cmd/api/lift/discovery.go` (62.6%, 97/155)
- `cmd/api/lift/push_subscriptions.go` (24.2%, 32/132)
- `cmd/api/lift/timelines.go` (74.6%, 91/122)
- `cmd/api/lift/markers.go` (39.4%, 41/104)
- `cmd/api/lift/interactions.go` (72.0%, 67/93)
- `cmd/api/lift/conversations.go` (73.4%, 58/79)
- `cmd/api/lift/service_registry.go` (0.0%, 0/69)
- `cmd/api/lift/mutes.go` (68.9%, 42/61)
- `cmd/api/lift/favorites.go` (69.2%, 18/26)
- `cmd/api/lift/relationships.go` (66.7%, 16/24)

## Testing strategy (use the harness)

- Use `round11NewHandler` + `round10NewLiftContext` helpers; avoid `httptest.NewServer`.
- Prefer “middleware-driven” error assertions (standard envelope with `error_code`).
- Do not build one-off mocks; use `RegistryStub` and the shared service stubs in `cmd/api/lift/service_registry_stubs_test.go`.
- If you need to edit shared harness/stubs, pause and coordinate first.

## Done criteria

- `go test -count=1 ./cmd/api/...` passes.
- You believe every file in your slice is ≥ 90%.
- Post: files touched + tests run + any blockers, then stop and wait for the coordinator’s next coverage snapshot.
