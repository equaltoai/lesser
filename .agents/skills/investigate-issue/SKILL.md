---
name: investigate-issue
description: Use when a user reports a bug, regression, federation failure, or unexpected behavior — inbound activity rejected, outbound delivery failing, Mastodon client showing wrong data, GraphQL subscription disconnecting, DynamoDB query returning empty unexpectedly, async processor backing up, schema-induced read error, operator deploy issue. Runs before any fix is proposed. Produces an investigation note, not a patch.
---

# Investigate an issue

Investigation comes before implementation. lesser has more dimensions to investigation than a typical Go service: many Lambdas with different triggers, GSIs serving specific access patterns, federation with arbitrary remote servers running arbitrary ActivityPub implementations, a Mastodon-compat REST surface consumed by clients you can't reach, a GraphQL surface, an ActivityPub actor surface, an AGPL mission, and a Theory Cloud framework-consumer role. A fix against a misunderstood symptom can break federation trust, strand Mastodon clients, or erode the schema-as-contract. The bar is "what breaks for every operator running the next release," not "does the test suite pass."

## Start with memory

Call `memory_recent` first. Scan for prior investigations in the same area — federation edge cases, schema-evolution decisions, Mastodon-compat constraints, AGPL considerations, framework-feedback signals already sent. Federation-platform investigations are context-dense.

## Capture the claim precisely

Record the user's report literally, then extract:

- **Symptom** — what was observed, verbatim where possible
- **Surface** — HTTP REST (`/api/v1/*`), GraphQL, WebSocket, ActivityPub inbox/outbox/actor/WebFinger, async processor, CLI (`lesser up` / `lesser verify` / etc.)
- **Lambda implicated** — `api`, `graphql`, `graphql-ws`, `inbox`, `outbox`, `actor`, `webfinger`, `objects`, `collections`, `federation-delivery`, `sse`, `streaming`, or an async processor (`note-processor`, `activity-processor`, `ai-processor`, `moderation-processor`, `media-processor`, `cost-aggregator`, `federation-aggregator`, `trend-aggregator`, etc.)
- **Consumer class** — operator running a lesser instance / Mastodon client / remote ActivityPub server / sibling equaltoai repo / direct API caller
- **Instance context** — `(<app>, <base-domain>)`, stage (`dev` / `staging` / `live`), recent deploy timestamp, `bodyEnabled` (legacy `soulEnabled`) setting
- **Actor / activity context** — `username`, local vs remote actor, activity type, payload shape (redact signing material)
- **Expected vs actual**
- **Reproduction path** — request shape, remote-actor context, payload

## Ground the investigation

Your first structural questions are always:

1. **Is this a federation-trust issue?** If the symptom suggests signature-verification bypass, delivery to or from an untrusted peer, actor impersonation, governance-state regression, or quiet swallowing of delivery errors — elevate. Federation-trust-suspected symptoms get elevated handling even if they turn out to be benign. Route through `protect-federation-trust`.
2. **Is this a Mastodon-compat issue?** If the symptom involves a Mastodon client seeing the wrong data or an unexpected error shape, route through `preserve-mastodon-api-compat` to walk the contract.
3. **Is this a schema issue?** If the symptom involves PK/SK construction, GSI projection, TableTheory tag behavior, or optimistic-concurrency versioning, route through `validate-schema`.
4. **Is the symptom in lesser, in a sibling repo, in a Theory Cloud framework, or in a remote Fediverse peer?** Many reported "lesser bugs" turn out to be `body` (MCP runtime) behavior, `host` (provisioning) config, greater-components rendering, Mastodon-client-side parsing, or remote-peer quirks. Before accepting the symptom as a lesser bug, confirm.
5. **Is this stage-specific?** `dev` vs `staging` vs `live` symptoms usually point at environment configuration (SSM, IAM, Secrets Manager, Cognito, DynamoDB capacity) rather than code.
6. **Is this instance-specific?** Different operator deployments have different configurations. An issue on one `(<app>, <base-domain>)` that doesn't reproduce on another often traces to instance-level config rather than code.
7. **Is the right fix here, or upstream in a framework?** If the symptom traces to an AppTheory middleware surprise or a TableTheory query-builder limitation, route through `coordinate-framework-feedback` — the fix probably belongs in the framework.

## Evidence before hypotheses

Gather before theorizing:

- `git log` on the affected package since the last known-good deploy
- `git blame` on the specific lines the reproduction implicates
- The affected Lambda's version and deploy timestamp
- CloudWatch logs for the affected Lambda and relevant request IDs (through the user; you don't have direct AWS access)
- X-Ray traces for latency or timeout symptoms
- SNS error-topic messages (this service publishes Error-level logs to SNS)
- SQS dead-letter queue depth for delivery-related symptoms
- DynamoDB Streams iterator age for processor-backup symptoms
- For federation issues: the remote-peer's actor object, their stated software version (from NodeInfo or user-agent), any FEP-specific behavior
- For Mastodon-client issues: the reported client, the exact request and response shapes
- For schema issues: the actual DynamoDB item shape (PK, SK, all attributes, GSI projections) for the affected record
- `query_knowledge` for cross-repo context — AppTheory / TableTheory patterns, sibling equaltoai repos, reference implementations

If `memory_recent` or `query_knowledge` returns an auth error, stop — investigating federation-platform regressions without context continuity compounds the risk.

## The specialist-routing question

Every investigation answers: **which specialist skill, if any, should handle this?**

- **Federation-trust** (signatures, delivery, inbox verification, moderation gates, governance state) → `protect-federation-trust`
- **Mastodon API / GraphQL / ActivityPub actor shape** → `preserve-mastodon-api-compat`
- **Schema / PK-SK / GSI / TableTheory tag / optimistic concurrency** → `validate-schema`
- **Framework awkwardness** (AppTheory middleware, TableTheory query-builder, CDK construct) → `coordinate-framework-feedback`
- **Deploy / stage / CLI / bootstrap mnemonic** → `deploy-instance`
- **Advisor-originated brief** (email ending `@lessersoul.ai` with provenance signature) → `review-advisor-brief`
- **None** — routes through the standard `scope-need` → `enumerate-changes` → `plan-roadmap` → `implement-milestone` → `deploy-instance` flow

## Rank hypotheses by evidence

List theories in descending order of support:

1. **Hypothesis** — one sentence
2. **Evidence for** — commits, logs, item state, stream lag, test coverage
3. **Evidence against** — what would be true if this were wrong
4. **Verification step** — the cheapest test to prove or disprove it

## Output: the investigation note

```markdown
## Reported symptom
<verbatim>

## Dimensions
- Surface: <REST / GraphQL / WebSocket / ActivityPub / async processor / CLI>
- Lambda: <...>
- Consumer class: <operator / Mastodon client / remote AP server / sibling repo / direct caller>
- Instance: <(app, base-domain, stage)>
- Actor / activity context: <...>
- Recent deploys: <...>

## Specialist elevation check
<normal investigation / elevate to protect-federation-trust / preserve-mastodon-api-compat / validate-schema / coordinate-framework-feedback / deploy-instance / review-advisor-brief>

## What is definitely true
<verified facts — DynamoDB item state, log entries, remote-peer evidence>

## Fix-locus verdict
<fix here (lesser) / fix upstream (AppTheory, TableTheory) / fix in sibling repo / fix in consumer / fix in deployment config / no-fix (peer-side issue, client-side issue)>

## Hypotheses (ranked)
1. <hypothesis> — evidence: <...>
2. <...>

## Verification step
<the one thing to run next>

## Proposed next skill
<investigate-issue again / fix directly / scope-need / protect-federation-trust / preserve-mastodon-api-compat / validate-schema / coordinate-framework-feedback / deploy-instance / review-advisor-brief / none — cross-repo report>
```

## Persist

Append only if the investigation surfaces something worth remembering — a federation edge case with a specific remote software version, a Mastodon-client parsing quirk, a schema or GSI subtlety, a framework awkwardness worth reporting upstream, an advisor-brief pattern, an operator-deploy constraint that will recur. Routine "typo in a log line" findings aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff rules

- **Federation-trust-adjacent** — route through `protect-federation-trust`.
- **Mastodon-compat or contract-surface issue** — route through `preserve-mastodon-api-compat`.
- **Schema / PK-SK / GSI / TableTheory** — route through `validate-schema`.
- **Framework awkwardness** (AppTheory, TableTheory) — route through `coordinate-framework-feedback` and report upstream; don't patch locally.
- **Deploy / stage / stack / CLI** — route through `deploy-instance`.
- **Advisor brief** — route through `review-advisor-brief`.
- **Small, contained fix** — route through `scope-need` → `enumerate-changes` → `implement-milestone` → `deploy-instance`.
- **Cross-repo or cross-framework finding** — report cleanly to the user; do not cross the boundary.
- **Peer-side (remote Fediverse server) issue** — document the finding, note that the fix is not in lesser, consider whether lesser needs defensive handling or tolerance improvements.