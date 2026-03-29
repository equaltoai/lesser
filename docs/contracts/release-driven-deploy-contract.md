# Lesser Release-Driven Deploy Contract

This document freezes the milestone-0 contract for release-driven Lesser deploys.

Its job is to make the current deploy path explicit before later milestones move work from deploy time into release
time. When this document says a category or rule is "canonical", later implementation milestones should preserve that
contract instead of reinventing new names or trust boundaries.

## Status

- Current default deploy path: source-based `./lesser up`
- First-phase Lambda release assets are now published as release artifacts
- `lesser up` now supports artifact consumption through `--release-dir`, while source builds remain the default when that flag is absent
- Explicit non-goal for milestone 0: claiming deploy execution is already fully immutable
- Current operator truth: deploys still depend on repo-local CDK source, auth UI source, and deploy-time AWS state

## M0.1 Current Mutable Deploy Inventory

The table below inventories the current release-driven path and separates Lesser-owned work from runner-owned work.

| Step | Current owner | Current source of truth / input | Deterministic at release time? | Must still happen at deploy time? |
| --- | --- | --- | --- | --- |
| Materialize a Lesser checkout or release payload for execution | Operator or `lesser-host` runner | Git checkout or downloaded release assets | No | Yes |
| Provide AWS credentials, region, and deploy inputs | Operator or `lesser-host` runner | Ambient AWS auth, CLI flags, `--provisioning-input` JSON | No | Yes |
| Resolve `app`, `base-domain`, hosted zone, and stage selection | Lesser CLI (`lesser up`) | CLI flags plus Route53 lookup | No | Yes |
| Validate external toolchain (`aws`, `cdk`, `go`, `pnpm`) | Lesser CLI | Local machine / runner environment | No | Yes |
| Build `bin/*.zip` Lambda artifacts | Lesser CLI | Repo-local Go source plus `infra/cdk/inventory/lambdas.go` | Yes | Today: yes. Future phase-1 goal: no |
| Run `cdk bootstrap` for the target account and region | Lesser CLI | AWS account + region state | No | Yes |
| Run `cdk deploy` for the shared and stage stacks | Lesser CLI | Repo-local `infra/cdk/`, deploy contexts, AWS account state | No | Yes |
| Build `auth-ui/` and upload it to the auth bucket | Lesser CLI | Repo-local `auth-ui/` source and `pnpm` toolchain | Yes | Yes |
| Write CloudFront invalidations and other deploy-time side effects | Lesser CLI | Stack outputs plus live AWS state | No | Yes |
| Bootstrap instance state, sync feature config, and write the local receipt | Lesser CLI | DynamoDB tables, deploy outputs, local filesystem | No | Yes |

### Lesser-owned mutable work today

These steps currently happen inside the `lesser` repo during `./lesser up`:

- Lambda compilation into `bin/*.zip`
- CDK bootstrap and deploy execution
- auth UI build and upload
- deploy receipt and stage bootstrap writes

### Runner-owned work today

The managed runner currently orchestrates deploy execution, but it does not define Lesser deploy assets itself. Its
responsibilities are:

- deciding which Lesser revision or release to materialize
- supplying AWS credentials and deploy inputs
- invoking `./lesser up`
- preserving any higher-level managed receipts outside this repo

### Release-time deterministic candidates

The current path mixes deterministic build work with truly deploy-time work. Milestone-0 freezes the distinction:

- Deterministic release-time candidates:
  `lesser` CLI binaries, `bin/*.zip` Lambda artifacts, and future release metadata that identifies those artifacts
- Still inherently deploy-time:
  CDK bootstrap, CloudFormation deploys, hosted-zone resolution, auth UI upload, bootstrap/admin state writes, and
  local receipt generation

## M0.2 Deploy Artifact Taxonomy

Milestone-0 freezes four artifact categories. These names are part of the contract.

| Category ID | Canonical assets | Purpose | Primary consumers | Status |
| --- | --- | --- | --- | --- |
| `operator_cli` | `lesser-<os>-<arch>` | Human- or runner-invoked CLI executable | Operators, CI, `lesser-host` runner | Already published |
| `release_metadata` | `checksums.txt`, `lesser-release.json` | Release-level discovery and integrity metadata | Operators, CI, artifact mode in `lesser up` | Already published |
| `lambda_bundle` | `lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json` | First-phase immutable deploy asset containing the canonical `bin/*.zip` set plus its manifest | Artifact mode in `lesser up`, managed runners | Published |
| `deploy_assembly` | Reserved future category | Later deploy package that may include more than prebuilt Lambdas | Future thin deploy executor / managed runners | Explicitly out of scope for the first phase |

### Taxonomy rules

- Operator CLI binaries are not deploy assets. They execute deploy workflows but are not themselves the payload being
  deployed.
- First-phase immutable deploy work targets only the `lambda_bundle` category.
- `deploy_assembly` is a separate future category and must not be backfilled into the first-phase Lambda bundle.
- Deploy receipts, bootstrap key material, provisioning input JSON, and other instance-local files are runtime inputs
  or outputs, not release artifacts.
- CDK source, auth UI source, and repo-local helper scripts remain source inputs until a later milestone explicitly
  publishes them as release artifacts.

## M0.3 Manifest And Checksum Contract For The Lambda Bundle

The first-phase Lambda deploy artifact category uses two canonical release assets:

- `lesser-lambda-bundle.tar.gz`: archive that reproduces the deployable `bin/*.zip` layout after extraction
- `lesser-lambda-bundle.json`: machine-readable manifest for the archive contents

The normative manifest schema is:

- `docs/contracts/lambda-bundle-manifest.schema.json`

The illustrative example is:

- `docs/contracts/examples/lesser-lambda-bundle.example.json`

### Manifest rules

- `kind` is always `lesser.lambda_bundle_manifest`.
- `schema_version` starts at `1`. Any breaking shape change increments `schema_version` instead of reusing the old
  meaning.
- `release` identifies the Lesser release that published the bundle.
- `bundle.path` is always `lesser-lambda-bundle.tar.gz`.
- `bundle.sha256` is the top-level checksum for the archive bytes exactly as published in the release.
- `files[]` enumerates the extracted deploy layout that `lesser up` will materialize under `bin/`.
- Every `files[].path` is a relative `bin/<lambda>.zip` path, sorted lexicographically, with no duplicates.
- Every `files[].sha256` hashes the exact zip file bytes at that extracted path.
- Every `files[].size_bytes` records the extracted zip size in bytes.
- `inventory_source.path` points at `infra/cdk/inventory/lambdas.go`, which remains the canonical source of truth for
  the deployable Lambda set.

### Checksum rules

- `checksums.txt` remains the release-level checksum file for top-level published assets.
- `lesser-lambda-bundle.tar.gz` and `lesser-lambda-bundle.json` must both appear in `checksums.txt`.
- `bundle.sha256` duplicates the archive checksum inside the manifest so consumers can verify the deploy artifact even
  after the bundle is staged away from the original release directory.
- Consumers must reject the bundle if the archive checksum, manifest checksum, or any listed file checksum does not
  match.
- The manifest must enumerate the exact stage-stack Lambda zip set and exclude non-deploy extras such as
  `cloudfront-keygen.zip`.

## M0.4 Artifact Selection Rules For `lesser up`

Milestone-0 chooses one canonical artifact-driven input surface for `lesser up`:

- `--release-dir <path>`

Phase one does not define a separate `--lambda-bundle <path>` flag. Operators and managed runners should stage the
trusted release assets into a directory and hand that directory to `lesser up`.

### Why `--release-dir` is canonical

- Operators naturally download or mirror whole release assets.
- Managed runners can materialize the same directory layout in ephemeral storage.
- The CLI can verify release metadata, top-level checksums, and the Lambda bundle from one trust root.
- A bundle-only flag would fragment trust, because the CLI would still need a second way to discover release metadata
  and integrity expectations.

### Required contents of `--release-dir`

Artifact mode requires these files in the release directory:

- `checksums.txt`
- `lesser-release.json`
- `lesser-lambda-bundle.tar.gz`
- `lesser-lambda-bundle.json`

Future milestones may add more files to the release directory, but first-phase Lambda deploy consumption only depends on
the four files above.

### Selection and verification behavior

When `--release-dir` is set, `lesser up` must:

1. Fail fast if any required release file is missing.
2. Verify `checksums.txt` for `lesser-release.json`, `lesser-lambda-bundle.tar.gz`, and `lesser-lambda-bundle.json`.
3. Read `lesser-release.json` to confirm the release is a Lesser release and to discover
   `artifacts.deploy_artifacts.lambda_bundle`.
4. Validate `lesser-lambda-bundle.json` against schema version `1`.
5. Verify the bundle archive checksum from the manifest.
6. Extract the archive deterministically into the exact `bin/*.zip` layout current CDK deploys consume.
7. Verify every extracted `bin/<lambda>.zip` against the manifest before using it.
8. Skip source-based Lambda compilation once verified assets are present.

### Override rules

- `--rebuild-lambdas` remains the explicit source-build override path.
- If both `--release-dir` and `--rebuild-lambdas` are set, `--rebuild-lambdas` wins and `lesser up` rebuilds Lambdas from source.
- Without `--release-dir`, `lesser up` continues to use the current source-based build path.
- Invalid or incomplete release assets are hard errors in artifact mode; the CLI must not silently fall back to an
  unrequested source build.

## M0.5 Deploy-Time Responsibilities That Remain Instance-Specific

Prebuilt Lambda release assets reduce compilation cost, but they do not eliminate deploy-time work. The following
responsibilities remain deployment-specific even after first-phase Lambda bundles exist:

| Responsibility | Why it remains deploy-time |
| --- | --- |
| AWS credential and account selection | Trust, billing, and target account state vary per deploy |
| Hosted-zone lookup and stage-domain routing | Domain ownership and DNS records are instance-specific |
| `cdk bootstrap` and `cdk deploy` execution | CloudFormation synthesis and updates depend on the target account, region, and stack history |
| CDK context inputs such as `bodyEnabled`, `lesserHostUrl`, translation flags, tips, and AI toggles | These are per-instance configuration choices, not release artifacts |
| Auth UI build and upload | The current deploy path still builds `auth-ui/` from repo-local source and uploads it separately |
| CloudFront invalidations and post-deploy writes | These target live distributions, buckets, DynamoDB tables, and local receipts |
| Bootstrap/admin state handling | Wallet/bootstrap behavior depends on the instance lifecycle and provisioning mode |

### Honest first-phase scope

The first-phase immutable deploy contract means:

- Lesser releases can publish trusted Lambda deploy assets.
- `lesser up` can consume those assets through `--release-dir` instead of recompiling handlers.

It does not mean:

- auth UI is already a release-published deploy artifact
- CDK execution is detached from repo-local infrastructure source
- deploys can run without AWS account, domain, or per-instance configuration context
- managed runners no longer need to provide provisioning input or retain their own higher-level receipts

### Current repo-local source assumptions that remain canonical

Artifact-driven Lambda deployment is now supported, but `lesser up` still requires a Lesser checkout with these
repo-local source inputs present:

- `infra/cdk/cdk.json` and the surrounding `infra/cdk/` app, because CDK synthesis and deploy still run from source
- `auth-ui/package.json` and the surrounding `auth-ui/` app, because auth UI upload is still built at deploy time

The CLI now validates those two source inputs up front so artifact mode fails with an explicit missing-source error
instead of drifting into later `cdk` or `pnpm` failures.
