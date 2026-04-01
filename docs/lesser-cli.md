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

## CLI client auth (device flow)

See `docs/cli/auth.md` for the headless OAuth device flow used by the CLI (wallet login + consent via the web UI).

Common commands:

```bash
./lesser auth login --base-url https://dev.example.com
./lesser auth status --base-url https://dev.example.com
./lesser auth whoami --base-url https://dev.example.com
./lesser auth logout --base-url https://dev.example.com

./lesser api request --base-url https://dev.example.com --method GET --path /api/v1/accounts/verify_credentials
```

## Deploy workflows

### Deploy dev + live (and optional staging)

Current behavior:

- Default mode is still the legacy source path and builds deploy artifacts from the repo checkout.
- `--release-dir <path>` is the release-asset-driven path. It verifies the published release assets, stages the Lambda
  bundle, auth UI bundle, and deploy assembly into the Lesser state dir, and then deploys through the repo-local CDK
  app using the real app slug plus the staged release-built assets.
- Current release mode still requires a release-matched repo checkout plus `cdk` and `go`, but it does not require
  rebuilding Lambdas or auth UI from source and it does not require `pnpm`.
- `--rebuild-lambdas` remains the explicit source-build override path and wins even if `--release-dir` is also set.

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

Deploy from published release assets instead of rebuilding from source:

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --release-dir /tmp/lesser-release \
  --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

Key outputs:

- Sensitive bootstrap material: `~/.lesser/<app>/<base-domain>/bootstrap.json` (0600)
- Non-secret receipt: `~/.lesser/<app>/<base-domain>/state.json`
- Staged Lambda asset root: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`
- Lambda asset provenance: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/metadata.json`
- Staged auth UI bundle: `~/.lesser/<app>/<base-domain>/deploy/auth-ui/`
- Staged deploy assembly: `~/.lesser/<app>/<base-domain>/deploy/deploy-assembly/`

### Install a FaceTheory client release

Run this from the FaceTheory app repo after `lesser up` has already provisioned the shared `/l/` host resources:

```bash
./lesser client install \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --config ./facetheory.lesser.json
```

Notes:

- `lesser client install` uploads SSR server artifacts to the stage artifact bucket, uploads public assets to `/l/_assets/`,
  flips the active install manifest, and invalidates CloudFront.
- Use `--stage dev|staging|live|both|all` to target specific stages.
- Use `--skip-build` if the FaceTheory app is already built.
- `lesser client deploy` is retired for SSR-first `/l/` hosting.

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
./lesser test coverage --scope pkg --include-testing
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
