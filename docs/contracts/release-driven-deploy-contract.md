# Lesser Release-Driven Deploy Contract

This document freezes the current release-driven deploy contract and the next-step target contract for later immutable
deploy milestones.

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
| `deploy_assembly` | `lesser-deploy-assembly.tar.gz`, `lesser-deploy-assembly.json` | Later deploy package that may include more than prebuilt Lambdas | Future thin deploy executor / managed runners | Contract target defined, but not yet published in live releases |

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

- `lesser-lambda-bundle.tar.gz`: archive that reproduces the deployable `bin/*.zip` layout inside the local deploy workspace after extraction
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
6. Extract the archive deterministically into `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/bin/*.zip`.
7. Verify every extracted `bin/<lambda>.zip` against the manifest before using it.
8. Pass that staged asset root into CDK as the Lambda asset input and skip source-based Lambda compilation once verified assets are present.

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
- `infra/cdk/inventory/lambdas.go`, because artifact mode validates the release bundle against the checkout's canonical Lambda inventory
- `auth-ui/package.json` and the surrounding `auth-ui/` app, because auth UI upload is still built at deploy time

The CLI now validates those repo-local source inputs up front so artifact mode fails with an explicit missing-source error
instead of drifting into later `cdk` or `pnpm` failures.

## M3 Managed-Runner Consumption Path

Managed runners should treat artifact-driven deploy execution as a two-input model:

- repo-local execution source that still contains `infra/cdk/` and `auth-ui/`
- release-local deploy assets staged into `--release-dir`

The minimal managed-runner input contract is:

- a Lesser checkout with:
  - `infra/cdk/cdk.json`
  - `infra/cdk/inventory/lambdas.go`
  - `auth-ui/package.json`
- a checkout version whose canonical Lambda inventory matches the selected release bundle
- a `lesser` CLI binary from that checkout
- a release directory containing:
  - `checksums.txt`
  - `lesser-release.json`
  - `lesser-lambda-bundle.tar.gz`
  - `lesser-lambda-bundle.json`
- deploy-time AWS credentials and instance inputs (`--app`, `--base-domain`, optional `--provisioning-input`, and bootstrap/output flags)

When artifact mode runs successfully, the local Lesser state dir becomes the canonical execution workspace:

- transient release extraction workspace: `~/.lesser/<app>/<base-domain>/deploy/release-lambda-assets.*`
- staged Lambda asset root: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/`
- staged Lambda provenance metadata: `~/.lesser/<app>/<base-domain>/deploy/lambda-assets/metadata.json`
- per-stack CDK outputs: `~/.lesser/<app>/<base-domain>/deploy/cdk-outputs/<stack>.json`
- stable instance receipt: `~/.lesser/<app>/<base-domain>/state.json`

This keeps the managed-runner contract explicit:

- release assets are external deploy inputs, not mutable repo byproducts
- repo-local CDK/auth-ui source still remains an execution prerequisite
- the checkout must stay version-aligned with the release bundle because bundle validation uses the checkout's canonical Lambda inventory
- receipt/output behavior stays in the same Lesser-owned state dir the operator path already uses

## M4.1 Per-Instance Context Inventory That Still Blocks A Reusable Deploy Assembly

Moving beyond prebuilt Lambdas requires naming which contexts still keep deploy execution instance-bound today.

| Context family | Examples | Current source of truth | Release-generic or instance-bound? | Compile/release-time or deploy-time? | Why it still blocks a reusable deploy assembly today |
| --- | --- | --- | --- | --- | --- |
| App identity | `--app`, stack names, Lesser state-dir prefix | CLI flags and local receipt paths | Instance-bound | Deploy-time | The same release can be installed under different app slugs, so synthesized stack names and local state paths cannot be baked once per release |
| Domain routing | `--base-domain`, per-stage domains, CloudFront aliases | CLI flags, `stageURLs`, Route53 resolution | Instance-bound | Deploy-time | DNS ownership and stage-domain mappings vary per deployment and depend on live hosted-zone/account state |
| Hosted zone selection | hosted zone ID/name, public DNS authority | Route53 lookup in the target AWS account | Instance-bound | Deploy-time | The deploy executor must bind the release to the actual hosted zone for the target account instead of assuming one zone per release |
| AWS target environment | account ID, region, bootstrap state, CDK environment | Ambient AWS auth and CDK bootstrap state | Instance-bound | Deploy-time | CloudFormation updates, bootstrap buckets, and trust boundaries are specific to the target account and region |
| Stage state | stage stack history, prior outputs, stage-specific URLs | Existing CloudFormation stacks plus Lesser receipts | Instance-bound | Deploy-time | Stage deploys are updates against live stack history rather than one-shot release materialization |
| Feature/config toggles | `bodyEnabled`, `lesserHostUrl`, translation flags, AI toggles, tips, managed provisioning inputs | CLI flags, env vars, provisioning JSON, stack context | Instance-bound | Deploy-time | These change behavior for one installation without changing the underlying release payload, so they must stay outside generic release artifacts |
| Bootstrap/admin lifecycle | bootstrap receipts, setup completion state, admin writes | `~/.lesser/...`, DynamoDB, setup endpoints | Instance-bound | Deploy-time | Lifecycle state depends on what has already happened in the target installation |
| Auth/UI publish destination | auth bucket name, CloudFront invalidation target | Stack outputs and live AWS state | Instance-bound output over release-generic source | Deploy-time for upload, release-time candidate for build | The UI source is generic, but the destination bucket/distribution is chosen from live stack outputs at deploy time |

### Generic release state versus instance-bound deploy state

The reusable pieces that belong to a release contract are:

- the Lesser revision (`version`, `git_sha`) and its published checksums
- the operator CLI binary that executes deploy workflows
- the Lambda bundle and manifest that reproduce the canonical `bin/*.zip` set
- the repo revision that defines the canonical CDK app, auth UI source, and Lambda inventory
- any future published deploy assembly payload that is valid for every installation of that release

The still-instance-bound pieces are:

- app slug, base domain, hosted zone, AWS account, and region
- stage stack history, bootstrap state, and prior CloudFormation outputs
- per-instance feature/config toggles and provisioning input JSON
- post-deploy side effects such as DNS records, invalidations, receipt writes, and bootstrap/admin state changes

### Compile-time versus deploy-time contexts

Milestone 4 keeps one boundary explicit:

- Release-time / compile-time candidates: Lambda compilation, auth UI bundling, CDK or equivalent deploy assembly synthesis, release manifests, and checksums
- Deploy-time only: AWS credential selection, hosted-zone lookup, stack update planning against live history, feature-flag injection, DNS writes, invalidations, and instance receipt/bootstrap updates

## M4.2 Future Release Contract For A Publishable Deploy Assembly

Milestone 4 does not publish a deploy assembly yet, but it does freeze the outer contract shape for when that happens.

The future release directory layout for the `deploy_assembly` category is:

- `lesser-deploy-assembly.tar.gz`: archive containing the release-published deploy assembly payload
- `lesser-deploy-assembly.json`: descriptor that tells a thin deploy executor how to verify and interpret the archive

The normative descriptor schema is:

- `docs/contracts/deploy-assembly-descriptor.schema.json`

The illustrative example is:

- `docs/contracts/examples/lesser-deploy-assembly.example.json`

### Why the contract uses a descriptor instead of freezing one payload format

The outer Lesser contract stays stable even if the inner payload changes over time. That is why the descriptor freezes:

- the published archive path and checksum
- the release identity (`name`, `version`, `git_sha`)
- the payload kind and entrypoint
- the executor compatibility contract
- the required instance-input categories and verification expectations

The descriptor deliberately does not freeze one implementation such as AWS Cloud Assembly forever. Instead:

- `payload.kind` names the inner assembly type, such as `aws.cloud_assembly`
- `payload.entrypoint` identifies the file inside the archive that the future executor should start from
- `payload.contract_version` version-controls the inner payload independently of the outer Lesser descriptor

This leaves room for Cloud Assembly or an equivalent plan format without forcing the release contract to rename assets or
change trust boundaries later.

### Compatibility with the current release manifest model

When Lesser eventually publishes a deploy assembly, the top-level `lesser-release.json` contract should extend the
existing `artifacts.deploy_artifacts` section instead of inventing a second discovery document.

The future shape is:

- `artifacts.deploy_artifacts.deploy_assembly.path`
- `artifacts.deploy_artifacts.deploy_assembly.manifest_path`
- `artifacts.deploy_artifacts.deploy_assembly.manifest_kind`
- `artifacts.deploy_artifacts.deploy_assembly.manifest_schema_version`

That mirrors the existing `lambda_bundle` reference model:

- release metadata points at one published archive plus one machine-readable descriptor
- the descriptor carries the detailed contract for the archive contents and compatibility rules
- `checksums.txt` remains the top-level integrity root for the published assets

Milestone 4 does not change the live release manifest schema yet. It freezes the next-step target so the future
`deploy_assembly` category can be added as a sibling of `lambda_bundle` rather than as a separate trust model.

## M4.3 Separate Generic Release Artifacts From Instance-Specific Deploy Inputs

The future deploy contract uses a two-part model:

- generic release artifacts published once per Lesser release
- instance-specific deploy inputs supplied separately for each installation or update

### Generic release artifacts

These artifacts must stay identical for every consumer of the same Lesser release:

| Release artifact | Scope | Why it stays generic |
| --- | --- | --- |
| `lesser-<os>-<arch>` CLI binaries | Release | They execute workflows, but they are not parameterized by one target installation |
| `checksums.txt` and `lesser-release.json` | Release | They define discovery and integrity for the published assets |
| `lesser-lambda-bundle.tar.gz` and `lesser-lambda-bundle.json` | Release | They reproduce the canonical Lambda asset set for that release |
| Future `lesser-deploy-assembly.tar.gz` and `lesser-deploy-assembly.json` | Release | They package the reusable deploy assembly payload and its descriptor once per release |

### Canonical instance-specific input set

The future thin deploy executor should consume these input categories separately from the release artifacts:

| Input category | What it covers | Why it stays instance-specific | Current operator / runner surface |
| --- | --- | --- | --- |
| `app_identity` | app slug, stack prefix, state-dir namespace | One release can be installed under many app names | `--app` and local receipt paths |
| `aws_target` | AWS credentials, account, region, bootstrap environment | Trust and billing boundaries vary per deploy | AWS profile/env credentials plus CLI region resolution |
| `base_domain` | root domain for the installation | Domains vary per customer/instance | `--base-domain` |
| `hosted_zone` | actual Route53 hosted zone binding | The same domain may resolve in different AWS accounts or zones | Route53 lookup and optional operator choice |
| `stage_plan` | which stages are being updated plus their live stack history | Deploys are updates against existing stage stacks | CloudFormation state and Lesser receipts |
| `feature_config` | `bodyEnabled`, translation flags, AI toggles, tips, and similar behavior switches | These are installation-level decisions, not release identity | CLI flags, env vars, and provisioning input JSON |
| `managed_service_urls` | `lesserHostUrl`, attestations URL, similar managed endpoints | Managed control-plane wiring differs by installation | provisioning input JSON or env vars |
| `provisioning_input` | operator/runner-supplied bootstrap and managed config blob | It is provided per deploy request | `--provisioning-input` |
| `bootstrap_io` | output/bootstrap file locations and receipt handling | File paths are runner-local execution concerns | `--out`, local Lesser state dir, managed receipts |

### Separation rules

The contract boundary is:

- release artifacts may describe required input categories, but they must not embed per-instance values
- app names, domains, hosted-zone IDs, account IDs, feature flags, and provisioning JSON stay outside published artifacts
- the same release artifact set must be reusable across different customers and domains without rewriting the artifact bytes
- operators and managed runners should map their local invocation surfaces into the same canonical input categories above
- `docs/contracts/deploy-assembly-descriptor.schema.json` freezes those exact category names so future descriptors cannot silently invent a different taxonomy

This keeps the future deploy assembly usable by both modes:

- operators can continue supplying flags and files that map onto the canonical categories
- managed runners can build the same category map from their higher-level job model without mutating the published release artifacts

## M4.4 Migration Path From Source-Shelling `cdk deploy` To A Thin Deploy Executor

Today, `./lesser up` still treats a source checkout as the deploy substrate:

- it shells into the repo-local CDK app
- it validates or builds local deploy artifacts
- it builds and uploads `auth-ui/` from source
- it writes local receipts and bootstrap outputs in the same execution flow

Milestone 4 freezes the migration path so later work can remove those source dependencies deliberately instead of by
accident.

### Target responsibilities for a thin deploy executor

The future executor should still own only the work that must remain deploy-time:

- verify the selected release assets and their checksums
- accept the canonical instance-input set for one installation
- bind release artifacts to the target AWS account, region, domain, and hosted zone
- execute the infrastructure update plan against live CloudFormation state
- publish the resulting outputs, receipts, and post-deploy side effects

The future executor should stop owning release-time work such as:

- compiling Lambdas from source
- synthesizing the deploy assembly from repo-local CDK/app source
- rebuilding any release-generic frontend payloads
- discovering artifact shape from ad-hoc repo structure

### Migration sequence

| Phase | Deploy substrate | What still comes from source? | What must move into release assets next? |
| --- | --- | --- | --- |
| Current M0-M3 path | source checkout plus optional `--release-dir` Lambda bundle | CDK app, auth UI source, Lambda inventory, helper scripts | reusable deploy assembly, auth UI artifact, executor-facing input contract |
| M4 contract target | source checkout for execution, but with a frozen future deploy-assembly contract | CDK shelling and auth UI source still remain live dependencies | publishable deploy assembly descriptor/archive and explicit instance-input contract |
| Future phase-2 executor | release directory plus instance-input set | only tooling needed to execute the deploy contract, not the full repo source tree | release-published infrastructure assembly, frontend payloads, and executor compatibility metadata |

### Dependencies that must become explicit future work

The migration cannot complete until Lesser publishes or freezes contracts for:

- a deploy assembly payload that replaces repo-local CDK synthesis as the generic release artifact
- release-published frontend assets, or an explicit reason the frontend remains outside the immutable path
- executor compatibility rules for CloudFormation/bootstrap behavior against live AWS state
- receipt/output contracts that preserve the current Lesser state-dir behavior without depending on a mutable checkout

Milestone 4 keeps those dependencies named. It does not imply they are already solved by the Lambda bundle milestone.

## M4.5 Phase-2 Cost, Latency, Reliability, And Verification Goals

The next immutable deploy phase exists to solve an operational problem, not just to publish more files. The target
state is a thinner deploy path that avoids repeated release-generic work during each installation or client update.

### Outcome goals

| Goal area | Current managed-deploy pain | Phase-2 target state | How success should be verified |
| --- | --- | --- | --- |
| Cost | Each deploy still pays for repo-local synthesis/build tooling beyond the Lambda bundle | Release-generic assembly work happens once per release publish, not once per deploy | Managed deploy logs show artifact verification plus executor work, but no repo-local Lambda compile, deploy-assembly synthesis, or frontend rebuild steps |
| Latency | Mutable source preparation still delays deploy start and compounds runner time | A managed runner should reach the first live AWS deploy operation after artifact verification and input validation only, without any release-generic build stage in the path | Future runner smoke tests and timing instrumentation show deploy preparation is bounded by asset download/verification rather than by source builds |
| Reliability | Repo drift and local toolchain state can change deploy behavior for the same release | The same release artifacts plus the same instance-input set should resolve to the same deploy assembly contract and verification results | Descriptor/checksum validation, executor compatibility checks, and source-independence smoke tests all pass for the same release/input pair |
| Verification | Today the contract is partly implicit in repo layout and CLI behavior | The future executor must fail fast on missing assets, incompatible descriptor versions, or incomplete instance inputs before mutating AWS state | CI contract tests, release checks, and executor preflight tests catch drift before a live deploy begins |

### Target-state verification guardrails

Later implementation work should prove the phase-2 target with explicit checks:

- contract tests for the published deploy assembly descriptor schema and examples
- release checks that the future deploy assembly assets and `lesser-release.json` references are present and checksummed
- executor integration tests that consume release assets plus instance inputs without needing repo-local synthesis or rebuilds
- managed-runner smoke tests that confirm the deploy path stays on the artifact/executor route rather than silently falling back to source-shelling

### What milestone 4 deliberately leaves unchanged

Milestone 4 defines the target and the migration path, but it does not claim:

- CDK source is already gone from deploy execution
- auth UI is already a release-published immutable asset
- a no-checkout deploy path is shipping today
- managed deploy pain is solved by the Lambda bundle milestone alone
