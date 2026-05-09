---
name: plan-roadmap
description: Use after enumerate-changes. Takes a flat enumerated change list and sequences it with dependencies, risks, and a per-stage rollout plan across dev → staging → live. Produces a roadmap document, not code or project state.
---

# Plan a roadmap

A flat enumerated list answers "what changes." A roadmap answers "in what order, with what risks, through which stages, for which operators, with what coordination outside this repo." This skill is the bridge.

lesser roadmaps are bounded by three axes: per-deployment (each operator runs their own `(<app>, <base-domain>)`, so the roadmap applies to whichever deployments adopt the release), per-stage (`dev → staging → live` per deployment), and per-federation-peer (changes that affect federation behavior have implicit rollout across arbitrary remote peers). The roadmap names the reality of each.

## Input required

An approved enumerated change list from `enumerate-changes`. Specialist-skill findings (from `protect-federation-trust`, `preserve-mastodon-api-compat`, `validate-schema`, `coordinate-framework-feedback`, `deploy-instance`, `review-advisor-brief`) if applicable. Load prior context with `memory_recent`.

## Dependency analysis

For each enumerated item, identify:

- **Hard dependencies** — items that must land first to compile or pass tests
- **Soft dependencies** — items that should land first for the change to make sense in review
- **Sibling-repo coordination dependencies** — items requiring `body`, `soul`, `host`, `greater`, or `sim` steward awareness / parallel work
- **Framework coordination dependencies** — items requiring AppTheory, TableTheory, or other Theory Cloud framework stewards to be aware
- **Mastodon-client coordination dependencies** — items where breaking changes require ecosystem notification
- **Remote-peer coordination dependencies** — items where federation behavior change may require peer-operator awareness (rare; usually additive via FEPs)
- **Parallelizable siblings** — items with no ordering constraint

## Phase shape

Canonical phase patterns for lesser:

1. **Infrastructure / dependency baseline** — Go module bumps, CDK foundation changes, AppTheory / TableTheory version bumps. Lands first.
2. **Model / schema additions (backward-compatible)** — new TableTheory-tagged fields, new SK patterns for new entity types, new GSI projections. Lands before handlers that populate new fields.
3. **Service-layer changes** — domain services, federation logic, moderation logic. Lands before handlers.
4. **Handler changes** — REST handlers, GraphQL resolvers, ActivityPub endpoint handlers, async processors. Consumes model and service layers.
5. **Contract regeneration** — `docs/contracts/graphql-schema.graphql`, `docs/contracts/openapi.yaml` updates. Rides with handler changes.
6. **CDK changes** — infrastructure adjustments. Isolated from Lambda code.
7. **Documentation** — operator docs, architecture docs, federation docs, README.

Not every roadmap uses all phases. A pure bug fix may be one phase. A schema-expanding federation feature may be five or six. More than six phases suggests scope crept past the scoped-need document; revisit `scope-need`.

## Stage rollout discipline

Every roadmap answers: **how does this reach `live` safely, for operators running the release, with federation peers and Mastodon clients unsurprised?**

The default rollout:

1. **Feature branch work completes.** Tests pass. Required review. Merge to `main`.
2. **Operator deploys to `dev`** via `./lesser up --stage dev`. Observable evidence that behavior is correct.
3. **Soak in `dev`.** Not a timer — evidence. Account operations work, note creation + federation delivery work, inbox verification works, Mastodon-compat surfaces behave correctly, SNS error-topic is clean, SQS DLQ is empty for delivery.
4. **Deploy to `staging`** if the operator uses it. Integration partners exercise real flows. Soak again.
5. **Deploy to `live`** via `./lesser up --stage live`. Real users. Real federation.
6. **Post-deploy monitoring**:
   - CloudWatch error rate per Lambda
   - API Gateway 4xx / 5xx rates
   - GraphQL subscription disconnects
   - SNS error-topic messages
   - SQS DLQ depth (federation-delivery, ai-processor, moderation-processor)
   - DynamoDB Streams iterator age
   - Federation delivery success rate per remote domain
   - Mastodon-client integration signals (through operator reports)
   - HTTP Signature verification failure rate
   - Locked/unlocked state consistency

**Never set timeouts on CDK deploys.** Let them run to completion.

**Never skip `dev` soak for urgency.** Hotfix compression is possible within stages, not by skipping them.

## Per-operator dimension

Because operators each run their own `(<app>, <base-domain>)`, the roadmap does not prescribe which operators upgrade when. Instead:

- **Release on GitHub Releases** with checksums, release notes, and migration guidance
- **Document breaking changes prominently** in the release notes
- **Provide migration tools or scripts** if data migration is required
- **Communicate federation-behavior changes** in the release notes so operators can choose when to upgrade their federation surface

For managed consumers (notably `lesser-host`'s provisioning worker), release-artifact verification is part of the rollout. A breaking change to the release artifact shape coordinates with the `host` steward before landing.

## Federation-peer dimension

Changes that affect federation behavior (new activity types, changed signature algorithms, changed actor-object shape, new FEP adoption) have implicit cross-peer rollout:

- **FEP-aligned changes** — consult the FEP for adoption guidance. Typically additive; peers implementing the same FEP interoperate.
- **Mastodon-practical changes** — what does Mastodon do in this scenario? Align with Mastodon's behavior as the de-facto standard unless deviation is deliberate and FEP-supported.
- **Breaking-peer-assumptions changes** — rare. Requires explicit impact analysis and typically staged across multiple releases with a migration window.

## Risk register

- **Known unknowns** — things you know you don't know
- **Federation-trust risks** — for signature / verification / delivery changes, what's the failure mode? Who sees it first?
- **Mastodon-client risks** — for REST changes, which clients are most likely affected?
- **Schema-cascade risks** — for schema changes, every consumer reading the affected shape must be identified
- **Framework-compat risks** — does the change assume an AppTheory / TableTheory version that isn't yet released?
- **CDK / IaC risks** — stack-update failures mid-deploy, cross-stack reference changes, GSI additions (which require DynamoDB capacity planning)
- **Bootstrap-state risks** — does the change affect first-deploy bootstrap? Existing deployments' bootstrap mnemonics?
- **Async-processor risks** — DynamoDB Streams shape changes, SQS message shape changes, DLQ impact
- **Operator-facing risks** — does the change require operator action (config update, manual migration)? Is it documented?
- **AGPL-adjacent risks** — does any item introduce a dependency that requires re-vetting license posture?
- **Advisor-brief risks** — for advisor-dispatched roadmaps, is Aron's authorization scoped to the full roadmap or just the initial brief?
- **Rollback risks** — schema changes that can't be cleanly undone by Lambda-version revert alone

A risk with no mitigation is a blocker. Call it out; do not proceed.

## Output format

```markdown
# Roadmap: <scoped-need name>

## Goal
<one paragraph — what the full roadmap delivers and why>

## Classification
<security / federation-trust / contract-stability / schema / operational-reliability / AGPL / framework-feedback / bug-fix / test-coverage / dependency-maintenance / docs>

## Surfaces affected
<enumerated from the change list>

## Sibling-repo coordination
- body: <required / not required, what>
- soul: <required / not required, what>
- host: <required / not required, what>
- greater: <required / not required, what>
- sim: <required / not required, what>

## Framework coordination
- AppTheory: <required / not required, what>
- TableTheory: <required / not required, what>
- Other: <...>

## Federation / ecosystem coordination
- Mastodon clients: <no impact / documented in release notes / specific coordination>
- Remote AP peers: <no impact / FEP-aligned / explicit migration guidance>

## Phases

### Phase 1: <name>
- Items: <enumerated item numbers>
- Dependencies: <what must land first>
- Risks: <bullet list>

### Phase 2: <name>
...

## Stage rollout plan

### Dev
- Command: `./lesser up --stage dev`
- Soak duration: <...>
- Soak criteria: <observable evidence required>

### Staging (where used)
- Command: `./lesser up --stage staging`
- Soak duration: <...>
- Soak criteria: <...>

### Live
- Command: `./lesser up --stage live`
- Authorization: <operator-run; release notes + migration guidance>
- Post-deploy monitoring plan: <...>

## Release artifact plan
- GitHub Release: <version tag>
- Release notes: <breaking changes, migration guidance, federation-behavior changes>
- Managed-consumer (lesser-host) impact: <none / coordinated with host steward>

## Rollback plan
- Lambda-version rollback: <prior version>
- CDK stack rollback: <revert commit + redeploy>
- Schema rollback: <straightforward / requires data-migration>
- Federation-state rollback: <not recallable — what local view recovery looks like>

## AGPL posture
- No proprietary blobs added: <confirmed>
- Dependency license vetting: <completed if applicable>

## Advisor-brief authorization (if applicable)
- Brief source: <advisor identity, email provenance>
- Aron's authorization: <scope, date, notes>

## Open questions
<unresolved>
```

## Persist

Append only if the roadmap exposes a recurring risk pattern, a rollout subtlety worth remembering, a coordination pattern (sibling / framework / ecosystem) that will recur, or a release-artifact detail that affects managed consumers. Routine roadmaps aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff

- If approved, invoke `create-github-project` (or proceed informally if the team doesn't track this roadmap in a GitHub Project).
- If the rollout plan surfaces coordination not yet happening (sibling steward not consulted, framework steward not consulted, Mastodon ecosystem not notified), pause and surface first.
- If the roadmap reveals scope growth, revisit `scope-need`.
- If the roadmap is a security / federation-trust response requiring compressed cadence, ensure authorization is explicit.
