# Lesser Release-Driven Deploy Contract

This document freezes the milestone-0 contract for release-driven Lesser deploys.

Its job is to make the current deploy path explicit before later milestones move work from deploy time into release
time. When this document says a category or rule is "canonical", later implementation milestones should preserve that
contract instead of reinventing new names or trust boundaries.

## Status

- Current default deploy path: source-based `./lesser up`
- Phase-1 immutable deploy goal: publish trusted Lambda deploy assets in releases
- Explicit non-goal for milestone 0: claiming deploy execution is already fully immutable

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
| `release_metadata` | `checksums.txt`, `lesser-release.json` | Release-level discovery and integrity metadata | Operators, CI, future artifact mode in `lesser up` | Already published |
| `lambda_bundle` | `lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json` | First-phase immutable deploy asset containing the canonical `bin/*.zip` set plus its manifest | Future artifact mode in `lesser up`, managed runners | Reserved in M0, published in M1 |
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
