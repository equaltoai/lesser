---
name: validate-schema
description: Use when a change touches the DynamoDB schema — PK/SK construction, GSI1–GSI8 key or projection, entity-type SK patterns, attribute names / types, TableTheory struct tags (`theorydb:"pk"`, `theorydb:"sk"`, `theorydb:"gsi*pk"`, `theorydb:"version"`, `theorydb:"ttl"`), or optimistic-concurrency versioning. Walks the schema impact across every entity type, every read path, every async processor, and every consumer before the change proceeds.
---

# Validate the single-table design

The single-table DynamoDB schema is lesser's most load-bearing internal contract. Every read path, every GSI lookup, every async processor, every TableTheory-tagged field, and every consumer that depends on item shape depends on the schema staying stable. A schema change that looks correct in one entity type can silently break a GSI projection, desynchronize an async processor, or cascade into Mastodon-client errors.

This skill is the walk that makes schema-touching changes safe to land.

## The schema (memorize)

**Single table**: `lesser-{stage}` (e.g. `lesser-dev`, `lesser-staging`, `lesser-live`).

**Composite primary key**: PK + SK.

**Entity-type patterns** (not exhaustive; confirm against current `pkg/storage/models/*.go`):

- **Account**: PK = `USER#{username}`, SK = `ACCOUNT`
- **AccountKeys** (encrypted signing material): PK = `USER#{username}`, SK = `ACCOUNT_KEYS`
- **AgentGovernanceState**: PK = `USER#{username}`, SK = `AGENT_GOVERNANCE`
- **Notes, activities, follows, relationships, delivery state, cost tracking, trend aggregates** — each with its own PK/SK pattern

**Global Secondary Indexes (8 total)** — enumerated in `docs/architecture/dynamodb/gsi_usage_guide.md`:

- GSI1–GSI8 serve access patterns like: user-by-username, notes-by-created-at, federation-relationships-by-domain, activities-by-actor, and others. Each GSI's PK (and often SK) attributes are set on the items that should participate.

**TableTheory struct tags** (load-bearing):

- `theorydb:"pk"` — partition key
- `theorydb:"sk"` — sort key
- `theorydb:"gsi1pk"`, `theorydb:"gsi1sk"`, ..., `theorydb:"gsi8pk"`, `theorydb:"gsi8sk"` — GSI keys
- `theorydb:"version"` — optimistic-concurrency version field
- `theorydb:"ttl"` — TTL attribute
- Other attribute tags as established in `pkg/storage/models/*.go`

## When this skill runs

Invoke this skill when:

- A change adds, modifies, or removes an entity type (new SK pattern, deprecated SK pattern)
- A change modifies PK construction
- A change modifies GSI key population — which items set which GSI attributes
- A change adds a new GSI (requires DynamoDB capacity planning + table update)
- A change removes or restructures an existing GSI
- A change modifies attribute names or types on an existing entity shape
- A new attribute is introduced that consumers will consume
- A change modifies TableTheory struct tags on any model
- A change modifies optimistic-concurrency handling (the `version` field)
- A change modifies DynamoDB Streams shape consumed by async processors
- A change modifies TTL semantics
- A consumer (sibling repo, async processor, Mastodon-client-facing handler) reports reading stale / missing / wrong-shape data traced to schema-related construction

## Preconditions

- **The change is described concretely.** "Add a new field" is too vague; "add optional `edited_at` attribute (type `string`, RFC3339 timestamp) to Note model, populated by the Update activity handler, not indexed, included in note-processor stream output" is concrete.
- **MCP tools healthy**, `memory_recent` first.
- **The affected entities are named.**

## The six-dimension walk

### Dimension 1: PK impact

- **Does PK format change?** (It should almost never change.) A PK format change invalidates every partition query and every consumer code path that constructs PKs.
- **Is `USER#{username}` semantics preserved?** Account-anchored entities (AccountKeys, AgentGovernanceState, note ownership) use `USER#{username}` as the partition anchor; changes cascade.
- **Are there items whose PK is *not* `USER#{username}`?** Yes — certain entity types have their own PK patterns (note IDs, activity IDs, federation-state keys). Confirm the change respects those.

### Dimension 2: SK impact

- **Does the change add a new SK pattern?** New entity types via new SK patterns are the preferred extension mechanism. Convention: uppercase, `#` separator for compound keys (e.g. `ACTIVITY#{id}`, `FOLLOW#{target}`).
- **Does the change modify an existing SK pattern?** High-risk. Every consumer reading that entity type depends on the SK format. Consumer coordination is mandatory.
- **Does the change remove an existing SK pattern?** Very high-risk. Items with that SK exist in the data until migrated. Consumer coordination plus data-migration planning is mandatory.
- **Is the new SK pattern consistent with established conventions?** Uppercase, `#`-separated, no lowercase. Reject inconsistent patterns.

### Dimension 3: GSI impact

- **Which GSIs does the change affect?** Enumerate by number (GSI1 through GSI8).
- **For each affected GSI**:
  - What's the existing access pattern the GSI serves? (Refer to `docs/architecture/dynamodb/gsi_usage_guide.md`.)
  - Does the change preserve that access pattern?
  - Does the change add items that should participate in the GSI? Ensure population is consistent.
  - Does the change add items that should NOT participate? Ensure they don't set the GSI key attributes.
- **Does the change add a new GSI?** Significant — requires DynamoDB capacity planning, table-update time (new GSIs backfill asynchronously), and CDK infrastructure changes. Escalate.
- **Does the change remove or restructure a GSI?** Very high-risk. Every consumer reading through that GSI breaks. Requires explicit migration.
- **Consumer impact per GSI**: enumerate which services, Lambdas, async processors, Mastodon-API endpoints, and sibling repos read through each affected GSI.

### Dimension 4: Attribute-semantics impact

- **Are new attributes introduced?** Naming convention (`snake_case` attribute names at the DynamoDB level, Go-struct `PascalCase` with TableTheory tags that map). Type stability, null semantics, default-value handling.
- **Are existing attributes renamed?** Very high-risk; consumers depend on names.
- **Are existing attribute types changed?** String → number, nullable → non-nullable, etc. Breaks deserialization. Usually requires a dual-attribute transition.
- **Do new attributes need indexing?** If yes, a new GSI is required; escalates the walk.
- **Do new attributes contain PII, credentials, or cryptographic material?** If yes, sanitization / encryption / audit-logging rules apply.
- **Does the change affect the `version` field?** Optimistic concurrency is the contract for concurrent writes. Skipping version handling regresses integrity.

### Dimension 5: DynamoDB Streams and async-processor impact

- **Which async processors consume the DynamoDB Stream?** `note-processor`, `activity-processor`, `ai-processor`, `moderation-processor`, `media-processor`, `cost-aggregator`, `federation-aggregator`, `trend-aggregator`, and others.
- **Does the change affect the stream event shape for any of these processors?** New attributes are visible in stream events (INSERT / MODIFY / REMOVE); consumers must tolerate them.
- **Does the change add items that should trigger a processor that isn't currently aware of them?** Processor logic may need updating.
- **Does the change remove items that a processor is currently handling?** Removal events flow through the stream; processors should handle REMOVE events correctly.
- **TTL-triggered removal** — if TTL is involved, the REMOVE event carries the TTL flag. Processors distinguishing user-delete from TTL-delete must be updated.

### Dimension 6: Sync / consumer propagation

- **Which Mastodon-API endpoints read this shape?** List the handlers in `cmd/api/handlers/` and `pkg/services/`.
- **Which GraphQL resolvers read this shape?**
- **Which ActivityPub endpoints (actor, object, collection) read this shape?**
- **Which sibling repos read through the data table?**
  - `body` (lesser-body) reads from lesser's DynamoDB table; MCP tool registration may depend on specific attribute shapes. Coordinate with the `body` steward.
  - `host` (lesser-host) reads through lesser's trust API (not direct DynamoDB); changes that affect API-exposed shapes coordinate with the `host` steward.
- **Backward compatibility during the transition**: can each consumer continue to work unchanged during the schema change window? If yes, the change is additive. If no, each consumer has a migration plan.

## The audit output

```markdown
## Schema-integrity audit: <change name>

### Proposed change
<concrete description>

### Entity types affected
<Account / AccountKeys / AgentGovernanceState / Note / Activity / Follow / cross-cutting>

### PK impact
- Format changed: <no — default; yes — justification + cascade plan>
- `USER#{username}` semantics preserved: <yes / changed>

### SK impact
- New patterns: <list>
- Modified patterns: <list, with consumer impact>
- Removed patterns: <list, with data-migration plan>
- Convention compliance: <uppercase, `#`-separated — confirmed / deviation justified>

### GSI impact
- GSIs touched: <GSI1 / GSI2 / ... / GSI8>
- For each touched GSI:
  - Access pattern preserved: <yes / no — consumer impact>
  - Population rule change: <no / yes — describe>
  - Consumer impact: <enumerated by service / Lambda / sibling repo>
- New GSI added: <no — default; yes — escalation to capacity planning + CDK + rollout>
- GSI removed / restructured: <no — default; yes — migration plan>

### Attribute-semantics impact
- New attributes: <name, type, nullability, default, PII/credential status>
- Renamed attributes: <old → new, consumer-migration plan>
- Type changes: <old → new, transition strategy>
- New attributes requiring indexing: <none / escalation to new-GSI walk>
- `version` field handling: <preserved / changed>

### DynamoDB Streams / async-processor impact
- Stream-event shape changed: <no / additive / breaking>
- Processors affected: <note-processor / activity-processor / ai-processor / moderation-processor / media-processor / cost-aggregator / federation-aggregator / trend-aggregator / other>
- TTL handling affected: <no / yes — impact>
- Processor-side updates required: <none / enumerated>

### Consumer propagation
- Mastodon REST handlers: <no impact / path X affected / coordination plan>
- GraphQL resolvers: <no impact / resolver X affected>
- ActivityPub endpoints: <no impact / endpoint X affected>
- Sibling repos:
  - body: <no impact / coordination via body steward>
  - host: <no impact / coordination via host steward>
- Backward compatibility during transition: <yes / no — migration plan>
- Rollback story: <straightforward / requires data-migration>

### TableTheory tag correctness
- Tags match intended semantics: <verified>
- Tag-based test coverage: <added / existing — tests validate PK/SK/GSI construction for the new shape>

### Security / PII / credential implications
<assessed — new PII handling, new credential, new cryptographic material>

### Proposed next skill
<enumerate-changes if audit clean; preserve-mastodon-api-compat if consumer-contract coordination required; protect-federation-trust if ActivityPub shape affected; scope-need if audit surfaces scope growth; coordinate-framework-feedback if TableTheory awkwardness surfaces>
```

## Refusal cases

- **"Change PK from `USER#{username}` to just `{username}`."** Refuse. Cascades across every partition query.
- **"Drop GSI5; we're not using it."** Refuse without explicit enumeration of every service / Lambda / sibling-repo read that goes through GSI5, plus evidence of zero traffic and sign-off.
- **"Rename `version` to `ver` for brevity."** Refuse. Tag rename cascades silently.
- **"Skip the `version` field on a new model; we'll add optimistic concurrency later."** Refuse. Optimistic concurrency is on-from-day-one.
- **"Change `created_at` from RFC3339 string to Unix epoch number."** Refuse without a full dual-attribute transition.
- **"Store signed activity payloads in a new attribute without encryption."** Refuse for credential-adjacent data; apply encryption discipline.
- **"Add a new SK pattern `activity#follow` (lowercase)."** Refuse. Convention is uppercase.
- **"Skip updating async processors; they'll handle extra attributes fine."** Processors should tolerate, but test. Schema changes that add attributes to processor-consumed shapes benefit from processor-side test coverage.
- **"Introduce a second DynamoDB table for this new entity type."** Refuse unless the new entity genuinely doesn't fit — rare.
- **"Rename an attribute in TableTheory tag without updating the downstream read path."** Refuse. Tag renames cascade into query construction.

## Persist

Append when the audit surfaces a recurring pattern — a tag-interaction subtlety, a GSI population edge case, a processor-shape consumption detail, an optimistic-concurrency conflict pattern, a backward-compatibility technique. Routine audits that resolve cleanly aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff

- **Audit clean, backward-compatible** — invoke `enumerate-changes`.
- **Audit clean, contract-affecting** — invoke `preserve-mastodon-api-compat` before enumeration.
- **Audit touches ActivityPub actor shape** — invoke `protect-federation-trust` as well.
- **Audit surfaces TableTheory awkwardness** — invoke `coordinate-framework-feedback`.
- **Audit surfaces scope growth** — revisit `scope-need`.
- **Audit reveals an existing schema bug** — route through `investigate-issue`.
- **Audit reveals a new GSI is needed** — escalate: requires DynamoDB capacity planning, CDK changes, and a distinct rollout plan.
- **Audit reveals sibling-repo impact** — report cross-repo for the relevant steward.