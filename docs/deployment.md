# Deployment (`lesser up`)

<!-- AI Training: Operator deployment workflow for Lesser -->

Deployments are managed with AWS CDK (CloudFormation) under `infra/cdk/`, but the operator interface is the `lesser`
CLI.

Current state:

- `lesser up` has two deploy modes:
  - source mode: build from the repo checkout and deploy through the legacy CDK path
  - release mode: verify a published `--release-dir`, stage the release assets into the Lesser state dir, and deploy
    from the release-published CloudFormation assembly without a repo checkout
- Releases now publish immutable deploy assets for all three release-generic parts of the deploy:
  `lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json`, `lesser-auth-ui.tar.gz`,
  `lesser-deploy-assembly.tar.gz`, `lesser-deploy-assembly.json`, `lesser-release.json`, and `checksums.txt`
- Regardless of origin, `lesser up` stages the Lambda zip set into
  `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`.
- The published contract for that release path lives in `docs/contracts/release-driven-deploy-contract.md`.

## Prerequisites

- A public Route53 hosted zone that exactly matches your `base-domain` (for example: `example.com`)
- An AWS profile with a default region configured (the CLI derives region from the profile)

Release mode (`--release-dir`) requires:

- a built `lesser` CLI binary
- AWS credentials/config for the selected profile or ambient environment
- a release directory containing the published release assets

Source mode additionally requires:

- AWS CLI configured (and logged in for your chosen profile)
- AWS CDK v2 installed and on `PATH` (`npm install -g aws-cdk`)
- Go 1.25+
- `pnpm` installed

## What `lesser up` does

At a high level, `./lesser up`:

- Builds Lambda zip artifacts locally, or installs them from `--release-dir` after verification
- Stages the deployable Lambda zip set into `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`
- In release mode, stages the auth UI bundle into `~/.lesser/<app>/<base-domain>/deploy/auth-ui/`
- In release mode, stages the deploy assembly into `~/.lesser/<app>/<base-domain>/deploy/deploy-assembly/`
- In source mode, ensures CDK bootstrap exists for the target account/region and deploys through the repo-local CDK app
- In release mode, deploys the shared and stage stacks from the verified release assembly with CloudFormation
- Uploads the auth UI payload and preserves CloudFront invalidation behavior
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

Deploy from published release assets:

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

Deploy workspace:

- Staged Lambda asset root: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`
- Lambda asset provenance sidecar: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/metadata.json`
- Staged auth UI bundle: `~/.lesser/<app>/<base-domain>/deploy/auth-ui/`
- Staged deploy assembly: `~/.lesser/<app>/<base-domain>/deploy/deploy-assembly/`

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

- Source mode remains available and is still the default when `--release-dir` is absent.
- Release mode is source-free, but deploy-time responsibilities such as hosted-zone resolution, CloudFormation
  execution against live stacks, auth UI upload, invalidation, and receipt/bootstrap writes remain instance-specific.

If you changed Lambda code and want to force refresh zip artifacts:

```bash
./lesser up --app <app> --base-domain <base-domain> --aws-profile <profile> --rebuild-lambdas
```

`--rebuild-lambdas` is the explicit source-build override and takes precedence over `--release-dir`.

## Managed runner artifact mode

For a managed consumer or CI runner, the minimal artifact-driven deploy inputs are:

- a built `lesser` CLI binary
- a staged `--release-dir` containing:
  - `checksums.txt`
  - `lesser-release.json`
  - `lesser-lambda-bundle.tar.gz`
  - `lesser-lambda-bundle.json`
  - `lesser-auth-ui.tar.gz`
  - `lesser-deploy-assembly.tar.gz`
  - `lesser-deploy-assembly.json`
- AWS credentials plus the normal instance inputs (`--app`, `--base-domain`, optional `--provisioning-input`, and bootstrap/output flags as needed)

When `--release-dir` is used, the runner should treat the local deploy workspace under `~/.lesser/<app>/<base-domain>/deploy/`
as the canonical execution workspace for:

- `lambda-assets/`
- `auth-ui/`
- `deploy-assembly/`

The repo-root `bin/` directory and repo-local `infra/cdk/` tree are not part of the artifact-mode execution contract.

The current release trust signal for managed consumers is the artifact-driven deploy certification gate:

- CI and release publication both run `bash scripts/verify_artifact_deploy.sh`
- tagged releases also run that certification against the freshly built `dist/release/` asset set before publication

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
