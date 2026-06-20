---
name: scope-need
description: Use when a user brings a new capability, feature request, or enhancement need in vague terms. Interviews conversationally and produces a scoped-need document. Applies Gate 1 (federation/API/schema/AGPL-mission alignment), Gate 2 (narrowest scope), and Gate 3 (specialist routing) before producing output.
---

# Scope a need

A need arrives fuzzy. A feature arrives sharp. This skill is the conversation that turns fuzzy into sharp, with three specific filters: federation-mission alignment, narrowest-scope discipline, and specialist-skill routing.

## Your posture

You are interviewing, not pitching. lesser is the flagship open-source ActivityPub platform of the equaltoai ecosystem and the canonical consumer of Theory Cloud frameworks. The scoping question is always three-part:

1. **Is this federation-platform work, Mastodon-compat work, schema-rigor work, operator-reliability work, AGPL-discipline work, or framework-feedback work — or is it scope growth outside the mission?**
2. **If it's in-mission, what is the narrowest possible scope that preserves federation trust, API contract, schema, AGPL coverage, and idiomatic framework consumption?**
3. **Does the change touch federation, the API, the schema, AGPL posture, framework consumption, a sibling repo, or an advisor-dispatched brief? If yes, route to the appropriate specialist skill before enumeration.**

The default answer to Gate 1 for security, federation-trust, Mastodon-compat, schema-correctness, operator-reliability, AGPL-compliance, and framework-feedback work is "yes, evaluate at Gate 2." The default answer for net-new capability outside the mission is "no."

## Start with memory and the architecture

- **Read `AGENTS.md`, `README.md`, and `docs/`** for the canonical architecture and contracts before proposing scope.
- `memory_recent` — has this need or adjacent work been scoped before? Federation-platform scoping often repeats.
- `query_knowledge` — do Theory Cloud frameworks, sibling equaltoai repos, or the broader ActivityPub ecosystem already cover this concept?

If tools are unavailable, surface it and ask the principal to re-auth.

## The interview

Ask, one or two at a time:

1. **Who is asking and why now?** Operator report, Mastodon-client compatibility issue, CVE response, FEP adoption, sibling-repo coordination, Theory Cloud framework evolution signal, advisor-dispatched brief, or principal-direct scope?
2. **What problem does it solve?** Current pain in production, not speculative improvement.
3. **Which surface does it touch?** Mastodon REST (`/api/v1/*`), GraphQL, ActivityPub (inbox / outbox / actor / WebFinger / delivery / signatures), async processors, CLI (`lesser up` / `lesser verify` / schema regen), DynamoDB schema, CDK infrastructure, AGPL / licensing, framework consumption pattern, sibling-repo contract (body / soul / host / greater / sim).
4. **Which consumers are affected?** Operators running instances, Mastodon clients, remote ActivityPub peers, sibling equaltoai repos, Theory Cloud framework stewards, direct API integrators.
5. **Is this federation-trust work?** Signature, verification, delivery, moderation, governance state.
6. **Is this contract-changing?** Mastodon-compat, GraphQL schema, ActivityPub actor object shape, JSON-LD context shape, OpenAPI spec.
7. **Is this schema-changing?** PK / SK / GSI / TableTheory tag / optimistic concurrency.
8. **Is this AGPL-adjacent?** Proprietary-code introduction, license incompatibility, release-posture change.
9. **Is this framework-awkward?** AppTheory, TableTheory, CDK construct patterns. If yes, probable upstream scoping conversation.
10. **Is this growth or preservation?** Both are welcome; shape differs.
11. **What does success look like?** Observable, testable. For federation work, "this remote peer can successfully follow this local actor"; for Mastodon-compat, "this client renders the response correctly"; for schema, "this read path continues to work after migration."
12. **What is explicitly out of scope?**

## The three gating questions

### Gate 1: Is this lesser-mission work?

Six possible verdicts:

1. **Yes — security / federation-trust work.** CVE response, signature-verification bug, delivery-integrity fix, governance-state correctness. Always accepted. Proceed to Gate 2. Route through `protect-federation-trust`.
2. **Yes — contract-stability work.** Mastodon-compat fix, ActivityPub actor-shape fix, GraphQL schema refinement that preserves compatibility, OpenAPI accuracy. Accepted. Proceed to Gate 2. Route through `preserve-mastodon-api-compat`.
3. **Yes — schema / data-integrity work.** PK/SK discipline, GSI correctness, TableTheory tag fix, optimistic-concurrency correctness. Accepted. Proceed to Gate 2. Route through `validate-schema`.
4. **Yes — operational-reliability or AGPL-discipline work.** Latency fix, availability fix, observability addition for specific observed gap, rate-limiting refinement, moderation tooling, AGPL compliance patch, dependency vetting. Proceed to Gate 2.
5. **Yes — framework-feedback work.** A lesser concern surfaces framework awkwardness; the scoped output is a signal back to the framework steward, not a local change here. Proceed to `coordinate-framework-feedback`.
6. **No — out-of-scope growth.** Transaction routing, payments processing, identity issuance, general-purpose SaaS capability, proprietary extensions. Produces a redirect document naming the appropriate home (sibling equaltoai repo, Theory Cloud framework, separate new repo, or refusal).

### Gate 2: What is the narrowest possible scope?

If Gate 1 passed, the next question is how to deliver the need with the smallest possible change.

Prefer:

- Bug fixes scoped to the specific reported symptom, not the area around it
- Additive API changes (new optional fields, new endpoints alongside old) over breaking changes
- Federation work that follows existing FEPs or established Mastodon behavior rather than lesser-unique inventions
- Schema additions (new SK patterns for new entity types) over schema modifications
- Handler-level changes that preserve the AppTheory middleware chain shape
- TableTheory-idiomatic queries over raw DynamoDB access
- Dependency bumps within current major versions
- Moderation-tooling refinements that preserve existing operator workflows

Avoid:

- Refactors "while we're in there"
- New entity types when an existing SK pattern would serve
- New Lambdas when an existing function's handler set could be extended
- New GSIs when an existing one could serve (GSI additions require DynamoDB capacity planning)
- Bypassing TableTheory or AppTheory for local convenience
- Net-new federation surfaces that don't track an FEP or Mastodon precedent
- Introducing dependencies with AGPL-incompatible licenses
- Adding proprietary blobs or "source-available" code

### Gate 3: Specialist routing

If the change touches any of these, the specialist skill runs before enumeration:

- **Federation trust** (signatures, delivery, inbox verification, moderation gates, governance state) → `protect-federation-trust`
- **Mastodon / GraphQL / ActivityPub actor contract** → `preserve-mastodon-api-compat`
- **Schema / PK-SK / GSI / TableTheory** → `validate-schema`
- **Framework awkwardness** (AppTheory, TableTheory, CDK) → `coordinate-framework-feedback`
- **Deploy / CLI / stack / bootstrap mnemonic** → `deploy-instance`
- **Advisor-dispatched brief** (email ending `@lessersoul.ai` with provenance signature) → `review-advisor-brief`

Specialist findings feed back into enumeration. Skipping them is scope shortcut that routinely becomes expense later.

## Output: the scoped-need document

### For Gate 1 verdict "lesser-mission work":

```markdown
# Scoped Need: <short name>

## Background
<one paragraph of context>

## Driver
<operator / Mastodon-client / CVE / FEP / sibling-repo / framework-feedback / principal-direct / advisor-dispatched>

## Problem
<what is broken, missing, or painful today>

## Surface affected
<REST / GraphQL / WebSocket / ActivityPub inbox/outbox/actor / WebFinger / async processor / CLI / DynamoDB / CDK / docs>

## Lambda(s) affected
<enumerated, or "none — infrastructure only">

## Classification
<security / federation-trust / contract-stability / schema / operational-reliability / AGPL / framework-feedback / bug-fix / test-coverage / dependency-maintenance / docs>

## Narrowest-scope proposal
<the smallest possible change that addresses the need>

## What this need explicitly does not cover
<bounded scope; avoid scope creep>

## Success criteria
<observable, testable conditions>

## Specialist routing
- Federation trust: <not touched / walk required via protect-federation-trust>
- API / contract: <not touched / walk required via preserve-mastodon-api-compat>
- Schema: <not touched / walk required via validate-schema>
- Framework consumption: <idiomatic / awkwardness reported via coordinate-framework-feedback>
- Deploy / CLI: <not touched / walk required via deploy-instance>
- Advisor brief: <n/a / review required via review-advisor-brief>

## Consumer impact
<operators / Mastodon clients / remote AP peers / sibling repos / framework stewards>

## AGPL posture
<no change / confirmed AGPL-compatible / decision required>

## Open questions
<unresolved>
```

### For Gate 1 verdict "out-of-scope growth":

```markdown
# Redirect: <short name>

## Background
<one paragraph — what was asked>

## Why this doesn't belong in lesser
<scope is bounded to ActivityPub federation platform concerns; this is X, which belongs in Y>

## Appropriate owner
<sibling repo (body / soul / host / greater / sim) / Theory Cloud framework steward / separate new repo / scoping conversation with the principal>

## Path for the requesting user
<rough outline — escalate to steward Y, defer, start a new repo, etc.>

## Recommended next step
<specific handoff>
```

## Persist before handoff

Append only if the scoping conversation surfaces a recurring pattern — a redirect category that keeps coming up, an AGPL discipline finding, a framework awkwardness worth remembering, a Mastodon-compat constraint that resurfaces, a consumer-coordination signal. Routine scope completions aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff

- **In-mission, federation-trust** — invoke `protect-federation-trust` before enumeration.
- **In-mission, contract-stability** — invoke `preserve-mastodon-api-compat` before enumeration.
- **In-mission, schema-touching** — invoke `validate-schema` before enumeration.
- **In-mission, framework-feedback** — invoke `coordinate-framework-feedback`; the output may be an upstream report rather than local enumeration.
- **In-mission, deploy-adjacent** — invoke `deploy-instance` for the CLI / stack discipline.
- **Advisor-dispatched scope** — `review-advisor-brief` already ran; this skill's output includes the principal's authorization from that review.
- **In-mission, none of the above** — invoke `enumerate-changes` directly.
- **Out-of-scope** — redirect document *is* the handoff.
- **Scope resolved to "no change needed"** — record and stop.
- **User defers** — record and stop.