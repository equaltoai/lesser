---
name: preserve-mastodon-api-compat
description: Use when a change affects an observable contract surface — Mastodon REST API (/api/v1/*), GraphQL schema, ActivityPub actor object shape, JSON-LD context, OpenAPI spec, or error response format. Walks the contract impact across every consumer class and produces a coordination plan. Contract breaks require named coordination before landing.
---

# Preserve Mastodon API compatibility

lesser's contract surfaces are consumed by parties you cannot directly coordinate with — mobile apps, browser clients, third-party tooling, remote ActivityPub servers, sibling equaltoai repos, and operator-maintained integrations. Silent contract changes strand consumers; coordinated changes preserve the ecosystem relationship. This skill is the walk that turns "the contract is changing" into "the coordination plan that makes it safe."

## The contract surfaces (memorize)

- **Mastodon REST API** under `/api/v1/*` — authoritative spec in `docs/contracts/openapi.yaml`. Clients: Mastodon mobile apps (Tusky, Mastonaut, Ivory, etc.), browser clients, third-party tools.
- **GraphQL schema** — composed from `graph/core.graphql` + `graph/phase1.graphql` + `graph/phase2.graphql` + `graph/phase3.graphql`. Regenerated into `docs/contracts/graphql-schema.graphql` via `./lesser schema`. Clients: lesser's own UIs (including greater-components consumers and simulacrum), potentially other equaltoai-direct integrations.
- **WebSocket / SSE streaming** — `/api/v1/streaming` (Mastodon-compat), GraphQL subscriptions via `graphql-ws`. Clients: real-time UIs.
- **ActivityPub actor object** — `GET /users/{username}` (with `Accept: application/activity+json`). Clients: every remote ActivityPub server in the Fediverse. Shape includes `inbox`, `outbox`, `followers`, `following`, `publicKey`, `endpoints`, `preferredUsername`, `delegated_by` (soul-integration), and other standard fields.
- **ActivityPub objects / collections** — `GET /objects/...`, `GET /collections/...`. Remote servers fetch these when resolving references in activities.
- **WebFinger** — `GET /.well-known/webfinger?resource=acct:user@domain`. Discovery.
- **NodeInfo** — `GET /.well-known/nodeinfo`. Instance software / usage reporting.
- **Error response shapes** — Mastodon has conventions; deviation confuses clients.

## When this skill runs

Invoke this skill when:

- A change modifies an HTTP response shape (fields, types, nesting, pagination envelope)
- A change modifies an HTTP request shape (new required parameter, removed parameter, changed semantics)
- A change modifies error-response shape or error codes
- A change modifies an endpoint signature (URL, method, status code semantics)
- A change modifies the GraphQL schema (type / field / argument / directive changes)
- A change modifies the ActivityPub actor object shape or JSON-LD context
- A change modifies WebFinger / NodeInfo output
- A change adds, removes, or renames an endpoint
- A change modifies streaming / subscription envelope shape
- `scope-need` flags a change as contract-affecting

## Preconditions

- **The change is described concretely.** "Improve the status response" is too vague; "add optional `edited_at` field to `GET /api/v1/statuses/:id`, non-null when the status has been edited at least once, backward-compatible for clients that ignore unknown fields" is concrete.
- **MCP tools healthy**, `memory_recent` first — prior contract decisions are high-signal for consistency.
- **The surfaces affected are identified.**

## The four-dimension walk

### Dimension 1: Compatibility classification

Classify every contract change:

- **Additive / backward-compatible** — new optional field in response, new optional request parameter, new endpoint alongside existing, new optional error code, additive GraphQL type field. Clients continue to work unchanged.
- **Semantic refinement** — same shape, more precise behavior (tighter validation, stricter error conditions for previously-accepted inputs). Clients relying on lax behavior may break; clients using the contract as documented continue to work.
- **Additive but observable** — new required request parameter, new required response field that breaks old clients' validation, new mandatory error code. Clients must eventually update.
- **Breaking** — removed field, renamed field, changed type, changed semantics for same input, removed endpoint, changed error code for same condition, changed ActivityPub actor shape, changed GraphQL schema (removed type / field / argument, non-nullable → nullable), changed JSON-LD context.

Breaking changes require:

- Consumer-class enumeration by name
- Coordination plan per consumer class (who, when, what they change)
- Versioning strategy (new endpoint alongside old, deprecation notice, migration window)
- Rollout plan that lets consumers validate before cutover

### Dimension 2: Consumer-class enumeration

For each consumer class, answer:

- **Which surface does this class consume?** REST, GraphQL, WebSocket / streaming, ActivityPub actor / object / collection, WebFinger, NodeInfo.
- **Is this class affected by the change?** Based on compatibility classification.
- **How is the class reachable for coordination?**
  - **Mastodon mobile/desktop clients** — via ecosystem channels (Mastodon dev community, client-maintainer outreach). lesser's operators don't control these clients.
  - **Third-party tooling / bots** — via operator advisories in release notes.
  - **Remote ActivityPub servers** — not directly reachable; use FEPs or Mastodon-de-facto standards as coordination mechanism.
  - **Sibling equaltoai repos** — via the repo's steward (body / soul / host / greater / sim).
  - **Direct API integrators** — via operator or ecosystem channels.
  - **lesser's own UIs** (including greater-components-based) — via the `greater` steward for component updates.
- **What's the class's test posture?** Integration tests against lesser's API? Reliance on Mastodon compatibility?

Enumerate explicitly. "All consumers will adapt" is not a plan.

### Dimension 3: Versioning and transition

For non-trivial changes:

- **Can old and new coexist?** Adding a new optional field is trivial; replacing endpoint semantics requires a path where both work simultaneously.
- **What's the transition mechanism?**
  - **Additive field** — old clients ignore new field; new clients use it. The default for most additive changes.
  - **New endpoint alongside old** — `/api/v2/...` alongside `/api/v1/...` for substantial breaking changes. Mastodon itself uses this pattern.
  - **Feature-flagged behavior** — new behavior conditional on a setting or version header. Fragile; avoid unless necessary.
  - **Dual-format GraphQL** — deprecated field preserved alongside new one; schema regeneration reflects deprecation.
- **What's the deprecation timeline?** Named conditions ("after two release cycles," "after Mastodon v5 adopts this"), not vague "eventually."
- **What's the signal that the old path can be removed?** Monitoring showing zero traffic, explicit ecosystem sign-off, or operational decision.

### Dimension 4: Rollout mechanics

- **How does each consumer class validate during rollout?** Operator deployments flow `dev → staging → live`; within those stages, client verification happens in specific test environments.
- **Does the rollout require consumer changes to deploy first?** For breaking changes where feasible. Mastodon clients typically can't deploy in lockstep with lesser operators.
- **What's the rollback signal per consumer class?** Specific monitoring metrics, client-community reports, or operator feedback.
- **Is there an SLA / behavior-change notice?** Release notes document the change, migration guidance, and any operator-action required.
- **Does the change require an OpenAPI / GraphQL-schema regeneration in the same PR?** Yes, for REST and GraphQL changes. `./lesser schema` regenerates GraphQL; OpenAPI updates ride with handler changes.

## The audit output

```markdown
## Contract audit: <change name>

### Proposed change
<concrete description>

### Surfaces affected
- Mastodon REST: <endpoints>
- GraphQL: <types / fields / arguments>
- Streaming / subscriptions: <...>
- ActivityPub actor / objects / collections: <...>
- WebFinger / NodeInfo: <...>

### Compatibility classification
<additive backward-compatible / semantic refinement / additive observable / breaking>

### Consumer-class enumeration

#### Mastodon clients (mobile, desktop, third-party tooling)
- Affected: <yes / no>
- Surface used: <specific endpoint / streaming channel>
- Expected impact: <none / clients must update X / breaking>
- Coordination channel: <ecosystem notice, release-note advisory, fedi-post from operator>
- Timeline flexibility: <...>

#### Remote ActivityPub servers (Mastodon, Pleroma, Misskey, GoToSocial, etc.)
- Affected: <yes / no>
- Surface used: <actor object, objects, collections, WebFinger, NodeInfo>
- Expected impact: <...>
- FEP alignment: <FEP-XXXX / de-facto Mastodon / deliberate deviation>

#### Sibling equaltoai repos
- body: <no impact / surface X affected / steward coordination>
- greater (UI library): <no impact / component X affected / steward coordination>
- sim: <no impact / UI workflow X affected / steward coordination>
- host: <no impact / release-artifact impact / steward coordination>
- soul: <no impact / namespace impact / steward coordination>

#### Direct API integrators / operator-maintained tooling
- Known integrations: <list via operator reports>
- Expected impact: <...>
- Release-notes advisory: <required / not required>

### Versioning strategy
- Transition mechanism: <additive / new endpoint alongside old / feature-flag / dual-format GraphQL / big-bang>
- Transition window: <expected duration>
- Deprecation signal: <named condition for removing old path>

### Rollout mechanics
- Consumer validation stages: <dev → staging → live, per consumer class>
- Rollback signal: <...>
- SLA impact: <none / latency / error-rate / behavior>
- OpenAPI regeneration: <completed>
- GraphQL schema regeneration: <completed>

### Mastodon-reference check
- Does Mastodon do this same thing? <yes — aligned / no — deliberate deviation / n/a — lesser-exclusive additive field>
- Deviation justified: <FEP alignment / lesser-exclusive feature / N/A>

### Audit-log implications
<what audit events are emitted around the contract change; retention; format>

### Release-notes content
<the specific notice operators and clients will read when the release ships>

### Proposed next skill
<enumerate-changes if clean; validate-schema if schema is also touched; protect-federation-trust if ActivityPub actor shape is touched; scope-need if audit surfaces scope growth>
```

## Refusal cases

- **"Change the response shape silently; clients will update."** Refuse breaking changes without coordination.
- **"Remove the old endpoint; nobody uses it."** Refuse without monitoring evidence of zero traffic + advisory notice.
- **"Change error response shape to match our internal convention instead of Mastodon's."** Refuse. Mastodon's error conventions are what clients code against.
- **"Skip updating `docs/contracts/openapi.yaml`; it's internal docs."** It is authoritative; updates ride with the code change.
- **"Skip regenerating `docs/contracts/graphql-schema.graphql`."** Schema file is the contract artifact; regeneration is part of the PR.
- **"Deprecate an endpoint in one release and remove in the next."** Too fast. Deprecation cycles are measured in multiple release cycles and named conditions, not single releases.
- **"Return extra data in the response; Mastodon clients will ignore it."** Check shape-sensitivity: some clients use strict schema validation. Additive changes are generally safe, but test.
- **"Change the ActivityPub actor object's inbox URL shape."** Breaks every remote peer that has cached the actor. Requires coordinated rotation with a migration window.
- **"Use a non-standard JSON-LD context URL that changes the semantic meaning of fields."** Breaks interoperability. JSON-LD context is a cross-project contract.

## Persist

Append when the audit surfaces a recurring pattern — a consumer-class discovery worth remembering, a versioning strategy that worked well, a Mastodon-reference decision that informs future changes, an FEP alignment consideration, an operator-reporting channel that's useful. Routine audits that resolve cleanly aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff

- **Audit clean, additive change** — invoke `enumerate-changes`.
- **Audit clean, breaking change with coordination plan** — coordination happens through operator channels and sibling stewards. Once sign-off is explicit, `enumerate-changes`.
- **Audit overlaps with schema walk** — ensure `validate-schema` is also complete.
- **Audit overlaps with federation-trust walk** — ensure `protect-federation-trust` is also complete (especially for actor-object changes).
- **Audit surfaces scope growth** — revisit `scope-need`.
- **Audit reveals a consumer-side bug** — report cleanly to the user; no contract change needed here.
- **Audit reveals the contract-change ask is actually framework-awkwardness** (GraphQL code generation, OpenAPI tooling) — invoke `coordinate-framework-feedback`.