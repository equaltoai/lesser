# Release, branch, and stage discipline

lesser uses a **single-main branch model** with feature branches and a **CLI-driven deployment model** rather than CI-driven per-branch pipelines. Each deployment is parameterized by `--app <slug> --base-domain <domain>` and reaches three stages per deployment: `dev`, optional `staging`, and `live`.

This differs from the release models used elsewhere in the Theory Cloud / PayTheory fleet (staging → premain → main, per-partner per-stage). lesser's shape reflects its open-source operator-run posture: the repo is the source of truth; operators consume releases and run their own deployments.

## Branch model

Observed pattern:

- **`main`** — canonical, mainline. Every merge lands here. No formal staging or premain branch.
- **Feature branches** — short-lived, often personal (`aron/*`, `chore/*`, topic-named like `aron/status-contract-s*`, `aron/dm-rewrite-m*`, `chore/apptheory-v*`, `chore/deps-latest`).
- **codex/-prefixed branches** — codex-driven exploration and milestone work (e.g., `codex/federation-m1.4f`).
- **`main` is always deployable.** Operators can check out any commit and run `./lesser up`.

Branch protection on `main` enforces required reviews and status checks. These are governance, not inconvenience.

## Local CI discipline before PRs

For lesser, the hard local pre-PR gate is the repo-native CI command:

```bash
go build -o lesser ./cmd/lesser
./lesser build lambdas
./lesser verify ci
```

Rules:

- **Nothing gets PR'd without `./lesser verify ci` passing locally.**
- **`./lesser verify ci` is the source-of-truth local gate for `main` readiness.** Do not substitute generic `go vet ./...`,
  `gofmt -l .`, or ad hoc command bundles for PR-readiness decisions.
- **Be patient.** A normal local `./lesser verify ci` run can take around **30 minutes**. Long quiet stretches are normal,
  especially during coverage batches and package sweeps.
- **Mirror CI's sequence.** The GitHub Actions workflow builds the `lesser` CLI and Lambda artifacts before `./lesser verify ci`;
  local runs should do the same.
- **Do not infer a broken gate from impatience.** If a local `./lesser verify ci` run looks slow, wait for completion before
  concluding that the branch is red.
- **If local and GitHub CI disagree, investigate the environment and the exact failing phase.** Do not weaken the gate.

## The three-stage deployment model

A lesser deployment is scoped by `(<app>, <base-domain>)`. For a given deployment, three stages exist:

### `dev` stage

- **Subdomain**: `dev.<base-domain>`
- **Purpose**: development integration. Operators run real flows against a live AWS deployment to verify behavior before promoting.
- **Stack names**: `LesserSharedStack` (once per app/account/region, cross-stage) + `LesserAPIStack-dev`.
- **Typical consumers**: the operator team running the deployment, integration tooling, test data.

### `staging` stage (optional)

- **Subdomain**: `staging.<base-domain>`
- **Purpose**: partner/operator validation. Integration partners run real flows against production-equivalent code in a non-production domain.
- **Stack names**: `LesserAPIStack-staging`.
- **Not every deployment uses staging.** Some operators deploy `dev → live`; others insert a `staging` step for partner-facing verification.

### `live` stage

- **Apex**: `<base-domain>` (no subdomain prefix)
- **Purpose**: production. Real users, real federation, real activities, real operator responsibility.
- **Stack names**: `LesserAPIStack-live`.
- **CDK configuration uses RemovalPolicy.RETAIN** for stateful resources in live (DynamoDB, S3 content buckets), protecting data across stack updates.

## The `lesser up` CLI

The canonical deploy path is:

```
./lesser up \
  --app <slug> \
  --base-domain <domain> \
  --aws-profile <profile> \
  --stage <dev|staging|live> \
  [--out <path>] \
  [--release-dir <path>] \
  [--staging]
```

Behaviors:

- **Idempotent.** Running `./lesser up` twice produces the same deployment state.
- **Bootstrap mnemonic**: on first deploy, a bootstrap mnemonic is written to `~/.lesser/<app>/<base-domain>/bootstrap.json` (or `--out <path>` override). This file unlocks signing-key material and is operator-critical — preserving it across re-deploys is required. Losing it requires re-provisioning the deployment.
- **Shared stack + per-stage stack sequence**: the shared stack (S3, CloudFront, DNS foundation) deploys once per app/account/region; the per-stage stack deploys each time.
- **Locked-on-deploy**: new deployments boot in a locked state (empty timelines, signups disabled) until the operator unlocks via the `config` endpoint. This prevents federation chatter before the instance is properly configured.
- **Consumer ingestion mode** (`--release-dir <path>`) — managed consumers (notably `lesser-host`'s provisioning worker) deploy lesser from a downloaded release bundle rather than building from source, with checksum verification.

## Rollout discipline

The standard rollout for a change:

1. **Feature branch merges to `main`** via PR with required review.
2. **Operator deploys to `dev`** for their `<app, base-domain>` via `./lesser up --stage dev`. Watch CodeBuild / local CDK output; deploys run to completion — no timeouts.
3. **Soak in `dev`**. Observable evidence that the change behaves correctly: account operations, note creation, federation delivery, inbox handling, moderation tooling, API-compat surfaces. Soak is evidence, not a timer.
4. **Deploy to `staging`** if the operator uses a staging stage. Integration partners exercise real flows. Soak again.
5. **Deploy to `live`**. Real users see the change. Post-deploy monitoring: CloudWatch error rate, API latency, federation delivery success rate, SQS DLQ depth, DynamoDB capacity metrics, SNS error topic.

Skipping stages requires explicit operator authorization. The default is `dev → (staging →) live` with soak between each.

## Release artifacts and consumer verification

When cutting a release that managed consumers (notably `lesser-host`) will ingest:

- **`lesser-release.json`** — the release manifest (version, commit SHA, timestamps, stack list).
- **`lesser-lambda-bundle.tar.gz`** — the compiled Lambda functions.
- **Checksums** — `sha256` for each asset. Managed consumers verify these before deploy.
- **GitHub Release** on `equaltoai/lesser` — the canonical publication point. Assets are attached there.

The release workflow is documented in `docs/release.md` (or equivalent). When a release workflow exists, it's driven by a git tag (`v<version>`) and produces the artifacts above.

**Managed consumers' deployment verification is load-bearing.** `lesser-host`'s provisioning worker verifies checksums before proceeding; breaking that verification flow breaks every managed lesser deployment.

## Never set timeouts on CDK deploy commands

This rule is inherited from the broader Theory Cloud / PayTheory fleet and applies here: **never set timeouts on CDK deploy commands.** A deploy that feels stuck is almost always waiting on a CloudFormation resource (Lambda, DynamoDB, IAM, API Gateway, CloudFront, S3), a rollback, or a drift check. Aborting loses the output that diagnoses the issue and leaves CloudFormation in a half-migrated state that takes longer to unblock than just waiting.

Run deploys to completion. Capture full output. If a deploy is genuinely stuck, check CloudFormation console state through the user; don't abort.

## Hotfix discipline

For urgent production issues — federation-trust regressions, CVE responses, data-integrity bugs:

- **Compressed soak durations**, not skipped stages. `dev` soak may be minutes instead of hours; `staging` soak may be hours instead of days; `live` post-deploy monitoring intensifies.
- **Explicit user authorization** for compression is recorded.
- **Post-incident review.** Every hotfix produces a write-up on what the stage soak missed.

## Rollback discipline

Rollback mechanisms:

- **Lambda-version rollback.** Lambda versions are immutable; rolling back means pointing the active alias back at the prior version via the next `./lesser up` with the prior commit checked out, or via direct alias management through the operator.
- **CloudFormation stack rollback.** CDK's `cdk deploy` invokes CloudFormation's own rollback on failed deploys. For stable-but-regressed deploys, the rollback is a revert commit on `main` followed by a new `./lesser up`.
- **Schema rollback.** Schema-changing deploys are rare and require explicit schema-rollback planning. Removing a GSI, changing PK/SK semantics, or deleting attribute types are operations that cannot be cleanly undone by a Lambda-version revert alone.
- **Federation-state rollback** is not really a thing: activities that have been delivered to remote instances are out in the world. Rollback restores lesser's local view but does not recall what was sent.

- **Never delete a Lambda function version** that could be a rollback target.
- **Never delete a CloudFormation stack** without an explicit data-migration plan.
- **Never delete the bootstrap mnemonic file** — it's operator-critical and cannot be regenerated.

## GraphQL schema and OpenAPI spec discipline

lesser's public contracts are versioned in the repo:

- **GraphQL schema**: `graph/*.graphql` (modular: `core.graphql`, `phase1.graphql`, `phase2.graphql`, `phase3.graphql`). Composed into `docs/contracts/graphql-schema.graphql`. Regenerate via `./lesser schema` or `./scripts/generate_schema.sh`.
- **OpenAPI spec** (Mastodon-compat REST): `docs/contracts/openapi.yaml`. Regenerated alongside handler changes; the file is the authoritative contract.
- **Regeneration rides with the code change** that affects the contract. A PR that changes a resolver or handler signature must update the generated contract in the same PR.
- **`./lesser verify`** or equivalent runs schema consistency checks; a steward who changes contract-adjacent code must confirm verify passes before promoting.

## AGPL-compatible release discipline

- **No minified or obfuscated artifacts committed.**
- **Generated files commit with clear provenance** (tool, version, command used).
- **Dependencies vetted for AGPL compatibility** on add. `go.mod` additions that introduce incompatible licenses are refused.
- **DCO / signed commits** where the project enforces them.

## Commit and PR discipline

- Clear, present-tense commit subjects. Lowercase style observed: "lint green", "[codex] complete federation m1.4f", "feat: milestone M1.4g".
- First line under 72 characters.
- Explain the *why* in the body for federation-adjacent, schema-adjacent, API-adjacent, or AGPL-adjacent changes.
- **Run the full local hard gate before PRing:** `go build -o lesser ./cmd/lesser && ./lesser build lambdas && ./lesser verify ci`.
- PRs through required review. Review is substantive.
- Conventional Commits style (`feat:`, `fix:`, `chore(deps):`, `docs:`) is welcomed but not mandatory; the lowercase present-tense style is also observed.

## Rules you do not break

- Never force-push to `main`.
- Never amend a commit that has been pushed.
- Never skip pre-commit hooks (`--no-verify`).
- Never bypass required review.
- Never open or recommend a PR as ready before `./lesser verify ci` has passed locally.
- Never deploy to `live` without successful `dev` (and `staging` where used) soak.
- **Never set a timeout on a CDK deploy command.**
- Never commit secrets, signing keys, partner credentials, or `.env` files.
- Never log full actor private keys, full JWT tokens, raw passwords, or raw credentials.
- Never change PK/SK patterns, GSI structure, or TableTheory tags without running the `validate-schema` walk and coordinating with every affected processor and consumer.
- Never break Mastodon REST API compatibility without explicit consumer coordination and a migration plan.
- Never change the ActivityPub actor object shape without federation-compat verification.
- Never skip HTTP Signature verification on inbound activities.
- Never skip HTTP Signature signing on outbound activities.
- Never delete the bootstrap mnemonic file or its documented location.
- Never delete Lambda function versions that could be rollback targets.
- Never run production deploys without operator authorization.
- Never introduce proprietary blobs or AGPL-incompatible dependencies.
- Never patch AppTheory or TableTheory locally. Framework awkwardness is signal; report it upstream via `coordinate-framework-feedback`.
- Never execute an advisor-dispatched brief without running the `review-advisor-brief` skill and surfacing to Aron for authorization.
