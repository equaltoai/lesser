# Lesser Release-Driven Deploy Contract

This document describes the current release-driven deploy contract for Lesser.

Its job is to keep the deploy boundary stable:

- build release-generic assets once per Lesser release
- keep install-specific values outside those assets
- let `lesser up --release-dir ...` consume the published release while preserving the live stack’s real-app logical IDs

## Status

- Default deploy path: source-based `./lesser up`
- Release-asset-driven deploy path: `./lesser up --release-dir <dir>`
- Explicit source override: `./lesser up --release-dir <dir> --rebuild-lambdas`
- Release-published deploy assets now include the Lambda bundle, auth UI bundle, and deploy assembly
- Install-specific values remain deploy-time inputs; they are not baked into the published release bytes
- Current safe release mode still requires a release-matched Lesser checkout plus `aws`, `cdk`, and `go`

## Current Deploy Inventory

The table below separates release-generic work from deploy-time execution.

| Step | Current owner | Current source of truth / input | Deterministic at release time? | Must still happen at deploy time? |
| --- | --- | --- | --- | --- |
| Materialize a Lesser CLI binary, release payload, and release-matched repo checkout | Operator or managed runner | Downloaded release assets plus matching Lesser checkout | Yes for release bytes, no for local runner materialization | Yes |
| Provide AWS credentials and install-specific inputs | Operator or managed runner | Ambient AWS auth, CLI flags, `--provisioning-input` JSON | No | Yes |
| Resolve `app`, `base-domain`, hosted zone, and stage selection | Lesser CLI | CLI flags plus Route53 lookup | No | Yes |
| Verify published release assets and stage them into the Lesser state dir | Lesser CLI | `checksums.txt`, `lesser-release.json`, bundle manifests, bundle archives | Yes | Yes |
| Synthesize and deploy the shared stack with the real app slug | Lesser CLI | Repo-local CDK app plus deploy-time inputs plus staged release Lambda assets | No | Yes |
| Synthesize and deploy stage stacks with the real app slug | Lesser CLI | Repo-local CDK app plus deploy-time inputs plus staged release Lambda assets | No | Yes |
| Publish the prebuilt auth UI bundle and invalidate CloudFront | Lesser CLI | `lesser-auth-ui.tar.gz` plus live stack outputs | Yes for bundle bytes, no for destination state | Yes |
| Stage and verify the published deploy assembly | Lesser CLI | `lesser-deploy-assembly.tar.gz`, `lesser-deploy-assembly.json` | Yes | Yes |
| Bootstrap instance state, sync feature config, and write the local receipt | Lesser CLI | DynamoDB tables, deploy outputs, local filesystem | No | Yes |
| Build Lambdas or auth UI from source | Lesser CLI | Repo-local checkout plus toolchains | Yes | Only on the legacy source path |

### Lesser-owned deploy-time work

These steps remain owned by `lesser up` even in release-asset-driven mode:

- release verification
- CDK/CloudFormation execution against live stacks
- auth UI publication and CloudFront invalidation
- receipt/bootstrap writes and stage bootstrap state changes

### Release-time deterministic work

These now happen once per Lesser release instead of once per deploy:

- building the `lesser` CLI binaries
- compiling the canonical Lambda zip set
- building the `auth-ui` bundle
- synthesizing the shared and stage deploy templates
- collecting deploy-time file assets into the deploy assembly
- writing release manifests, descriptors, and `checksums.txt`

The published deploy assembly remains useful for certification and future thin-executor work, but it is not the default
live-update execution path while existing stacks still depend on real-app CDK logical IDs.

## Published Artifact Taxonomy

These artifact category names are part of the contract.

| Category ID | Canonical assets | Purpose | Primary consumers | Status |
| --- | --- | --- | --- | --- |
| `operator_cli` | `lesser-<os>-<arch>` | Human- or runner-invoked CLI executable | Operators, CI, managed runners | Published |
| `release_metadata` | `checksums.txt`, `lesser-release.json` | Release-level discovery and integrity metadata | Operators, CI, `lesser up --release-dir` | Published |
| `lambda_bundle` | `lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json` | Canonical `bin/*.zip` Lambda asset set plus manifest | `lesser up --release-dir`, certification, managed runners | Published |
| `auth_ui_bundle` | `lesser-auth-ui.tar.gz` | Prebuilt auth UI payload for deploy-time upload | `lesser up --release-dir`, managed runners | Published |
| `deploy_assembly` | `lesser-deploy-assembly.tar.gz`, `lesser-deploy-assembly.json` | Release-published shared/stage templates plus deploy-time file assets and future thin-executor contract | Certification, migration planning, managed runners | Published |

### Taxonomy rules

- CLI binaries execute deploy workflows, but they are not themselves the deploy payload.
- Release artifacts must stay byte-identical for every consumer of the same Lesser release.
- Deploy receipts, bootstrap key material, provisioning input JSON, and other local files are runtime inputs or outputs, not release artifacts.
- Install-specific values must be supplied separately from the published release artifacts.
- The top-level release manifest remains the discovery root:
  - `artifacts.deploy_artifacts.lambda_bundle`
  - `artifacts.deploy_artifacts.auth_ui_bundle`
  - `artifacts.deploy_artifacts.deploy_assembly`

## Lambda Bundle Contract

The Lambda deploy artifact category uses two canonical release assets:

- `lesser-lambda-bundle.tar.gz`
- `lesser-lambda-bundle.json`

The normative manifest schema is:

- `docs/contracts/lambda-bundle-manifest.schema.json`

The illustrative example is:

- `docs/contracts/examples/lesser-lambda-bundle.example.json`

### Manifest rules

- `kind` is always `lesser.lambda_bundle_manifest`.
- `schema_version` starts at `1`. Breaking shape changes increment `schema_version`.
- `bundle.path` is always `lesser-lambda-bundle.tar.gz`.
- `bundle.sha256` is the checksum of the published archive bytes.
- `files[]` enumerates the extracted deploy layout under `bin/`.
- Every `files[].path` is a relative `bin/<lambda>.zip` path, sorted lexicographically, with no duplicates.
- Every `files[].sha256` hashes the exact extracted zip file bytes.
- Every `files[].size_bytes` records the extracted zip size in bytes.
- `inventory_source.path` points at `infra/cdk/inventory/lambdas.go`, which is still the canonical source of truth for the Lambda inventory at release build time.

### Checksum rules

- `checksums.txt` remains the release-level checksum file for top-level published assets.
- `lesser-lambda-bundle.tar.gz` and `lesser-lambda-bundle.json` must both appear in `checksums.txt`.
- Consumers must reject the bundle if the archive checksum, manifest checksum, or any listed file checksum does not match.
- The manifest must enumerate the exact stage-stack Lambda zip set and exclude non-deploy extras such as `cloudfront-keygen.zip`.

## `--release-dir` Contract For `lesser up`

`--release-dir <path>` is the canonical artifact-driven input surface for Lesser deploys.

### Why `--release-dir` is canonical

- Operators naturally download or mirror whole release assets.
- Managed runners can materialize the same directory layout in ephemeral storage.
- The CLI can verify release metadata, top-level checksums, the Lambda bundle, the auth UI bundle, and the deploy assembly from one trust root.
- A bundle-only flag would fragment trust and create separate discovery paths for the same release.

### Required contents of `--release-dir`

Artifact mode requires these files in the release directory:

- `checksums.txt`
- `lesser-release.json`
- `lesser-lambda-bundle.tar.gz`
- `lesser-lambda-bundle.json`
- `lesser-auth-ui.tar.gz`
- `lesser-deploy-assembly.tar.gz`
- `lesser-deploy-assembly.json`

### Selection and verification behavior

When `--release-dir` is set, `lesser up` must:

1. Fail fast if any required release file is missing.
2. Verify `checksums.txt` for every published top-level deploy asset in the selected release directory.
3. Read `lesser-release.json` to confirm the release identity and discover the canonical deploy artifact references.
4. Validate and verify `lesser-lambda-bundle.json` plus `lesser-lambda-bundle.tar.gz`.
5. Validate and verify `lesser-deploy-assembly.json` plus `lesser-deploy-assembly.tar.gz`.
6. Extract and verify the Lambda bundle into `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/bin/*.zip`.
7. Extract the auth UI bundle into `~/.lesser/<app>/<base-domain>/deploy/auth-ui/`.
8. Extract and verify the deploy assembly into `~/.lesser/<app>/<base-domain>/deploy/deploy-assembly/`.
9. Resolve the release-matched repo-local CDK app and canonical lambda inventory inputs.
10. Deploy the shared and stage stacks through repo-local `cdk deploy` using the real app slug plus the verified staged
    Lambda asset root instead of rebuilding Lambda or auth UI artifacts from source.
11. Upload the prebuilt auth UI bundle to the stack-selected auth UI bucket and preserve CloudFront invalidation behavior.

### Override rules

- `--rebuild-lambdas` remains the explicit source-build override path.
- If both `--release-dir` and `--rebuild-lambdas` are set, source mode wins.
- Without `--release-dir`, `lesser up` continues to use the existing source-based path.
- Invalid or incomplete release assets are hard errors in artifact mode; the CLI must not silently fall back to an unrequested source build.

## Deploy Assembly Contract

The deploy assembly is the release-published infrastructure payload.

- outer descriptor: `lesser-deploy-assembly.json`
- payload archive: `lesser-deploy-assembly.tar.gz`

The normative descriptor schema is:

- `docs/contracts/deploy-assembly-descriptor.schema.json`

The illustrative example is:

- `docs/contracts/examples/lesser-deploy-assembly.example.json`

### Descriptor rules

- `kind` is always `lesser.deploy_assembly_descriptor`.
- `schema_version` starts at `1`.
- `assembly.path` is always `lesser-deploy-assembly.tar.gz`.
- `payload.kind` identifies the inner assembly type.
- `payload.entrypoint` points at the manifest inside the archive.
- `compatibility.release_manifest_path` stays anchored to `lesser-release.json`.
- `compatibility.deploy_artifacts_key` stays anchored to `deploy_assembly`.
- `instance_inputs.required` and `instance_inputs.optional` freeze the canonical install-time input taxonomy.

### Current payload shape

The current deploy assembly payload packages:

- the shared CloudFormation template
- the stage CloudFormation templates for `dev`, `staging`, and `live`
- a payload manifest that records template and asset checksums
- the deploy-time file assets referenced by those templates

The stage templates are release-generic and install-time values flow in through parameters rather than release-time rewrites.
They are still published and certified, but the default live-update path does not execute them directly unless
`LESSER_USE_LEGACY_RELEASE_ASSEMBLY=1` is set for debugging.

## Current Lesser-Host Runner Contract

Until the thin executor lands, the current safe lesser-host/update provisioner path is:

1. Materialize the published Lesser release assets and CLI binary for the target version.
2. Materialize a Lesser repo checkout at the matching release tag or git SHA.
3. Assume or inject AWS credentials for the managed instance account before invoking `lesser up`.
4. Write the managed `--provisioning-input` JSON with app identity, stage plan, lesser-host URLs, feature flags, and
   the instance key ARN.
5. Run `./lesser up --release-dir <dir> --base-domain <domain> --provisioning-input <json> [--stage <stage>]`.
6. On first provisioning only, run `./lesser init-admin --base-domain <domain> --provisioning-input <json>`.
7. On later updates, rerun only `./lesser up --release-dir ...`.

Rules:

- The managed runner must not set `LESSER_USE_LEGACY_RELEASE_ASSEMBLY=1` in normal operation.
- `lesser-host` may orchestrate from its own control-plane account, but the actual `lesser up` invocation must run with
  credentials for the managed instance account.
- The current safe path is release-asset-driven, but it is not yet “no checkout / no CDK / no go”.

## Canonical Install-Time Input Taxonomy

The release assembly descriptor freezes these input category names:

| Input category | What it covers | Current executor surface |
| --- | --- | --- |
| `app_identity` | app slug, stack prefix, state-dir namespace | `--app`, stack names, Lesser state dir |
| `aws_target` | AWS credentials, account, region, live stack environment | ambient AWS auth plus SDK account/region resolution |
| `base_domain` | root domain for the installation | `--base-domain` |
| `hosted_zone` | actual Route53 hosted zone binding | Route53 lookup and selected hosted zone ID |
| `stage_plan` | which stages are being deployed or updated | `--stage`, `--with-staging`, live stack history |
| `feature_config` | install-level behavior flags such as translation and tips | CLI flags and provisioning input JSON |
| `managed_service_urls` | managed control-plane URLs and instance-key ARN | CLI flags and provisioning input JSON |
| `provisioning_input` | managed bootstrap/config blob supplied with the deploy request | `--provisioning-input` |
| `bootstrap_io` | local output paths and receipt/bootstrap handling | `--out`, local Lesser state dir |
| `binding_secrets` | ARN-backed body/Ptah → Lesser binding receiver credential | `SOUL_BINDING_INTEGRATION_KEY_ARN` or provisioning input JSON |

### Separation rules

- Release artifacts may describe required input categories, but they must not embed per-install values.
- App slugs, domains, hosted-zone IDs, AWS account state, feature flags, and provisioning JSON stay outside the published release bytes.
- The same release directory must be reusable across different customers and domains without rewriting the artifact bytes.
- Operators and managed runners should map their local invocation surfaces into the same canonical categories above.

## CloudFormation Parameter Contract

Install-specific values that affect the release assembly are expressed as CloudFormation parameters in the synthesized templates.

### Shared template parameters

- `AppSlug`

### Stage template required parameters

- `AppSlug`
- `BaseDomain`
- `HostedZoneId`
- `ReleaseAssetBucketName`

### Stage template optional parameters

- `LesserHostUrl`
- `LesserHostAttestationsUrl`
- `LesserHostInstanceKeyArn`
- `SoulBindingIntegrationKeyArn`
- `TranslationEnabled`
- `TipEnabled`
- `TipChainId`
- `TipContractAddress`

### Mapping notes

- Shared-stack resources that are installation-specific but stable across stages are resolved through shared-stack outputs or dynamic SSM references at deploy time.
- The release assembly keeps CloudFormation templates generic; it does not rewrite template bytes per installation.
- `ReleaseAssetBucketName` binds the stage templates to the shared release-asset bucket created by the shared stack for that installation.
- `SoulBindingIntegrationKeyArn` is the ARN for the dedicated body/Ptah → Lesser receiving credential used by `/api/v1/souls/bindings`. Managed runners must pass the ARN, not a raw secret value, when deploying the body instance-plane/Ptah binding surface.

## Managed Runner And Operator Execution Contract

Artifact mode no longer rebuilds Lambda or auth UI artifacts from source, but the current safe path still requires a
release-matched checkout.

The minimal release-asset-driven deploy inputs are:

- a built `lesser` CLI binary from the selected release
- a release directory containing the seven required published release assets
- a release-matched Lesser checkout containing `infra/cdk/cdk.json` and `infra/cdk/inventory/lambdas.go`
- `aws`, `cdk`, and `go` on `PATH`
- AWS credentials plus the normal instance inputs (`--app`, `--base-domain`, optional `--provisioning-input`, stage
  selection, and bootstrap/output flags as needed)

When artifact mode runs successfully, the Lesser state dir becomes the canonical execution workspace:

- staged Lambda asset root: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`
- Lambda provenance sidecar: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/metadata.json`
- staged auth UI bundle: `~/.lesser/<app>/<base-domain>/deploy/auth-ui/`
- staged deploy assembly: `~/.lesser/<app>/<base-domain>/deploy/deploy-assembly/`
- stable instance receipt: `~/.lesser/<app>/<base-domain>/state.json`

This keeps the runner contract explicit:

- release assets are external deploy inputs, not mutable repo byproducts
- the Lesser state dir is the canonical execution workspace
- the same release directory can be reused across installations
- install-specific values arrive through executor inputs and CloudFormation parameters, not by mutating the published release artifacts

## Remaining Deploy-Time Responsibilities

Release-asset-driven consumption removes rebuilds, but it does not eliminate deploy-time work.

These responsibilities remain installation-specific:

| Responsibility | Why it remains deploy-time |
| --- | --- |
| AWS credential and account selection | Trust, billing, and target account state vary per deploy |
| Hosted-zone lookup and stage-domain routing | Domain ownership and DNS records are installation-specific |
| CDK/CloudFormation execution against live shared and stage stacks | Updates depend on target account, region, existing stack history, and the real app slug |
| Shared release bucket writes and template/asset publication | The destination bucket belongs to the target installation |
| Auth UI upload and CloudFront invalidation | The destination bucket and distribution come from live stack outputs |
| Bootstrap/admin lifecycle and feature-config sync | These depend on the current state of the installation |
| Local receipt and bootstrap output writes | Output paths are local execution concerns |

## Source Mode Fallback

Legacy source mode is still supported and remains the default when `--release-dir` is omitted.

That path still depends on:

- a repo checkout containing `infra/cdk/` and `auth-ui/`
- local toolchains on `PATH` (`aws`, `cdk`, `go`, `pnpm`)
- local Lambda builds and local auth UI builds during deploy

This is now a compatibility path, not the only deploy path.

## Verification And Certification Guardrails

The release-driven deploy contract is enforced by:

- contract schemas and examples:
  - `docs/contracts/lambda-bundle-manifest.schema.json`
  - `docs/contracts/deploy-assembly-descriptor.schema.json`
  - `docs/contracts/examples/lesser-lambda-bundle.example.json`
  - `docs/contracts/examples/lesser-deploy-assembly.example.json`
- release asset verification in `lesser verify artifact-deploy --release-dir ...`
- CI and release publication running `bash scripts/verify_artifact_deploy.sh`
- CI and release publication running `bash scripts/verify_auth_ui_csp.sh` as a fail-fast freshness check before release
  assets are built
- targeted stack and CLI tests that prove release-mode deploys avoid repo-local rebuilds and repo-local `cdk synth`

The certification goal is straightforward:

- build once per release
- verify once per release payload
- let every deploy reuse the same published release artifacts with only install-time inputs changing
