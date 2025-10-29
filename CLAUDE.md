# CLAUDE.md

Guidance for AI pair-programming inside the Lesser repository.

---

## Quick Snapshot _(January 2025)_

- **Language / Toolchain**: Go 1.25, modules, `golangci-lint` 2.5, gqlgen 0.17.
- **Architecture**: Fully serverless on AWS CDK v2. Thirty-six Lambda functions, DynamoDB single-table via DynamORM + Lift, S3 + CloudFront for media, SQS/EventBridge for async work, CloudWatch/X-Ray for observability.
- **Primary APIs**: Mastodon-compatible REST, GraphQL (60+ ops), WebSocket streaming, ActivityPub federation (inbox/outbox).
- **Active Workstreams**: Reducing gocognit violations (see `docs/gocognit-remediation-plan.md`), dependency upgrades (Go 1.25 + AWS SDK v2.39+), Lift/DynamORM consolidation.

---

## Everyday Commands

| Task | Command(s) | Notes |
|------|------------|-------|
| Build all Lambda artifacts | `make build-lambdas` | Incremental, outputs zipped bootstraps in `bin/`. |
| Build specific Lambda | `make build-<name>` | Example: `make build-api`. |
| Rebuild everything | `make build` | Cleans, rebuilds all zips, verifies `go build ./...`. |
| Local API server | `make dev-init` (once) → `make dev` | `.env` provides required config before `go run ./cmd/api`. |
| Unit / full tests | `ENVIRONMENT=test STAGE=test make test` | `go test ./...` panics without `ENVIRONMENT`/`STAGE`. `make test-unit`, `test-integration`, `test-race` are available. |
| Lint / format | `make lint`, `make fmt`, `make lint-fix` | `golangci-lint` config lives in `.golangci.yml`. |
| GraphQL codegen | `make gqlgen` | Regenerates resolver bindings; combines with `make export-schema` to flatten `graph/*.graphql`. |
| Dependency tidy | `make tidy` | Wraps `go mod tidy`. |
| Deploy stacks | `make deploy-dev`, `deploy-test DOMAIN=...`, `deploy-live DOMAIN=...` | All infrastructure is managed through AWS CDK (`infra/cdk`). |

> **Testing note:** set `JWT_SECRET` and `DYNAMODB_ENCRYPTION_KEY` if running tests outside the Makefile. A safe default is:  
> `ENVIRONMENT=test STAGE=test JWT_SECRET=dummy DYNAMODB_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef go test ./...`

---

## Repository Layout (Top-Level)

```
cmd/                    Lambda entry points (36 total)
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
- `schema.graphql` — flattened schema from `make export-schema`.

---

## Architectural Notes

- **Lambdas**: Listed in `Makefile` (`LAMBDAS`). Deployment expects `GOOS=linux`, `GOARCH=arm64`, `CGO_ENABLED=0`.
- **DynamoDB**: Single table with ≥8 GSIs. Access exclusively via DynamORM repositories in `pkg/storage/repositories`. Models live under `pkg/storage/models` and expose `UpdateKeys()` helpers for index consistency.
- **Lift Integration**: Shared auth/middleware logic under `pkg/lift`. Upgrade notes are tracked alongside dependency bumps (`go.mod`).
- **Cost Tracking**: Use `pkg/common/cost` instrumentation when adding DynamoDB/S3 interactions. `go test` defaults enforce telemetry coverage.
- **Observability**: Lambdas emit structured logs via `zap`. CloudWatch dashboards per env (`make dashboard`). Alerts and custom metrics are defined in CDK stacks.

---

## Configuration & Environments

- Required config env vars (runtime): `ENVIRONMENT`, `STAGE`, `JWT_SECRET`, `AWS_REGION`, `DYNAMODB_TABLE`, `S3_BUCKET_NAME`, plus optional federation toggles (see `pkg/config`).
- Local development: `make dev-init` seeds `.env`; `make dev` exports it before running the REST API.
- Deployment secrets (CloudFront keys, VAPID) are created with `make ensure-cdn-credentials` / `make ensure-vapid-credentials` as part of deploy targets.
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

- Schema split across `graph/schema.graphql` (core), `graph/phase2.graphql`, `graph/phase3.graphql`. Export combined schema for clients via `make export-schema`.
- Resolvers bind into services via the registry pattern (`pkg/services/registry.go`). New operations typically require service methods + storage plumbing.
- Ensure dataloaders (`graph/dataloader.go`) handle new entity fetches to avoid N+1 regressions.

---

## Lint / Quality Targets

- `gocognit` threshold = 30. Ten existing violations are tracked in `docs/gocognit-remediation-plan.md` with actionable refactor steps.
- `gocyclo` threshold = 15.
- `golangci-lint` runs as part of CI; keep new code within limits or add scoped `//nolint` with justification.
- Dependency scanning via `make sec-scan` (gosec) and `make vuln-check` (govulncheck) — run before release branches.

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
- `docs/DEPLOYMENT_GUIDE.md` — CDK environments, secrets, DNS guidance.
- `docs/gocognit-remediation-plan.md` — prioritized refactors for complexity debt.
- `Makefile` — authoritative list of supported commands and Lambda targets.
- `infra/cdk/` — source of truth for infrastructure layout; update stacks here rather than ad-hoc AWS changes.

---

Keep this document current. If you add new workflows, services, or architectural constraints, update the relevant section so future AI collaborators (and humans!) stay aligned.
