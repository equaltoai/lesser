# Lesser CLI (`./lesser`)

<!-- AI Training: Canonical CLI workflows for the Lesser repository -->

The `lesser` CLI is the single user-facing interface for this repo (operators, developers, and CI guardrails).

✅ CORRECT: document and automate workflows via `./lesser …`.

❌ INCORRECT: document repo workflows as ad-hoc shell scripts or `make` targets (except building `lesser` itself).

## Build the CLI

The CLI is built from this repo:

```bash
go build -o lesser ./cmd/lesser
```

Run it from the repo root:

```bash
./lesser
```

## Deploy workflows

### Deploy dev + live (and optional staging)

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

Key outputs:

- Sensitive bootstrap material: `~/.lesser/<app>/<base-domain>/bootstrap.json` (0600)
- Non-secret receipt: `~/.lesser/<app>/<base-domain>/state.json`

### Deploy a standalone client UI

If you build a Greater-based UI to `./dist`, deploy it into the per-stage client buckets:

```bash
./lesser client deploy \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --dist ./dist
```

## Development workflows

### Initialize `.env` and run the API locally

```bash
./lesser dev init
./lesser dev
```

Optional: local DynamoDB:

```bash
./lesser dev dynamodb
```

### Seed + validate (destructive)

⚠️ WARNING: clears the target DynamoDB table. Only use on disposable environments.

```bash
./lesser dev seed-and-validate \
  --aws-profile <profile> \
  --dynamodb-table <table> \
  --base-url https://dev.example.com
```

## Build workflows

### Rebuild everything used by deploy

```bash
./lesser build
```

### Build Lambda zip artifacts

```bash
./lesser build lambdas
./lesser build lambdas --rebuild
```

### Build one Lambda artifact

```bash
./lesser build lambda api
```

## Generation workflows (contracts + inventories)

```bash
./lesser generate openapi
./lesser generate graphql-coverage
./lesser generate inventory
./lesser generate schema
```

Notes:

- OpenAPI output: `docs/contracts/openapi.yaml`
- GraphQL aggregated schema output: `docs/contracts/graphql-schema.graphql`
- Inventory output: `docs/specs/01-lambda-inventory-matrix.md`

## Verification workflows (repo guardrails)

Run all guardrails:

```bash
./lesser verify
```

Run a specific verifier:

```bash
./lesser verify docs
./lesser verify schema
./lesser verify openapi --strict
./lesser verify graphql-coverage --strict
./lesser verify inventory
./lesser verify lambda-set
```

## Testing workflows

```bash
./lesser test unit
./lesser test
./lesser test coverage
./lesser test coverage --scope pkg
./lesser test race
./lesser test integration
```

## Quality and security workflows

```bash
./lesser fmt
./lesser lint
./lesser lint --fix

./lesser sec-scan
./lesser vuln-check
```

## Ops workflows (CloudWatch helpers)

Tail logs for a Lambda:

```bash
./lesser logs --app <app> --function api --env dev --aws-profile <profile>
```

Scan recent errors:

```bash
./lesser errors --app <app> --env dev --function api --aws-profile <profile>
```

Open the CloudWatch dashboard in your browser:

```bash
./lesser dashboard --app <app> --env live --region us-east-1
```

## Smoke workflows (against a deployment)

```bash
./lesser smoke core --base-url=https://dev.example.com --token="Bearer <token>"
./lesser smoke federation --base-url=https://dev.example.com --username=alice --object-id=<id>
```

Or run them through `verify`:

```bash
./lesser verify --smoke --smoke-base-url=https://dev.example.com --smoke-token="Bearer <token>"
```

## Common mistakes

### “unknown command” / “missing generate command”

Run `./lesser` with no args to see the supported command set. Many subcommands expect a positional argument (for
example, `./lesser test unit`).

### “no .env file found”

Run:

```bash
./lesser dev init
```

### Tooling missing (AWS CLI / CDK / pnpm)

`./lesser up` shells out to external tools. See `docs/deployment.md` prerequisites and install the required tools for
your environment.
