# Agent 2 Brief — `cmd/api` (1 large → 2 small, repeat)

## Goal

Raise statement coverage to **≥ 90% per file** (not package-wide), in this order:

1) Large file: `cmd/api/lift/admin.go` (baseline **0.0%**, 0/842 statements)
2) Small file: `cmd/api/lift/vapid_check.go` (baseline **0.0%**, 0/15 statements)
3) Small file: `cmd/api/models/mastodon.go` (baseline **0.0%**, 0/4 statements)

After these 3 files hit ≥ 90%, repeat the cycle within your slice:
- Pick the **next largest** remaining `< 90%` file in `cmd/api/**`.
- Then pick **two small** remaining `< 90%` files in `cmd/api/**`.

## Status

Planned (round 10): not started.

## Constraints (must follow)

- Scope: `cmd/api/**` only (tests + minimal refactors are allowed inside this slice; do not touch other directories without coordination).
- No AWS calls, no external network.
- Do not use `httptest.NewServer` (avoid port binding); `httptest.NewRecorder` is fine.
- Tests must be deterministic (no sleeps; no asserting exact timestamps/UUIDs; no map-order assumptions).
- Do not “game” coverage by moving logic into new files or using build tags to exclude code. If you add a new non-test `.go` file in-scope, it must also reach **≥ 90%**.
- If `go test` fails due to compile errors **outside your assigned targets**, stop and report the error; let the relevant agent/coordinator resolve it.

## Approach (recommended)

- Prefer unit tests for handler/router functions without starting a server; build `http.Request` + `httptest.NewRecorder` and call handlers directly.
- If a file is “all in init/main”, extract a `run()`/`buildRouter()` helper (within your slice) so tests can hit logic without `os.Exit`.

## Validation

```bash
# Fast iteration
go test -short ./cmd/api/... -count=1

# Canonical coverage artifacts
./lesser test coverage --scope all

# File scoreboard for cmd
./lesser coverage scoreboard --profile coverage.out --prefix github.com/equaltoai/lesser/cmd/ --mode file --top 2000 | rg 'cmd/api/lift/admin\\.go|cmd/api/lift/vapid_check\\.go|cmd/api/models/mastodon\\.go'
```
