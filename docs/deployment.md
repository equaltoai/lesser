# Deployment (`lesser up`)

<!-- AI Training: Operator deployment workflow for Lesser -->

Deployments are managed with AWS CDK (CloudFormation) under `infra/cdk/`, but the operator interface is the `lesser`
CLI.

Current state:

- `lesser up` has two deploy modes:
  - source mode: build from the repo checkout and deploy through the legacy CDK path
  - release mode: verify a published `--release-dir`, stage the release assets into the Lesser state dir, and deploy
    through the repo-local CDK app using the real app slug plus the verified release-built Lambda/auth UI assets
- Releases now publish immutable deploy assets for all three release-generic parts of the deploy:
  `lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json`, `lesser-auth-ui.tar.gz`,
  `lesser-deploy-assembly.tar.gz`, `lesser-deploy-assembly.json`, `lesser-release.json`, and `checksums.txt`
- Regardless of origin, `lesser up` stages the Lambda zip set into
  `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`.
- The published contract for that release path lives in `docs/contracts/release-driven-deploy-contract.md`.

## Theory Cloud framework baseline

The current deployable baseline consumes the latest released Theory Cloud framework line that was evaluated on
2026-05-16:

- AppTheory `v1.6.0` in the Go runtime and CDK app.
- TableTheory `v1.8.3` in the Go runtime.
- FaceTheory `v3.1.2` as the recommended client-app dependency in `docs/guides/CLIENT_APP_GUIDE.md`.

Deploy implications:

- There is no new `lesser up` flag for these framework updates and the published release asset shape is unchanged.
- Lambda-optimized TableTheory clients now keep TableTheory's Lambda timeout safety buffer when a request/event context
  has a deadline. This is runtime hardening only; it does not change DynamoDB PK/SK/GSI schema.
- AppTheory strict route registration has been adopted only where route parity was proven (`webfinger` and `objects`).
  It is a fail-fast guard against route drift, not a URL or response-shape change.
- AppTheory CDK `AppTheoryFunction` is adopted only for triggerless inventory Lambdas where synth parity was proven and
  pre-existing Lambda logical IDs are preserved. Stream/SQS/scheduled Lambdas and the client SSR host remain on native CDK
  constructs until their downstream permissions/event-source behavior can be proven separately.

Release and rollback considerations:

- Run the normal hard gate before publishing or deploying a framework-update release:
  `go build -o lesser ./cmd/lesser`, `./lesser build lambdas`, and `./lesser verify ci`.
- For CDK-adjacent releases, review a representative `cdk synth`/CloudFormation diff before stage rollout. Do not set
  timeouts on CDK commands.
- Deploy through the normal `dev` → optional `staging` → `live` path with soak evidence at each stage. Framework-only
  work does not make stage soak optional.
- Roll back by deploying the previous release/commit through `./lesser up`. No schema migration is part of this framework
  baseline, and prior Lambda versions must remain available as rollback targets.

## Prerequisites

- A public Route53 hosted zone that exactly matches your `base-domain` (for example: `example.com`)
- An AWS profile with a default region configured (the CLI derives region from the profile)

Release mode (`--release-dir`) requires:

- a built `lesser` CLI binary
- AWS credentials/config for the selected profile or ambient environment
- a release directory containing the published release assets
- a release-matched Lesser repo checkout containing:
  - `infra/cdk/cdk.json`
  - `infra/cdk/inventory/lambdas.go`
- AWS CLI configured (and logged in for your chosen profile)
- AWS CDK v2 installed and on `PATH` (`npm install -g aws-cdk`)
- Go 1.25+

Source mode additionally requires:

- `pnpm` installed
- repo-local auth UI source at `auth-ui/package.json`

## What `lesser up` does

At a high level, `./lesser up`:

- Builds Lambda zip artifacts locally, or installs them from `--release-dir` after verification
- Stages the deployable Lambda zip set into `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`
- In release mode, stages the auth UI bundle into `~/.lesser/<app>/<base-domain>/deploy/auth-ui/`
- In release mode, stages the deploy assembly into `~/.lesser/<app>/<base-domain>/deploy/deploy-assembly/`
- Ensures CDK bootstrap exists for the target account/region and deploys through the repo-local CDK app
- In release mode, points CDK at the verified staged Lambda assets so the stack update reuses the real app slug and the
  live stack’s logical ID contract
- The staged deploy assembly remains published and verified, but it is not the default execution path for live updates
- Uploads the auth UI payload and preserves CloudFront invalidation behavior
- Writes local receipts under `~/.lesser/<app>/<base-domain>/`

## Soul / `lesser-body` integration at deploy time

Soul-related wiring is a **deployment concern** in Lesser.

- The current deploy-time switch is `bodyEnabled`.
- The legacy name `soulEnabled` is still accepted by the CDK stack for backward compatibility.
- In current Lesser code, both names mean “wire the Lesser domain to the imported `lesser-body` MCP Lambda.”

When that wiring is enabled, the stage stack fronts these routes on the Lesser domain:

- `GET/POST/DELETE /mcp`
- `GET/POST/DELETE /mcp/{actor}`
- `GET /.well-known/mcp.json`
- `GET /.well-known/oauth-protected-resource/mcp/{actor}`

Those routes are served by `lesser-body`, not by Lesser’s core API Lambdas. Lesser still owns the rest of the
ActivityPub, REST, GraphQL, and `/.well-known/lesser-soul-agent` surfaces.

Use `docs/soul.md` for the canonical repo-boundary explanation.

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
- Release mode is release-asset-driven, not fully source-free.
- The safe default release path still requires a release-matched Lesser checkout plus `aws`, `cdk`, and `go`, because
  CDK must synthesize with the real app slug before updating existing stacks.
- Deploy-time responsibilities such as hosted-zone resolution, CDK/CloudFormation execution against live stacks, auth UI
  upload, invalidation, and receipt/bootstrap writes remain instance-specific.

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
- a release-matched Lesser repo checkout containing:
  - `infra/cdk/cdk.json`
  - `infra/cdk/inventory/lambdas.go`
- `aws`, `cdk`, and `go` available on `PATH`
- target-account AWS credentials plus the normal instance inputs (`--app`, `--base-domain`, optional
  `--provisioning-input`, and bootstrap/output flags as needed)

When `--release-dir` is used, the runner should treat the local deploy workspace under `~/.lesser/<app>/<base-domain>/deploy/`
as the canonical execution workspace for:

- `lambda-assets/`
- `auth-ui/`
- `deploy-assembly/`

The repo-root `bin/` directory is not part of the artifact-mode execution contract.
The repo-local `infra/cdk/` tree is still part of the current safe runner contract because CDK synthesizes the shared and
stage stacks with the real app slug at deploy time.

### Managed runner end-to-end (lesser-host)

Current safe lesser-host/update provisioner flow:

1. Materialize the Lesser CLI binary plus the published release directory for the target Lesser version.
2. Materialize a Lesser repo checkout at the matching release/tag or git SHA.
3. Obtain target-account AWS credentials for the managed instance account before invoking `lesser up`.
4. Write a managed `--provisioning-input` JSON payload with the app slug, stage, admin wallet, lesser-host URLs,
   optional `api_cors_allowed_origins`, and any managed feature flags. For first provisioning, `consent_message` must be
   the exact structured `lesser.init_admin_consent.v1` JSON signed by the admin wallet and `consent_signature` must be
   the EIP-191 signature over those exact bytes.
5. Run `./lesser up --release-dir <dir> --base-domain <domain> --provisioning-input <json> [--stage <stage>]`.
6. On first provisioning only, run `./lesser init-admin --base-domain <domain> --provisioning-input <json>` after the
   deploy succeeds.
7. On later updates, rerun only `./lesser up --release-dir ...`; do not repeat `init-admin` for an already activated
   instance.

Example first-time managed provisioning:

```bash
AWS_PROFILE=Sim ./lesser up \
  --base-domain simulacrum.greater.website \
  --release-dir /opt/lesser/releases/vX.Y.Z \
  --provisioning-input /tmp/simulacrum-provisioning.json \
  --stage dev

AWS_PROFILE=Sim ./lesser init-admin \
  --base-domain simulacrum.greater.website \
  --provisioning-input /tmp/simulacrum-provisioning.json
```

Example update:

```bash
AWS_PROFILE=Sim ./lesser up \
  --base-domain simulacrum.greater.website \
  --release-dir /opt/lesser/releases/vX.Y.Z \
  --provisioning-input /tmp/simulacrum-provisioning.json \
  --stage dev
```

Account boundary:

- `lesser-host` may orchestrate from its own account, but the actual `lesser up` stack update must run with credentials
  for the managed instance account.
- Do not set `LESSER_USE_LEGACY_RELEASE_ASSEMBLY=1` in the managed runner. That escape hatch only exists for debugging.
- When managed environments need soul/MCP wiring, treat that as part of the deployment input contract rather than as a
  manual post-deploy patch.

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
