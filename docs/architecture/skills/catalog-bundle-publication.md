# Skill catalog and bundle publication contract

Status: Project 21 M4.1 (`lesser#703`).

## Purpose

Lesser publishes approved canonical `SkillRevision` rows as catalog entries and
verifiable bundle descriptions. This contract lets downstream MCP clients such as
`lesser-body` fetch approved skill metadata, verify digests, and decide how to
install files locally without pretending Lesser writes into a client workspace.

Only Lesser-owned canonical state is authoritative:

- `Skill` describes the canonical skill root.
- `SkillRevision` describes approved revision material, files, digests, approval,
  principal metadata, and provenance.
- `SkillProposal`, local `SKILL.md` files, and lesser-host conversations remain
  provenance only. They never publish a bundle directly.

## REST surface

The M4.1 publication API is Lesser-exclusive and additive under `/api/v1/skills`:

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET /api/v1/skills/catalog` | optional bearer | Lists visible approved catalog entries. Query: `exposure`, `limit`, `cursor`. |
| `GET /api/v1/skills/{skillId}/revisions/{revisionNumber}/bundle` | optional bearer | Resolves one approved revision bundle. Query: `include_content=true` includes inline content only when it is present in the approved manifest. |

Both endpoints apply the same exposure boundaries as the M3 skill authority APIs:

- anonymous callers see approved `public` revisions only;
- authenticated local callers see approved `public` and `instance` revisions;
- admins can inspect private skill state elsewhere, but bundle publication still
  requires `SkillRevision.status == approved`.

Unapproved, revoked, superseded, missing, or malformed bundles fail closed: they
are omitted from catalog listing or returned as not found / invalid state on
direct bundle fetch.

## Bundle shape

Bundles use `schema_version = "lesser.skill.bundle.v1"`.

Each bundle includes:

- stable `bundle_id` (`skill:<skillID>:revision:<zero-padded revision number>`);
- `digests.manifest_digest`, `digests.bundle_digest`,
  `digests.publication_digest`, `digests.content_digest`, and
  `digests.approval_digest` when available;
- `files[]` with path, file digest, content type, role, size, and advisory
  install path;
- `install_hints` with layout, runtime targets, directory name, entrypoint, and
  required files;
- proposal/source/provenance references copied from the approved revision;
- approval authority, principal approval, approver, and approval reference
  metadata needed for trust display and verification.

When a revision has no stored `bundleDigest`, Lesser computes a stable bundle
digest from the approved revision identity, content/manifest/approval digests,
file metadata, and install hints. `publication_digest` is computed over the
published contract shape so downstream clients can detect contract-shape drift
separately from file-content drift.

## Manifest and inline content

The approved revision may carry `ManifestJSON`. M4.1 understands these optional
manifest fields:

```json
{
  "runtime_targets": ["codex", "generic"],
  "install_hints": {
    "layout": "skill-directory-v1",
    "directory_name": "example-skill",
    "entrypoint": "SKILL.md",
    "required_files": ["SKILL.md"]
  },
  "files": [
    {
      "path": "SKILL.md",
      "role": "entrypoint",
      "content_type": "text/markdown",
      "digest": "sha256:...",
      "content": "# Example skill\n"
    }
  ]
}
```

If file content is present and the caller passes `include_content=true`, the
bundle response includes that content with `encoding` and
`content_included=true`. If file content is absent, Lesser still publishes the
metadata/digest contract and downstream clients report local state as
`unknown_local_state` or equivalent until a later content source exists.

Unsafe file paths (absolute paths or `..` traversal) are never published as
install hints.

## Cross-repo handoff for M4.2/M4.3

`lesser-body#137` should consume these Lesser endpoints rather than inventing a
local catalog shape. Body can expose MCP tools that list catalog entries, fetch a
selected bundle, and choose inline vs metadata-only responses.

`lesser-body#138` should verify local install state against:

- `bundle.digests.bundle_digest` for approved bundle content/metadata identity;
- `bundle.digests.publication_digest` for the exact publication contract shape;
- each `bundle.files[].digest` for local file drift.

This slice partially unblocks body work: Body has a stable catalog and bundle
contract, but metadata-only bundles still require body-side `unknown_local_state`
handling until approved revisions include inline file content or another
content-addressed fetch path is added.

## Contract audit

- Compatibility classification: additive, backward-compatible Lesser-exclusive
  REST API. No existing Mastodon-compatible endpoint changes shape.
- Mastodon clients: no impact; this is not a Mastodon endpoint.
- Remote ActivityPub peers: no impact; actor, object, inbox, outbox, WebFinger,
  and NodeInfo shapes are unchanged.
- GraphQL: no schema change; routes are recorded as REST-only in
  `docs/specs/graphql_coverage.yaml`.
- Sibling repos: `lesser-body` is the intended downstream consumer; `host`,
  `soul`, `greater`, and `sim` have no immediate contract change.
- Rollout: normal dev → staging → live deployment; body M4.2/M4.3 should remain
  blocked until this Lesser contract lands.

## Schema audit

M4.1 adds no new DynamoDB attributes, TableTheory tags, PK/SK patterns, or
physical GSIs. It adds a new read path through the existing sparse
`SkillRevision` GSI1 status pattern:

`SKILL_REVISION#STATUS#approved / UPDATED#{time}#SKILL#{skillID}#REVISION#{n}`

The read path is fail-closed: it loads the corresponding `Skill`, applies
existing exposure checks, requires `SkillRevision.status == approved`, and
publishes only the approved revision bundle. Existing stream processors do not
consume skill rows and require no update in this slice.
