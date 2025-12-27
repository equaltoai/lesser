# Repository Guidelines

## Project Structure & Module Organization
- `cmd/` holds Lambda handlers that package to `bin/*.zip`.
- `pkg/` hosts shared domain logic (`activitypub`, `services`, `storage`, `streaming`) and supporting helpers.
- `graph/` contains GraphQL schema definitions, resolvers, and generated bindings.
- `infra/cdk/` defines AWS stacks and configs; `scripts/`, `docs/`, `examples/`, `tests/`, `integration/`, and `testdata/` supply tooling and fixtures.

## Build, Test, and Development Commands
- Build CLI: `go build -o lesser ./cmd/lesser`
- Package lambdas: `./lesser build lambdas` (use `./lesser build lambdas --rebuild` to force rebuild); artifacts land in `bin/`.
- Local dev: `./lesser dev init` (once) → `./lesser dev`; `./lesser dev dynamodb` starts a containerized table when persistence is needed.
- Test with `./lesser test`, `./lesser test coverage`, or `./lesser test race`; extend to the harness with `./lesser test integration`.
- Guard quality with `./lesser fmt`, `./lesser lint [--fix]`, `./lesser sec-scan`, and `./lesser vuln-check`; deploy via `./lesser up --app <slug> --base-domain <example.com> --aws-profile <profile>`.

## Coding Style & Naming Conventions
- Format with `go fmt` (tabs, single import groups) and let `golangci-lint` enforce line length, complexity, and vet checks.
- Use concise package names (`pkg/services/account`), CamelCase for exported identifiers, descriptive lowercase filenames, and context-aware errors.
- Document exported APIs sparingly, thread `context.Context` first, and favor small functions to stay within gocognit thresholds.

## Testing Guidelines
- Tests lean on Go’s `testing` package with `stretchr/testify`; run quick passes via `go test -short ./...` or `./lesser test unit`.
- Deeper scenarios live in `pkg/testing/harness` and `tests/system/`; expect AWS-style env vars and longer runtimes when using `./lesser test integration`.
- Store fixtures in `testdata/`, service checks in `scripts/seed_runner`, and keep coverage at or above the `coverage.out` baseline (`./lesser test coverage` regenerates reports).

## Commit & Pull Request Guidelines
- Favor short, present-tense subject lines (`lint green`, `gocognit remediation pass 3`) that describe the primary change; lowercase is preferred unless naming proper nouns.
- PRs should summarize impact, call out executed commands (`./lesser test`, `./lesser lint`), link issues, and include visuals or logs for user-facing work.
- Re-run the full check suite before re-requesting review and confirm automation is green before merging.

## Security & Configuration Notes
- Keep secrets out of source control; `.env`, `tmp/`, and AWS credentials should stay local or in Secrets Manager. Use `docs/deployment.md` for deployment credential requirements.
- `infra/cdk/config/` contains reference templates and is not loaded by the CDK app; treat `infra/cdk/stacks/` + `infra/cdk/inventory/` as the source of truth.
- Coordinate changes to shared stacks (`LesserSharedStack`) because every environment consumes them.
