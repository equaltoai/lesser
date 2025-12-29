# Round 11 — `cmd/api/**` coverage push (4 agents, no per-agent coverage runs)

## Goal

- Target: **≥ 90% statement coverage per file** across `cmd/api/**` (files with measurable statements).
- Model: `chatgpt-5.2-codex-high`.
- Constraints: preserve behavior; refactor only when needed to make code testable; **no coverage gaming** (no moving code to “hide” it).

## Coordination rules (collision avoidance)

- **Only the coordinator runs coverage.** Agents must **not** run:
  - `./lesser test coverage ...`
  - `./lesser coverage scoreboard ...`
  - Any `go test -cover*` commands (including package coverprofiles)
- Agents may run **tests only**:
  - `go test -count=1 ./cmd/api/...`
  - Narrowed variants like `go test -count=1 ./cmd/api/lift -run TestName`
- When your assigned slice is “done”, **stop and wait** for the coordinator’s next coverage report before making more coverage-driven changes.

## Refactor policy (high-trust, coverage-driven)

- You are expected to refactor when necessary to make code testable (especially to remove real network/AWS side effects).
- Allowed (and often required): dependency injection hooks (function vars), small interfaces for HTTP/remote calls, splitting large functions, extracting internal helpers, simplifying branching, tightening validation, passing `context.Context`, injecting clocks/randomness.
- Not allowed: moving production logic into new files or duplicate files to change coverage denominators (“thin main + impl.go” patterns), or adding build tags to exclude code.
- If you must move/rename production code:
  - Coordinate first (this is a merge-conflict magnet).
  - The destination must also reach **≥ 90%** coverage and **all packages must still compile**.

## Non-negotiables (avoid “can’t test it” excuses)

- No external network. If code would hit the network (e.g., remote search), **refactor it** to accept an injected client/interface so tests can stub it.
- No AWS calls in unit tests. Same rule: refactor to inject dependencies, and stub them.
- Do not skip branches because they’re “hard” or “too much refactoring”. The whole point of this round is to do the refactor required to make them testable.

## Shared mocks & harness (stop duplicating stubs)

- Use the shared Lift service interfaces + stubs instead of inventing per-file mocks:
  - Production interfaces/adapters: `cmd/api/lift/service_registry.go`
  - Test stubs: `cmd/api/lift/service_registry_stubs_test.go`
- Pattern:
  - Inject services via `Handler{registry: &RegistryStub{...}}` and `*XServiceStub{...Func: ...}`.
  - For remote search (network), inject `Handler{remoteSearch: func(store core.RepositoryStorage) remoteSearchService { ... }}`.
  - For health external HTTP checks, set `healthChecker.httpClient` to a stubbed `*http.Client` (custom `RoundTripper`) instead of calling the real network.
- **Do not** create new `mockAccountsService`, `mockNotesService`, `mockRegistry`, etc. That’s what caused the merge conflicts and duplicated work.
- If you truly need a method that isn’t in the shared interfaces/stubs, **pause and ask the coordinator** (do not independently change the shared harness files).

## Baseline report

- Baseline file-level scoreboard: `docs/ai-training/coverage-agent-briefs/2025-12-29-round-11/baseline-cmd-api-scoreboard-file.txt`
- Notes:
  - `cmd/api/lift/admin.go`, `cmd/api/lift/vapid_check.go`, and `cmd/api/models/mastodon.go` are already ≥ 90% in the baseline; avoid touching unless needed.

## Completion signal (what agents report)

When ready for the next coverage report, post:

- Files you touched (production + tests)
- Commands you ran (`go test ...`)
- Any blockers (especially compile errors outside your slice)
