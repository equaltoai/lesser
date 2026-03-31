# Lesser CDK Infrastructure

This directory contains the AWS CDK app (`infra/cdk/main.go`) used to synthesize Lesser infrastructure.

Operator deployments are managed by the `lesser` CLI:

- source mode shells out to `cdk deploy`
- release mode consumes the release-published deploy assembly and does not require a repo-local CDK checkout at deploy time

## Operator Deployments

Prefer `lesser up` from the repo root:

- Builds Lambda zip artifacts in `bin/`
- In source mode, runs `cdk bootstrap` (if needed) and deploys the shared stack, then each stage stack
- In release mode, deploys the shared and stage stacks from the release-published CloudFormation assembly
- Writes a local receipt to `~/.lesser/<app>/<base-domain>/state.json`

See `docs/deployment.md` for the operator workflow.

## Stack Layout (Deterministic)

- Shared stack: `<app>-shared`
- Stage stacks: `<app>-dev`, `<app>-live` (and optional `<app>-staging`)

Resource naming follows the same patterns:

- Non-global names: `<app>-<stage>-<resource>` and `<app>-shared-<resource>`
- Global-unique names (for example S3 buckets): include account ID + region

## CDK Context Inputs

The CDK app reads the following context values:

- `app` (default: `lesser`)
- `baseDomain` (required for stage stacks): base domain with an existing public hosted zone (for example `example.com`)
- `hostedZoneId` (recommended for stage stacks): Route53 hosted zone ID for `baseDomain` (otherwise CDK will do a lookup)
- `lambdaAssetRoot` (optional): root directory that contains `bin/*.zip`; defaults to the repo root when omitted
- `stage` (optional): `shared|dev|staging|live|all` (empty means “all stages”)
- `withStaging` (optional): `true` to include staging when deploying all stages

The app also expects to run under the CDK CLI so `CDK_DEFAULT_ACCOUNT` and `CDK_DEFAULT_REGION` are set.

## Lambda Assets

Stage stacks reference zip artifacts in `<lambdaAssetRoot>/bin/*.zip`.

- `lesser up` builds or verifies these automatically, stages them under
  `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`, and passes that path as `lambdaAssetRoot`.
- If you run `cdk` directly without `lesser up`, ensure the default repo-root `bin/*.zip` layout exists first (for example:
  `./lesser build lambdas`), or pass your own `--context lambdaAssetRoot=<path>`.
- Releases still publish a Lambda bundle archive, but CDK consumes the staged extracted files, not the archive directly.

### Artifact-root propagation chain

The `lambdaAssetRoot` propagation chain matters for direct CDK consumers and the legacy source-mode deploy path:

1. `lesser up --release-dir ...` prepares `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`
2. `cmd/lesser/up.go` passes that staged path as `cdkDeployRequest.LambdaAssetRoot`
3. `cmd/lesser/cdk.go` emits `--context lambdaAssetRoot=<path>` for both the shared-stack and stage-stack deploy calls
4. `infra/cdk/main.go` copies that context into each stage stack's config map
5. `infra/cdk/stacks/lesser_api_stack.go` threads the configured `lambdaAssetRoot` into `constructs.LambdaFunctionsProps`
6. `infra/cdk/constructs/lambda_functions.go` binds every inventory Lambda asset from `<lambdaAssetRoot>/bin/<name>.zip`

If any boundary in that chain drops `lambdaAssetRoot`, source-mode deploys can silently fall back to repo-local `bin/*.zip`.
Release-mode deploys do not use this chain; they consume the release-published deploy assembly instead.

## CDK Commands (Infra Contributors)

Run these from `infra/cdk/`.

Bootstrap (first time in an account/region):

```bash
AWS_PROFILE=Penny cdk bootstrap aws://<account-id>/<region>
```

Deploy only the shared stack:

```bash
AWS_PROFILE=Penny cdk deploy my-lesser-shared \
  --require-approval never \
  --context app=my-lesser \
  --context stage=shared
```

Deploy a single stage stack:

```bash
AWS_PROFILE=Penny cdk deploy my-lesser-dev \
  --require-approval never \
  --context app=my-lesser \
  --context baseDomain=example.com \
  --context hostedZoneId=Z1234567890 \
  --context stage=dev
```

Deploy dev + live (and optionally staging) in one go:

```bash
AWS_PROFILE=Penny cdk deploy --all \
  --require-approval never \
  --context app=my-lesser \
  --context baseDomain=example.com \
  --context hostedZoneId=Z1234567890

AWS_PROFILE=Penny cdk deploy --all \
  --require-approval never \
  --context app=my-lesser \
  --context baseDomain=example.com \
  --context hostedZoneId=Z1234567890 \
  --context withStaging=true
```

## Cross-Stack References

The shared stack writes a small “registry” of ARNs/IDs to SSM Parameter Store under `/<app>/shared/...`. Stage stacks read those values at deploy time (no CloudFormation exports/imports).

## Domains & Certificates

Stage domain is derived from `baseDomain`:

- `dev`: `dev.<baseDomain>`
- `staging`: `staging.<baseDomain>`
- `live`: `<baseDomain>`

Each stage stack creates regional ACM certificates validated via Route53:

- `<stage-domain>`
- `*.<stage-domain>`

## Troubleshooting

- Hosted zone errors: ensure a public hosted zone exists for `baseDomain`, or pass `--context hostedZoneId=...` (the CLI resolves this automatically).
- Missing `bin/*.zip`: run `./lesser build lambdas` or re-run `lesser up --rebuild-lambdas`.
