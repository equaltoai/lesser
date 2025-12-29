# Agent 4 Brief — Round 12 (`cmd/api` slice D)

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

## Your slice (17 files, ~2727 statements below 90%)

- `cmd/api/lift/admin_federation.go` (2.5%, 9/366)
- `cmd/api/lift/accounts.go` (28.9%, 99/343)
- `cmd/api/lift/websocket_cost_analytics.go` (57.8%, 155/268)
- `cmd/api/lift/oembed.go` (44.6%, 112/251)
- `cmd/api/lift/test_mocks.go` (76.7%, 171/223)
- `cmd/api/lift/announcements.go` (79.7%, 153/192)
- `cmd/api/lift/recovery_emailfree.go` (57.0%, 98/172)
- `cmd/api/lift/wallet.go` (62.5%, 100/160)
- `cmd/api/lift/debug.go` (0.0%, 0/145)
- `cmd/api/lift/lists.go` (61.4%, 81/132)
- `cmd/api/lift/custom_emojis.go` (54.4%, 62/114)
- `cmd/api/lift/trends.go` (75.0%, 75/100)
- `cmd/api/lift/preferences.go` (67.9%, 57/84)
- `cmd/api/lift/oauth_consent.go` (74.0%, 57/77)
- `cmd/api/lift/status_interactions.go` (79.5%, 35/44)
- `cmd/api/lift/errors.go` (83.3%, 35/42)
- `cmd/api/lift/endorsements.go` (71.4%, 10/14)

## Testing strategy (avoid duplicate mocks)

- Prefer `round11NewHandler` + `RegistryStub` and shared service stubs in `cmd/api/lift/service_registry_stubs_test.go`.
- Do not create new per-file mock registries or service mocks.
- If you need to add a stub method to the shared stubs file, pause and coordinate first.

## Done criteria

- `go test -count=1 ./cmd/api/...` passes.
- You believe every file in your slice is ≥ 90%.
- Post: files touched + tests run + any blockers, then stop and wait for the coordinator’s next coverage snapshot.
