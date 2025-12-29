# Agent 4 Brief — `cmd/*-processor` (1 large → 2 small, repeat)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Large file: `cmd/moderation-processor/main.go` (baseline **0.0%**, 0/709 statements)
2) Small file: `cmd/moderation-processor/errors.go` (baseline **0.0%**, 0/27 statements)
3) Small file: `cmd/notification-processor/errors.go` (baseline **0.0%**, 0/22 statements)

After these 3 files hit ≥ 90%, repeat the cycle within your slice (`cmd/*-processor/**`):
- Pick the **next largest** remaining `< 90%` file inside `cmd/*-processor/**`.
- Then pick **two small** remaining `< 90%` files inside `cmd/*-processor/**`.

## Status

Planned (round 10): not started.

## Constraints (must follow)

- Scope: `cmd/*-processor/**` only (tests + minimal refactors are allowed inside this slice; do not touch other directories without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding).
- Tests must be deterministic (no sleeps; avoid asserting exact timestamps/UUIDs; no map-order assumptions).
- Do not “game” coverage by moving logic into new files or using build tags to exclude code. If you add a new non-test `.go` file in-scope, it must also reach **≥ 90%**.
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

- Extract testable `run()` helpers that return errors (inside the slice) rather than exercising `main()` directly.
- Prefer dependency injection for clients/services so tests can use fakes/mocks.

## Validation

```bash
# Fast iteration (adjust package list as you work)
go test -short ./cmd/moderation-processor ./cmd/notification-processor -count=1

# Canonical coverage artifacts
./lesser test coverage --scope all

# File scoreboard for cmd
./lesser coverage scoreboard --profile coverage.out --prefix github.com/equaltoai/lesser/cmd/ --mode file --top 2000 | rg 'cmd/moderation-processor/main\\.go|cmd/moderation-processor/errors\\.go|cmd/notification-processor/errors\\.go'
```
