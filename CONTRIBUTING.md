# Contributing to Lesser

<!-- AI Training: Contributor workflow and repo conventions for Lesser -->

This doc describes how to contribute changes safely to the Lesser repository.

## Quick start (local dev)

Build the repo CLI (the only “build step” we document explicitly):

```bash
go build -o lesser ./cmd/lesser
```

Initialize local `.env` (one-time):

```bash
./lesser dev init
```

Run the API:

```bash
./lesser dev
```

## Before opening a PR

Run the same guardrails we expect to stay green:

```bash
./lesser fmt
./lesser lint
./lesser sec-scan
./lesser vuln-check
./lesser test unit
./lesser verify
```

Notes:

- `./lesser verify` includes drift checks (docs, schema, OpenAPI, GraphQL coverage, inventory) plus `go test -short ./...`.
- `./lesser test` runs the full unit test sweep (all packages).

### Quality bar (what we gate on)

The project aims for “secure by default” and predictable contributor workflows. New work should preserve these properties:

- **One-shot CI verification**: `./lesser verify ci` (runs `lint`, security scans, supply chain verification, and `verify all` without mutating tracked files).
- **Audit gates**: `./lesser verify audit` (run as part of `./lesser verify ci`) prevents common regressions while security/maintainability work is in flight. Baselines live in `tools/audit_gates/baseline.yml` and should be updated in the same PR that removes the underlying issue.
- **GraphQL abuse limits**: depth/complexity/token/timeout limits must remain enabled in non-debug deployments (tunable via env vars; see `docs/configuration.md`).
- **SSRF/URL validation**: untrusted outbound URLs must pass the shared URL/SSRF validator and use hardened dialing paths (no bespoke validators).
- **No secrets in logs**: sensitive values must be scrubbed/redacted; add tests when touching auth/logging.

## Branching and releases

Lesser uses `feature -> staging -> main`. Open feature PRs against `staging`; the existing `verify` job (`./lesser verify ci`) is the required feature -> staging gate. `main` is operator-owned and accepts promotion PRs from `staging` only; do not require or rerun the lesser verify/rubric gate for staging -> main promotion. Releases are manual `v*` tags cut from `main`.

See `docs/release-branching.md` for the branch-protection specs and operator apply commands.

## Repo conventions

### CLI-first workflows

✅ CORRECT: document and automate repo workflows via `./lesser …`.

❌ INCORRECT: document contributor workflows via `make …` targets.

The `Makefile` remains for CI/back-compat, but should be treated as internal (contributors should use the CLI).

### Go style

- Use `gofmt` (via `./lesser fmt`) and keep imports in one group.
- Document exported APIs sparingly; add comments when the “why” or contract isn’t obvious.
- Thread `context.Context` first in function signatures.
- Prefer small functions to stay within `gocognit` thresholds.

### Testing

- Prefer deterministic unit tests next to the code they cover (`*_test.go`).
- Use table-driven tests and `stretchr/testify` (`require`/`assert`).
- Avoid real AWS calls in unit tests (use fakes/mocks).

See `docs/testing.md` for more details.

## Common tasks

### Change GraphQL schema

Schema sources live in `graph/*.graphql`. After changes:

```bash
./lesser schema
./lesser verify schema
```

### Change REST routes (OpenAPI)

OpenAPI is generated and published as `docs/contracts/openapi.yaml`.

```bash
./lesser generate openapi
./lesser verify openapi --strict
```

## Debugging tips

Enable debug logging:

```bash
export DEBUG=true
export LOG_LEVEL=debug
```

Enable X-Ray tracing in local runs (if needed):

```bash
export XRAY_TRACING_ENABLED=true
```

## Reporting issues

Use GitHub issues for bugs and feature requests. Include:

- Clear reproduction steps
- Expected vs actual behavior
- Go version and OS
- AWS region/stage (if deployed)

## Security issues

Do not open public issues for security reports. Use the project’s preferred security reporting channel (see the repo’s
security policy, if present) or contact maintainers privately.
