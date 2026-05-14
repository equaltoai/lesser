# Canonical skill authority foundation

Status: Project 21 M3 canonical authority (#698/#699/#700/#701/#702), with M4.1
catalog/bundle publication defined in
[`catalog-bundle-publication.md`](catalog-bundle-publication.md).

## Purpose

M3 establishes **Lesser as the canonical skill authority**. Repo-backed `SKILL.md`
files and lesser-host mint conversations may be used as source material or
provenance, but neither is the system of record. The canonical authority lives in
Lesser's single DynamoDB table and exposes approval, exposure, effective-resolution,
proposal-promotion, approved catalog, and bundle-publication APIs from that state.

M4.1 adds the approved catalog and bundle publication contract; lesser-body MCP
tool implementation and local install-state verification remain downstream work.

## Entity model

| Entity | Role | Primary key |
|---|---|---|
| `Skill` | Canonical skill root metadata and current revision pointer. | `PK=SKILL#{skillID}`, `SK=SKILL` |
| `SkillRevision` | Canonical revision content/manifest/provenance under a skill. | `PK=SKILL#{skillID}`, `SK=REVISION#{revisionNumber:08d}` |
| `SkillProposal` | Non-authoritative source material proposed for canonicalization. | `PK=SKILL_PROPOSAL#{proposalID}`, `SK=PROPOSAL` |
| `SkillAssignment` | Explicit subject boundary for future effective-skill resolution. | `PK=SKILL_ASSIGNMENT#{subjectType}#{subjectID}`, `SK=SKILL#{skillID}#ASSIGNMENT#{assignmentID}` |

The model keeps revision and assignment boundaries separate:

- A `Skill` names the canonical concept and points at a current revision when one exists.
- A `SkillRevision` stores digestable revision material. It can reference a proposal,
  principal approval, and provenance without making the proposal or source canonical.
- A `SkillProposal` represents import/source material from local files, lesser-host
  conversations, or manual drafting. It is never authoritative by itself.
  Accepted proposals may be promoted into canonical `SkillRevision` rows; the
  proposal then records the resulting promoted revision ID/number, promotion
  digest, promoting actor, and promotion timestamp.
- A `SkillAssignment` binds a skill/revision to a subject (`instance`, `actor`, or
  `principal`) with exposure and lifecycle state for later resolution APIs.

## Exposure, approval, and provenance hooks

The foundation records hooks for later milestones without implementing their APIs:

- exposure: `public`, `instance`, `private`
- revision/proposal/assignment status fields
- explicit revision approval semantics:
  - `approvalID`
  - `approvalAuthorityType` / `approvalAuthorityID`
  - `approvalDigest` and optional `approvalSignature` / `approvalRef`
  - `approvedBy` / `approvedAt`
  - `principalID` / `principalApprovalID`
  - `revokedBy` / `revokedAt` / `revokedReason`
- source fields for `local_file`, `host_conversation`, and `manual`
- manifest/content/bundle digests and file-level digests
- conversation IDs as provenance only; lesser-host remains non-canonical

Promotion from conversation output is fail-closed:

- the source `SkillProposal` must already be `accepted`;
- `proposedManifestJSON` must be present, valid JSON object material and its
  canonical digest must match `proposedManifestDigest` and any caller-supplied
  expected digest;
- optional source digest expectations must match the proposal's source digest;
- the resulting `SkillRevision` is written as an approved canonical row using the
  same approval/principal digest semantics as direct revision approval;
- idempotent replays return the existing revision when proposal, revision,
  manifest digest, approval digest, and promotion digest match;
- conflicting existing revisions or conflicting promoted proposal metadata return
  a conflict rather than silently rewriting authority;
- promotion updates the skill's current revision pointer but does not create any
  `SkillAssignment` rows and does not broaden exposure beyond the proposal's
  accepted requested exposure.

## Schema-integrity audit

### Proposed change

Add canonical skill authority records (`Skill`, `SkillRevision`, `SkillProposal`,
`SkillAssignment`) to the existing single DynamoDB table using TableTheory model
tags and existing generic GSI slots.

### Entity types affected

Only new skill authority entity types are added. Existing account, actor,
federation, ActivityPub object, notification, conversation, and governance item
shapes are unchanged.

### PK impact

- Existing PK formats: unchanged.
- `USER#{username}` semantics: unchanged.
- New non-user partitions:
  - `SKILL#{skillID}`
  - `SKILL_PROPOSAL#{proposalID}`
  - `SKILL_ASSIGNMENT#{subjectType}#{subjectID}`

### SK impact

- New SK patterns:
  - `SKILL`
  - `REVISION#{revisionNumber:08d}`
  - `PROPOSAL`
  - `SKILL#{skillID}#ASSIGNMENT#{assignmentID}`
- Existing SK patterns: unchanged.
- Removed SK patterns: none.
- Convention compliance: uppercase prefixes with `#` separators.

### GSI impact

No new physical GSI is added and no existing GSI is restructured. The models use
sparse prefixes on existing `gsi1`/`gsi2`:

| Model | GSI | Pattern | Purpose |
|---|---|---|---|
| `Skill` | `gsi1` | `SKILL#STATUS#{status}` / `UPDATED#{time}#SKILL#{skillID}` | later lifecycle/catalog listing |
| `Skill` | `gsi2` | `SKILL#EXPOSURE#{exposure}` / `NAME#{slug}#SKILL#{skillID}` | later exposure-scoped catalog listing |
| `SkillRevision` | `gsi1` | `SKILL_REVISION#STATUS#{status}` / `UPDATED#{time}#SKILL#{skillID}#REVISION#{n}` | approval/revision queues and approved catalog publication |
| `SkillRevision` | `gsi2` | `SKILL_REVISION_DIGEST#{digest}` / `SKILL#{skillID}#REVISION#{n}` | manifest digest de-duplication |
| `SkillProposal` | `gsi1` | `SKILL#{skillID}#PROPOSAL` / `STATUS#{status}#CREATED#{time}#PROPOSAL#{proposalID}` | proposals for a skill |
| `SkillProposal` | `gsi2` | `SKILL_PROPOSAL#STATUS#{status}` / `CREATED#{time}#SKILL#{skillID}#PROPOSAL#{proposalID}` | later review queues |
| `SkillAssignment` | `gsi1` | `SKILL#{skillID}#ASSIGNMENT` / `SUBJECT#{type}#{id}#ASSIGNMENT#{assignmentID}` | assignment audit/revocation by skill |
| `SkillAssignment` | `gsi2` | `SKILL_ASSIGNMENT#STATUS#{status}` / `UPDATED#{time}#SUBJECT#{type}#{id}#SKILL#{skillID}#REVISION#{n}#ASSIGNMENT#{assignmentID}` | pending/revoked queues |

### Approval / provenance semantics added for #701

Approved `SkillRevision` rows are fail-closed:

- approved revisions require a local approval actor (`approvedBy`), timestamp
  (`approvedAt`), approval ID, principal ID, authority type/ID, and approval
  digest;
- optional signatures or external approval references are metadata over that
  digest, not a substitute for the Lesser-owned canonical row;
- revoked revisions require `revokedBy` and `revokedAt`;
- unknown status, exposure, source, subject, or approval-authority vocabularies
  are rejected before writes so GSI prefixes cannot fragment silently.

The canonical approval digest is computed over the Lesser-owned revision identity
(`skillID`, revision number, revision ID), source/proposal references, content
digests, revision default exposure, principal ID, and approval authority. That
keeps seed/import source, proposal provenance, approval actor, exposure boundary,
and canonical revision identity inspectable without making a local `SKILL.md`
file or lesser-host conversation authoritative.

Consumer impact is additive: no existing repository, Lambda, GraphQL resolver,
Mastodon REST handler, or ActivityPub endpoint reads these prefixes today.

### Attribute-semantics impact

New attributes are additive and scoped to the new item types. They include IDs,
status/exposure fields, manifest and bundle digest strings, file summaries,
capability strings, provenance references, optional approval references, and
created/updated timestamps. They do not contain credentials, actor signing keys,
JWTs, private wallet keys, raw passwords, or private conversation bodies.

Each mutable model includes `Version int` tagged as `theorydb:"version,attr:version"`
for optimistic concurrency from day one.

### DynamoDB Streams / async processors

The stream shape is additive because new item types will appear as ordinary
INSERT/MODIFY/REMOVE records. Current async processors do not consume skill item
prefixes, and no processor update is required in this slice. Later approval,
promotion, or bundle-publication processors must explicitly opt into these
prefixes rather than relying on generic stream scans.

TTL is not used for canonical skill authority records.

### Consumer propagation

- Mastodon REST: no impact.
- GraphQL: no schema or resolver change in this slice.
- ActivityPub: no actor/object/collection/inbox/outbox shape change.
- `lesser-body`: later catalog/tooling work will consume approved Lesser skill
  state, but this slice introduces no runtime MCP contract.
- `lesser-host`: host conversation IDs may be recorded as provenance only; host
  is not canonical.
- Rollback: code rollback leaves additive skill rows inert. No migration of
  existing rows is required.

### TableTheory tag correctness

The new models use `theorydb` tags for PK/SK, sparse GSI fields with `omitempty`,
attribute names, and version fields. Model tests cover key construction,
normalization, sparse digest indexing, and assignment boundaries.

### Security / PII / credential implications

The schema stores metadata, digests, provenance IDs, approval references, and
conversation/message identifiers. It deliberately avoids storing private
conversation content, secrets, signing material, OAuth/JWT bearer tokens, or raw
credentials. Any later API exposing this state must apply the public/instance/private
exposure policy and principal approval semantics defined by #700/#701.
