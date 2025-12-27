# CLAUDE.md

Guidance for AI pair-programming inside the Lesser repository.

---

## Quick Snapshot _(January 2025)_

- **Language / Toolchain**: Go 1.25, modules, `golangci-lint` 2.5, gqlgen 0.17.
- **Architecture**: Fully serverless on AWS CDK v2. Lambda fleet defined by `Makefile` `LAMBDAS` (currently 38 deployment functions), DynamoDB single-table via DynamORM + Lift, S3 + CloudFront for media, SQS/EventBridge for async work, CloudWatch/X-Ray for observability.
- **Primary APIs**: Mastodon-compatible REST, GraphQL (60+ ops), WebSocket streaming, ActivityPub federation (inbox/outbox).
- **Active Workstreams**: Dependency upgrades (Go + AWS SDK v2), Lift/DynamORM consolidation, docs/guardrail upkeep.

---

## Everyday Commands

| Task | Command(s) | Notes |
|------|------------|-------|
| Build Lesser CLI | `go build -o lesser ./cmd/lesser` | Only user-facing build step. |
| Build all Lambda artifacts | `./lesser build lambdas` | Outputs zipped bootstraps in `bin/`. |
| Build specific Lambda | `./lesser build lambda <name>` | Example: `./lesser build lambda api`. |
| Rebuild everything | `./lesser build` | Cleans and rebuilds all deploy artifacts. |
| Local API server | `./lesser dev init` (once) → `./lesser dev` | `.env` provides required config before `go run ./cmd/api`. |
| Unit / full tests | `./lesser test` | Sets safe defaults for `ENVIRONMENT`/`STAGE`/secrets. |
| Lint / format | `./lesser lint`, `./lesser fmt`, `./lesser lint --fix` | `golangci-lint` config lives in `.golangci.yml`. |
| GraphQL codegen | `./lesser gqlgen` | Regenerates resolver bindings. |
| Publish client schema | `./lesser schema` / `./lesser export-schema` | `docs/contracts/graphql-schema.graphql` → `schema.graphql`. |
| Dependency tidy | `./lesser tidy` | Wraps `go mod tidy`. |
| Deploy stacks | `./lesser up --app <slug> --base-domain <example.com> --aws-profile <profile> --out <path>` | Deploys shared + stage stacks via CDK. |

> **Testing note:** set `JWT_SECRET` and `DYNAMODB_ENCRYPTION_KEY` if running tests outside the Makefile. A safe default is:  
> `ENVIRONMENT=test STAGE=test JWT_SECRET=dummy DYNAMODB_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef go test ./...`

---

## Repository Layout (Top-Level)

```
cmd/                    Lambda entry points (see Makefile LAMBDAS)
pkg/                    Business logic, services, storage, Lift adapters
  activitypub/          Protocol implementation
  lift/                 Lift middleware/factories
  services/             Domain services (accounts, notes, hashtags, etc.)
  storage/              DynamORM models & repositories
  streaming/            WebSocket + push delivery
graph/                  GraphQL schema fragments + generated resolvers
infra/cdk/              AWS CDK app (stacks, constructs, env config)
tests/system/           Python end-to-end suites
scripts/                Helper scripts (credentials, bootstrap, etc.)
docs/                   Architecture notes, remediation plans, deployment guides
```

Generated files:
- `graph/schema.resolvers.go`, `graph/resolver.go` — managed by gqlgen.
- `bin/*.zip` — Lambda deployment bundles (ignored by git).
- `schema.graphql` — flattened schema from `./lesser export-schema`.

---

## Architectural Notes

- **Lambdas**: Inventory lives under `infra/cdk/inventory/`. Deployment expects `GOOS=linux`, `GOARCH=arm64`, `CGO_ENABLED=0`.
- **DynamoDB**: Single table with ≥8 GSIs. Access exclusively via DynamORM repositories in `pkg/storage/repositories`. Models live under `pkg/storage/models` and expose `UpdateKeys()` helpers for index consistency.
- **Lift Integration**: Shared auth/middleware logic under `pkg/lift`. Upgrade notes are tracked alongside dependency bumps (`go.mod`).
- **Cost Tracking**: Use `pkg/common/cost` instrumentation when adding DynamoDB/S3 interactions. `go test` defaults enforce telemetry coverage.
- **Observability**: Lambdas emit structured logs via `zap`. CloudWatch dashboards per env (`./lesser dashboard`). Alerts and custom metrics are defined in CDK stacks.

---

## Configuration & Environments

- Required config env vars (runtime): `ENVIRONMENT`, `STAGE`, `JWT_SECRET`, `AWS_REGION`, `DYNAMODB_TABLE`, `S3_BUCKET_NAME`, plus optional federation toggles (see `pkg/config`).
- Local development: `./lesser dev init` seeds `.env`; `./lesser dev` exports it before running the REST API.
- Deployment credential requirements live in `docs/deployment.md`.
- Integration tests use the `integration` build tag and expect AWS resources; they are slow and require valid credentials.

---

## Data & Storage Guardrails

1. **No raw AWS SDK calls** inside repositories — stick to DynamORM patterns (`r.db.WithContext(ctx)...`).
2. **Respect key casing & prefixes**. Common patterns (confirm in `pkg/storage/models` before changing):
   - Users: `PK="USER#<username>"`, `SK="PROFILE"`.
   - Actors: `PK="ACTOR#<username>"`, `SK="PROFILE"`.
   - Statuses: `PK="STATUS#<id>"`, `SK="STATUS"`, plus GSI projections.
   - Relationship data, pins, notes follow `"REL#…"`, `"PIN#"`, `"NOTE#"` conventions.
3. When adding models:
   - Define struct + tags in `pkg/storage/models`.
   - Implement `UpdateKeys()` including GSI keys, TTLs, version fields.
   - Add repository methods under `pkg/storage/repositories` and wire them into interfaces in `pkg/storage`.
4. Verify migrations against legacy behavior; compare with any remaining `pkg/storage/original` references before pruning.

---

## GraphQL & Service Layer

- Schema sources live under `graph/*.graphql` (see `gqlgen.yml`). The published client schema is `docs/contracts/graphql-schema.graphql` (generate via `./lesser schema` / export via `./lesser export-schema`).
- Resolvers bind into services via the registry pattern (`pkg/services/registry.go`). New operations typically require service methods + storage plumbing.
- Ensure dataloaders (`graph/dataloader.go`) handle new entity fetches to avoid N+1 regressions.

---

## Lint / Quality Targets

- `gocognit` threshold = 30.
- `gocyclo` threshold = 15.
- `golangci-lint` runs as part of CI; keep new code within limits or add scoped `//nolint` with justification.
- Dependency scanning via `./lesser sec-scan` (gosec) and `./lesser vuln-check` (govulncheck) — run before release branches.

---

## Working With AI Agents

When delegating work to automated agents (e.g., Lift/DynamORM migrations), always provide:

1. **Legacy Reference**: List exact files to study, key patterns to preserve, and expected repository interfaces.
2. **Strict Constraints**:
   - No direct `github.com/aws/aws-sdk-go` imports in repositories.
   - No delegation back to any `originalStorage` implementations.
   - Match error semantics and returned models exactly.
3. **Verification Checklist** (run manually):
   - `rg "github.com/aws/aws-sdk-go" pkg/storage/repositories` → must be empty.
   - `rg "dynamodb\." pkg/storage/repositories` → must be empty.
   - `go build ./pkg/storage/...`
   - Tests or targeted harness coverage if behavior changed.
4. **Key Pattern Audit**: Validate PK/SK/GSI keys and TTLs align with legacy code before approving.

Never merge agent output without human review and the above checks.

---

## Helpful References

- `README.md` — high-level product overview and deployment walkthrough.
- `docs/deployment.md` — CDK environments, secrets, DNS guidance.
- `docs/contracts/README.md` — API contract generation and verification.
- `cmd/lesser/` — source for the user-facing CLI.
- `infra/cdk/` — source of truth for infrastructure layout; update stacks here rather than ad-hoc AWS changes.

---

Keep this document current. If you add new workflows, services, or architectural constraints, update the relevant section so future AI collaborators (and humans!) stay aligned.
