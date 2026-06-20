---
name: deploy-instance
description: Use to walk a merged change through per-deployment per-stage rollout — dev → staging → live — via the `./lesser up` CLI. Respects the shared-stack → per-stage-stack deploy order, preserves bootstrap mnemonic handling, never sets timeouts on CDK commands, and coordinates rollback if a stage surfaces a regression. Also handles release-artifact publication for managed-consumer ingestion.
---

# Deploy lesser to a stage

After a feature → `staging` milestone is promoted to `main` by the operator, the change is ready to reach operator deployments. This skill is the discipline for walking a change through `dev → staging → live` for a given `(<app>, <base-domain>)` deployment, respecting the stack-deploy order, preserving bootstrap mnemonic state, and handling release-artifact publication.

## When this skill runs

Invoke when:

- A change has been promoted from `staging` to `main` and is ready for rollout on a given deployment
- An operational change (configuration, dependency) needs to propagate across stages
- A security or federation-trust fix is ready for compressed-cadence rollout (compression authorized separately)
- A rollback to a prior Lambda version or CloudFormation state is required
- A release cut is needed for managed-consumer ingestion (`lesser-host`'s provisioning worker)

## Preconditions

- **The change is on `main`.** Per-stage deploys run against the checked-out `main` (or specified commit) after the operator-owned staging → main promotion.
- **The deployment is identified** — `(<app>, <base-domain>, <aws-profile>)`.
- **The stage sequence is planned** — typically `dev → staging → live` or `dev → live` where staging is unused.
- **The roadmap's soak criteria are documented.** Soak is observable evidence, not a timer.
- **MCP tools healthy**, `memory_recent` first.
- **For compressed cadence** (security / federation-trust response), compression is authorized and recorded.
- **For rollback**, the target commit / Lambda version is identified; the target is still present and undeleted.
- **Bootstrap mnemonic** is present at `~/.lesser/<app>/<base-domain>/bootstrap.json` (or the `--out` override path). For first deploys, this file is created; for subsequent deploys, it must be present.

## The canonical deploy sequence

### Per deployment, per stage

For a given `(<app>, <base-domain>)`, progress through stages:

1. **Dev** — `./lesser up --app <slug> --base-domain <domain> --stage dev --aws-profile <profile>`
2. **Dev soak** — observable evidence meets criteria
3. **Staging** (where used) — `./lesser up ... --stage staging`
4. **Staging soak** — evidence meets criteria
5. **Live** — `./lesser up ... --stage live`
6. **Post-deploy monitoring** — active watch per declared plan

### The stack deploy order (within a stage)

`./lesser up` deploys stacks in this order (CDK handles the dependency resolution):

1. **`LesserSharedStack`** — deployed once per `(<app>, <aws-account>, <region>)`, not per stage. Shared S3 content buckets, CloudFront distribution, Route53 records, shared CloudFront Functions.
2. **`LesserAPIStack-<stage>`** — the per-stage stack. Lambda functions, API Gateway, DynamoDB, SQS, SNS, CloudWatch alarms, stage-specific IAM roles.

The shared stack deploys first because the per-stage stack references its outputs. CDK enforces this through stack dependencies; never alter the order manually.

### The CDK timeout rule

**Never set timeouts on CDK deploy commands.** A deploy that feels stuck is almost always waiting on a CloudFormation resource (Lambda, DynamoDB, IAM, API Gateway, CloudFront, S3), a stack rollback, or a CloudFront distribution propagation (which can take many minutes). Aborting loses the output that diagnoses the issue and leaves CloudFormation in a half-migrated state.

Run `./lesser up` to completion. Capture full output. If genuinely stuck, check CloudFormation console state through the user — don't abort.

### Running deploys

- **Live deploys run on operator authorization.** You do not initiate live deploys without explicit user authorization.
- **Dev and staging deploys** can be initiated by operators as part of normal rollout rhythm.
- **You do not modify deployment SSM parameters, IAM roles, Route53 records, or Secrets Manager entries manually outside CDK.**
- **You do not delete the bootstrap mnemonic file** or its canonical path (`~/.lesser/<app>/<base-domain>/bootstrap.json`). The mnemonic is operator-critical and cannot be regenerated.

## Dev soak

After `./lesser up --stage dev` completes:

- **Verify deploy success.** CloudFormation stacks reach `UPDATE_COMPLETE` or `CREATE_COMPLETE`. Lambda function versions updated. API Gateway stage pointing at new deploy. CloudFront distribution cache invalidation (if applicable).
- **Exercise the change.** Run integration tooling or manual verification against `https://dev.<base-domain>/`. Confirm the specific behavior the change targets.
- **Watch CloudWatch for error patterns** over the soak window.
- **Watch SNS error-topic messages** — this service publishes error-level logs to SNS; new messages are signal.
- **Watch SQS DLQ depth** for federation-delivery, ai-processor, moderation-processor.
- **Confirm DynamoDB Streams processors are healthy** — iterator age flat, invocations occurring on writes.
- **For federation-trust changes**: exercise signing and verification with test fixtures; monitor HTTP Signature verification failure rate.
- **For contract changes**: do not run smoke tests. Use targeted API/manual verification against the deployed endpoint plus static contract verification from the release commit.
- **For schema changes**: verify read and write paths against the new shape; confirm async processors see expected stream events.
- **Soak duration** per roadmap. Non-trivial: hours to a day. Federation / schema / contract changes: longer.

Do not promote to `staging` until dev soak criteria are met.

## Staging soak (where used)

Not every deployment uses staging. Where used, after `./lesser up --stage staging`:

- **Verify deploy success** (same checks as dev).
- **Integration partners exercise real flows** against `https://staging.<base-domain>/`.
- **Watch partner / operator support signals** through the user.
- **Federation behavior against test federation peers** is exercised here — if the deployment has a paired staging peer, confirm cross-instance activity delivery works.
- **Mastodon-client compatibility testing** happens here if the change affects REST API.
- **Soak duration** typically multiple days for non-trivial changes; longer for federation, schema, contract changes.

Do not promote to `live` until staging soak criteria are met.

## Live deploy

**Live is production. Real users. Real federation. Real remote Fediverse peers.** Deploy is fast; posture is measured.

- **Operator authorizes the live deploy explicitly.**
- **`./lesser up --stage live`** is the command. Apex domain `https://<base-domain>/`.
- **Post-deploy monitoring begins immediately. Watch:**
  - CloudWatch error rate per affected Lambda
  - API Gateway 4xx / 5xx rates
  - SNS error-topic messages
  - SQS DLQ depth (federation-delivery, ai-processor, moderation-processor)
  - DynamoDB Streams iterator age
  - Federation delivery success rate per remote domain
  - HTTP Signature verification failure rate
  - Locked / unlocked state consistency
  - Actor-object fetch success from remote peers (through Fediverse observability)
- **Watch window** varies. Narrow fix: minutes to hours. Federation / schema / contract changes: days.

## Release-artifact publication (for managed-consumer ingestion)

When a release will be ingested by `lesser-host`'s provisioning worker, the release cut requires:

- **Git tag** on `main` at the release commit — `v<major>.<minor>.<patch>`
- **`lesser-release.json`** — manifest (version, commit SHA, stack list, timestamps)
- **`lesser-lambda-bundle.tar.gz`** — compiled Lambda functions
- **Checksums** — SHA256 per asset, published alongside
- **GitHub Release** at `equaltoai/lesser/releases/tag/v<version>` — canonical publication point
- **Release notes** — breaking changes, migration guidance, federation-behavior changes, operator-facing configuration changes

Managed consumers verify checksums before deploying. Breaking the release-artifact shape without coordination breaks `lesser-host`'s provisioning flow — the change lands alongside a `host` steward coordination.

## If a stage surfaces a regression

- **Stop.** Do not promote further.
- **Diagnose quickly** — narrow (one operator, one endpoint) or broad?
- **Decide rollback scope**:
  - **Full rollback**: revert the commit on `main`, redeploy via `./lesser up` with the prior commit checked out. This is the primary mechanism.
  - **Lambda-version alias rollback** (emergency): point the active alias back at the prior Lambda version directly. Requires coordination; not the default path.
  - **Per-stage rollback**: roll back `live` while keeping `staging` / `dev` on the new commit.
- **Coordinate with operators through the user** if the regression affects production.
- **Never delete the regressed Lambda function version.** Lambda versions are immutable audit history.
- **Never delete the CloudFormation stack.** CDK rollback is the mechanism.
- **For schema rollbacks**, consider data remediation — some schema changes can't be cleanly undone by Lambda-version revert alone.
- **For federation-trust regressions**, consider whether the circuit breaker needs temporary tightening to prevent runaway retries.
- **Record the regression.** High-signal memory material.

## Output: the deploy record

```markdown
## Deploy record: <change name>

### Deployment
- App: <slug>
- Base domain: <domain>
- AWS profile: <profile>
- Operator: <identified>

### Dev
- Command: `./lesser up --app <slug> --base-domain <domain> --stage dev --aws-profile <profile>`
- Timestamp: <...>
- Lambda versions updated: <...>
- CloudFormation stack IDs: <shared stack, per-stage stack>
- Soak criteria met: <...>
- Soak duration: <...>
- Issues observed: <none / described>

### Staging (if used)
- Command: `./lesser up ... --stage staging`
- Timestamp: <...>
- Lambda versions: <...>
- Soak criteria met: <...>
- Soak duration: <...>
- Issues observed: <none / described>

### Live
- Authorized by: <operator>
- Command: `./lesser up ... --stage live`
- Timestamp: <...>
- Lambda versions: <...>
- Post-deploy monitoring window: <...>
- Issues observed: <none / described>

### Release artifacts (if cut)
- Git tag: <v<version>>
- GitHub Release URL: <...>
- Assets: lesser-release.json, lesser-lambda-bundle.tar.gz, checksums
- Release notes: <summary>
- Managed-consumer (lesser-host) coordination: <completed / n/a>

### Rollback (if any)
- Trigger: <...>
- Mechanism: <revert + redeploy / alias rollback / stack rollback>
- Prior commit / version: <...>
- Data remediation: <none / described>
- Operator coordination: <...>

### Follow-ups
- <subsequent scoping, fix, or monitoring task>
```

## Refusal cases

- **"Set a 10-minute timeout on the CDK deploy."** Never.
- **"Run the live deploy from my laptop without operator authorization."** Refuse.
- **"Skip `dev` soak; the change is small."** Refuse.
- **"Deploy to `live` without a staging soak" (where staging is used).** Refuse without authorization.
- **"Delete this Lambda function version; we're past it."** Refuse. Prior versions are rollback targets.
- **"Modify SSM parameters manually to fix the current issue."** Refuse unless operationally authorized.
- **"Deploy the per-stage stack before the shared stack; the shared stack is slow."** Refuse. CDK dependency order is real.
- **"Lose the bootstrap mnemonic file; we'll regenerate it."** Refuse. The mnemonic cannot be regenerated.
- **"Skip the checksum step for the release artifact."** Refuse. Managed consumers verify checksums; skipping breaks their ingestion.
- **"Abort the CDK deploy; it's been running too long."** Check CloudFormation console state first through the user.
- **"Disable an async processor while we deploy so the stream doesn't back up."** Usually refuse. Stream backpressure during deploys is expected; disabling processors creates a larger recovery problem.

## Persist

Append when the deploy surfaces something worth remembering — a CDK behavior quirk that matters for future deploys, a shared-stack vs per-stage-stack timing subtlety, a CloudFront propagation edge case, a managed-consumer verification detail, an operator-reporting pattern during soak, a rollback discovery, a release-artifact anomaly. Routine clean deploys aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff

- **All stages clean, release cut if applicable** — stop. Record the deploy, append memory if warranted, work is done.
- **Regression surfaced** — coordinate rollback, then route through `investigate-issue`.
- **Deploy-specific anomaly surfaced during soak** — route through `investigate-issue` or specialist skill.
- **Operator notification needed post-live** — surface to the user; you don't send notifications.
- **Managed-consumer (`lesser-host`) coordination issue surfaced** — report cross-repo to the `host` steward via the user.
- **Deploy reveals a scoping question** — route through `scope-need` once the current deploy is stable.