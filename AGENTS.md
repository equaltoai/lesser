# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` owns Lambda entry points (e.g. `cmd/api`, `cmd/graphql`, `cmd/inbox`) that compile to `bin/*.zip`.
- `pkg/` hosts reusable domain logic (`activitypub`, `services`, `storage`, `streaming`) plus cross-cutting helpers.
- `graph/` and `graphql/` contain the schema, resolvers, and generated bindings; regenerate after schema edits.
- `infra/cdk/` defines AWS stacks and configs, while `scripts/`, `docs/`, `examples/`, `tests/`, `integration/`, and `testdata/` provide tooling, references, and fixtures.

## Build, Test, and Development Commands
- `make build-lambdas` packages each Lambda (use `make rebuild-lambdas` when binaries must be refreshed); artifacts land in `bin/`.
- Run `make dev-init` once to scaffold `.env`, then `make dev` to launch the local API via `go run ./cmd/api`; `make local-dynamodb` starts a containerized table if you need persistence.
- Test with `make test`, `make test-coverage`, or `make test-race`; add the longer harness suite through `make test-integration`.
- Guard quality with `make fmt`, `make lint`, `make lint-fix`, and `make sec-scan`; deploy using `make deploy-dev|test|live DOMAIN=...` after `make deploy-shared` provisions shared secrets.

## Coding Style & Naming Conventions
- Format with `go fmt` (tabs, single import groups) and let `golangci-lint` enforce line length, complexity, and vet checks.
- Use concise package names (`pkg/services/account`), CamelCase for exported identifiers, descriptive lowercase filenames, and context-aware errors.
- Document exported APIs sparingly, thread `context.Context` first, and favor small functions to stay within gocognit thresholds.

## Testing Guidelines
- Tests lean on Go’s `testing` package with `stretchr/testify`; run quick passes using `go test -short ./...` or `make test`.
- Deeper scenarios live in `pkg/testing/harness` and `tests/system/`; expect AWS-style env vars and longer runtimes when you invoke `make test-integration`.
- Store fixtures in `testdata/`, update GraphQL validation scripts in `scripts/seed_runner`, and keep coverage at or above the `coverage.out` baseline (`make test-coverage` regenerates reports).

## Commit & Pull Request Guidelines
- Favor short, present-tense subject lines (`lint green`, `gocognit remediation pass 3`) that describe the primary change; lowercase is preferred unless naming proper nouns.
- PRs should summarize impact, call out executed commands (`make test`, `make lint`), link issues, and include visuals or logs for user-facing work.
- Re-run the full check suite before re-requesting review and confirm automation is green before merging.

## Security & Configuration Notes
- Keep secrets out of source control; `.env`, `tmp/`, and AWS credentials should stay local or in Secrets Manager. Generate signed material via `make ensure-cdn-credentials` and `make ensure-vapid-credentials`.
- Validate `infra/cdk/config/*.yaml` before deploying and coordinate changes to shared stacks (`LesserSharedStack`) because every environment consumes them.
