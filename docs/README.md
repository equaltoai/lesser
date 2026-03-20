# Lesser Documentation

<!-- AI Training: This is the documentation index for Lesser -->

This directory contains the canonical operator + developer documentation for Lesser.

✅ CORRECT: treat `docs/` as the source of truth for how to deploy, operate, and integrate with this repo.

❌ INCORRECT: rely on stale planning docs or one-off session notes (those are archived and not maintained).

## Start here (operators)

### Deploy (locked but reachable)

```bash
go build -o lesser ./cmd/lesser
./lesser up --app my-lesser --base-domain example.com --aws-profile Penny --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

After deploy, dev + live are reachable but **locked**. Verify:

```bash
curl -s "https://dev.example.com/setup/status" | jq .
```

### Activate

Open the setup wizard and follow the prompts:

- `https://dev.<base-domain>/auth/setup`
- `https://<base-domain>/auth/setup`

## Start here (developers)

```bash
go build -o lesser ./cmd/lesser
./lesser dev init
./lesser dev
```

## Start here (client teams)

- REST contract (OpenAPI): `docs/contracts/openapi.yaml`
- GraphQL contract (schema): `docs/contracts/graphql-schema.graphql`
- “How to use the APIs”: `docs/api-reference.md`
- “How to build a Greater client UI”: `docs/guides/CLIENT_APP_GUIDE.md`

## Command Convention

All user-facing workflows are documented via the `lesser` CLI (`./lesser ...`). The `Makefile` is treated as
internal/CI-only (except for building the `lesser` CLI itself).

## Docs Conventions

- Prefer `kebab-case.md` for new operator/developer docs.
- Some older architecture deep dives still use `UPPER_SNAKE_CASE.md`; treat these as legacy naming until migrated.
- Planning/audit/session notes belong in `docs/archive/` and should not be referenced from primary docs.

## Docs Map

### Operators

- Deploy: `docs/deployment.md`
- CLI workflows: `docs/lesser-cli.md`
- CLI auth (device flow): `docs/cli/auth.md`
- Configure: `docs/configuration.md`
- Operate: `docs/monitoring.md`, `docs/security.md`, `docs/backup-recovery.md`, `docs/operations/runbook.md`
- Release checklist: `docs/release-checklist.md`
- Troubleshoot: `docs/troubleshooting.md`
- Federation checks: `docs/federation.md`

### Developers

- Local dev: `docs/development.md`
- Testing: `docs/testing.md`
- CLI workflows: `docs/lesser-cli.md`
- CLI auth (device flow): `docs/cli/auth.md`
- Repo drift guardrails: `./lesser verify`
- Internal drift specs: `docs/specs/README.md`

### Client teams

- API usage patterns: `docs/api-reference.md`
- Contracts: `docs/contracts/README.md`
- Client app integration: `docs/guides/CLIENT_APP_GUIDE.md`
- Agent OAuth grant selection: `docs/device-code-agent-auth.md`

For long-lived local-agent access, prefer the wallet-backed agent lease contract over delegated refresh-token flows.
The current published REST, GraphQL, and GraphQL-coverage contracts all include that path.

### Architecture

- Overview: `docs/architecture.md`
- Deep dives: `docs/architecture/README.md`

## What is Lesser?

Lesser is a serverless ActivityPub implementation that provides Mastodon-compatible federated social media at a fraction of the traditional cost. Built on AWS Lambda and DynamoDB, it scales automatically and charges only for actual usage.

## Key Concepts

### Stage vs environment

- **Stage**: AWS deployment tier (`dev|staging|live`) used in resource names and domains.
- **Environment**: runtime mode (`development|staging|production`) used for behavior toggles.

### Receipts (local state)

After `./lesser up`:

- Sensitive bootstrap material: `~/.lesser/<app>/<base-domain>/bootstrap.json` (0600)
- Non-secret deployment receipt: `~/.lesser/<app>/<base-domain>/state.json`

### Serverless Architecture
Lesser runs entirely on AWS managed services, eliminating server management and reducing costs by up to 90% compared to traditional hosting.

### ActivityPub Federation
Full compatibility with the ActivityPub protocol allows Lesser instances to communicate with Mastodon, Pleroma, and other federated platforms.

### Cost-Aware Design
Every component is optimized for cost, with built-in tracking, budgets, and automatic optimization strategies.

### Multi-Tenant Support
A single Lesser deployment can host multiple independent instances with complete data isolation.

## For Developers

- [Development Setup](development.md) - Local development environment
- [Testing Guide](testing.md) - Running and writing tests
- [Contributing](../CONTRIBUTING.md) - How to contribute

## For Operators

- [Monitoring Guide](monitoring.md) - CloudWatch dashboards and alerts
- [Security Guide](security.md) - Security best practices
- [Backup & Recovery](backup-recovery.md) - Data protection strategies
