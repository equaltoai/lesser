# Deployment (`lesser up`)

<!-- AI Training: Operator deployment workflow for Lesser -->

Deployments are managed with AWS CDK (CloudFormation) under `infra/cdk/`, but the operator interface is the `lesser`
CLI.

Current state:

- `lesser up` still deploys CDK and builds `auth-ui/` from the repo checkout.
- Lambda zip artifacts can now come from either local source builds or a verified published release directory.
- Releases now publish immutable Lambda deploy assets (`lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json`, and
  `lesser-release.json` deploy-artifact metadata) for release-driven deploys and managed consumers.
- The published contract for that release path lives in `docs/contracts/release-driven-deploy-contract.md`.

## Prerequisites

- AWS CLI configured (and logged in for your chosen profile)
- AWS CDK v2 installed and on `PATH` (`npm install -g aws-cdk`)
- Go 1.25+
- `pnpm` installed (for building `auth-ui/` during deploy)
- A public Route53 hosted zone that exactly matches your `base-domain` (for example: `example.com`)
- An AWS profile with a default region configured (the CLI derives region from the profile)

## What `lesser up` does

At a high level, `./lesser up`:

- Builds Lambda zip artifacts locally, or installs them from `--release-dir` after verification
- Ensures CDK bootstrap exists for the target account/region
- Deploys the shared stack (`<app>-shared`)
- Deploys stage stacks (`<app>-dev`, `<app>-live`, optional `<app>-staging`)
- Builds and uploads `auth-ui/`
- Writes local receipts under `~/.lesser/<app>/<base-domain>/`

## Deploy

Build the CLI:

```bash
go build -o lesser ./cmd/lesser
```

Deploy **dev + live** (and optionally **staging**):

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

Deploy with published Lambda assets:

```bash
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --release-dir /tmp/lesser-release \
  --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

Bootstrap wallet key material:

- `lesser up` prints a 24-word Ethereum mnemonic **once** when it is generated.
- On first deploy, `--out <path>` is required so you don’t lose the mnemonic (the file is created with `0600` permissions).
- Recommended: `~/.lesser/<app>/<base-domain>/bootstrap.json`

Local receipt (non-secret):

- `~/.lesser/<app>/<base-domain>/state.json`

## What gets deployed

Stacks:

- Shared stack (once per app/account/region): `<app>-shared`
- Stage stacks: `<app>-dev`, `<app>-live` (and `<app>-staging` if enabled)

Stage domains:

- dev: `dev.<base-domain>`
- staging (optional): `staging.<base-domain>`
- live: `<base-domain>`

Bootstrap state:

- Each stage’s DynamoDB table gets an `InstanceState` record set to locked, with the bootstrap wallet address recorded.

## Updating an existing deployment

✅ CORRECT: rerun `./lesser up` with the same `--app` + `--base-domain` to apply changes.

Important scope note:

- Prebuilt Lambda release assets are now supported through `--release-dir`, but source builds remain the default when
  that flag is absent.
- Even in artifact mode, deploy-time responsibilities such as CDK execution, auth UI upload, hosted-zone resolution,
  and receipt/bootstrap writes remain instance-specific.

If you changed Lambda code and want to force refresh zip artifacts:

```bash
./lesser up --app <app> --base-domain <base-domain> --aws-profile <profile> --rebuild-lambdas
```

`--rebuild-lambdas` is the explicit source-build override and takes precedence over `--release-dir`.

## Verify “locked but reachable”

```bash
# Lock state + bootstrap actor descriptor
curl -s https://dev.example.com/setup/status | jq .

# Empty timeline while locked
curl -s https://dev.example.com/api/v1/timelines/public | jq .
```

## Activation

Activation is done via the setup wizard UI at `https://<stage-domain>/auth/setup`, which talks to backend endpoints
under `https://<stage-domain>/setup/*`.

### Build Failures
```bash
# Rebuild Lambda zip artifacts
./lesser build lambdas --rebuild

# Or rebuild the full deployment payload
./lesser build
```

### Certificate Issues
- Ensure certificate is in us-east-1 for CloudFront
- Verify DNS validation records are in place

### Domain Not Working
- Check Route53 hosted zone
- Verify CloudFront distribution status
- Allow up to 15 minutes for propagation

## Destroying a deployment (manual, use caution)

There is currently no `lesser down` command.

⚠️ WARNING: destroying stacks deletes infrastructure. DynamoDB deletion protection and PITR may prevent destroy (by
design). Review `docs/backup-recovery.md` before destructive actions.

From `infra/cdk/`:

```bash
AWS_PROFILE=<profile> cdk destroy <app>-dev <app>-live <app>-shared --force
```

## Next Steps

- [Configuration Reference](configuration.md) - Customize your instance
- [Monitoring Guide](monitoring.md) - Set up comprehensive monitoring
- [Federation Guide](federation.md) - Connect to the Fediverse

## References

- `infra/cdk/README.md` (CDK details)
- `docs/architecture/auth/OWNER_BOOTSTRAP_REQUIREMENTS.md` (bootstrap/lock semantics)
