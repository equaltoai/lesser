# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` holds Lambda handlers that package to `bin/*.zip`.
- `pkg/` hosts shared domain logic (`activitypub`, `services`, `storage`, `streaming`) and supporting helpers.
- `graph/` and `graphql/` contain schema definitions, resolvers, and generated bindings.
- `infra/cdk/` defines AWS stacks and configs; `scripts/`, `docs/`, `examples/`, `tests/`, `integration/`, and `testdata/` supply tooling and fixtures.

## Build, Test, and Development Commands
- `make build-lambdas` packages each function (use `make rebuild-lambdas` to refresh); artifacts land in `bin/`.
- Run `make dev-init` once to scaffold `.env`, then `make dev` to launch the local API; `make local-dynamodb` starts a containerized table when persistence is needed.
- Test with `make test`, `make test-coverage`, or `make test-race`; extend to the harness with `make test-integration`.
- Guard quality with `make fmt`, `make lint`, `make lint-fix`, and `make sec-scan`; deploy via `make deploy-dev|test|live DOMAIN=...` after `make deploy-shared`.

## Coding Style & Naming Conventions
- Format with `go fmt` (tabs, single import groups) and let `golangci-lint` enforce line length, complexity, and vet checks.
- Use concise package names (`pkg/services/account`), CamelCase for exported identifiers, descriptive lowercase filenames, and context-aware errors.
- Document exported APIs sparingly, thread `context.Context` first, and favor small functions to stay within gocognit thresholds.

## Testing Guidelines
- Tests lean on Go’s `testing` package with `stretchr/testify`; run quick passes via `go test -short ./...` or `make test`.
- Deeper scenarios live in `pkg/testing/harness` and `tests/system/`; expect AWS-style env vars and longer runtimes when using `make test-integration`.
- Store fixtures in `testdata/`, service checks in `scripts/seed_runner`, and keep coverage at or above the `coverage.out` baseline (`make test-coverage` regenerates reports).

## Commit & Pull Request Guidelines
- Favor short, present-tense subject lines (`lint green`, `gocognit remediation pass 3`) that describe the primary change; lowercase is preferred unless naming proper nouns.
- PRs should summarize impact, call out executed commands (`make test`, `make lint`), link issues, and include visuals or logs for user-facing work.
- Re-run the full check suite before re-requesting review and confirm automation is green before merging.

## Security & Configuration Notes
- Keep secrets out of source control; `.env`, `tmp/`, and AWS credentials should stay local or in Secrets Manager. Generate signed material via `make ensure-cdn-credentials` and `make ensure-vapid-credentials`.
- Validate `infra/cdk/config/*.yaml` before deploying and coordinate changes to shared stacks (`LesserSharedStack`) because every environment consumes them.
