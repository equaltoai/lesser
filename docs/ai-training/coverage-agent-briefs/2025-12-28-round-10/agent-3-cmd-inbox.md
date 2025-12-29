# Agent 3 Brief — `cmd/inbox` (1 large → 2 small, repeat)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Large file: `cmd/inbox/main.go` (baseline **0.0%**, 0/1504 statements)
2) Small file: `cmd/inbox/errors.go` (baseline **0.0%**, 0/64 statements)
3) Small file: `cmd/inbox/recovery_handler.go` (baseline **0.0%**, 0/34 statements)

After these 3 files hit ≥ 90%, repeat the cycle within your slice (`cmd/inbox/**`):
- Pick the **next largest** remaining `< 90%` file in `cmd/inbox/**`.
- Then pick **two small** remaining `< 90%` files in `cmd/inbox/**`.

## Status

Planned (round 10): not started.

## Constraints (must follow)

- Scope: `cmd/inbox/**` only (tests + minimal refactors are allowed inside this slice; do not touch other directories without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding); in-memory request/response testing is fine.
- Tests must be deterministic (no sleeps; avoid asserting exact timestamps/UUIDs; no map-order assumptions).
- Do not “game” coverage by moving logic into new files or using build tags to exclude code. If you add a new non-test `.go` file in-scope, it must also reach **≥ 90%**.
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

- Treat `main.go` as composition: extract a `run()`/`handle()` helper (within `cmd/inbox/**`) that returns errors instead of calling `os.Exit`, so tests can execute all branches.
- Mock/stub any external clients and keep tests pure/in-memory.

## Validation

```bash
# Fast iteration
go test -short ./cmd/inbox -count=1

# Canonical coverage artifacts
./lesser test coverage --scope all

# File scoreboard for cmd
./lesser coverage scoreboard --profile coverage.out --prefix github.com/equaltoai/lesser/cmd/ --mode file --top 2000 | rg 'cmd/inbox/main\\.go|cmd/inbox/errors\\.go|cmd/inbox/recovery_handler\\.go'
```
