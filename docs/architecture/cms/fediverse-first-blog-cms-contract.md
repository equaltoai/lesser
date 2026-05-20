# Fediverse-first Blog/CMS contract fence

Status: accepted M0 contract fence for Project 36

Scope: issues [#1024](https://github.com/equaltoai/lesser/issues/1024), [#1025](https://github.com/equaltoai/lesser/issues/1025), [#1026](https://github.com/equaltoai/lesser/issues/1026), [#1027](https://github.com/equaltoai/lesser/issues/1027), [#1028](https://github.com/equaltoai/lesser/issues/1028)

This document fixes the launch cutline for the MCP-first, fediverse-first Blog/CMS MVP before M1 implementation starts. It is a contract/spec PR only: it does **not** start the M1 route, storage, renderer, or federation changes.

## Current implementation drift to resolve after M0

Main currently has first-class CMS GraphQL services and draft publishing, but the implementation does not yet match the launch contract below:

- `createArticle` currently gives new Articles an object ID under `https://<domain>/objects/<uuid>`.
- draft publishing currently defaults new Article object IDs to `https://<domain>/objects/<uuid>` when the draft has no `objectId`.
- the objects Lambda currently serves ActivityPub objects from `/objects/{id}` and `/users/{username}/statuses/{id}`; it does not yet serve `/articles/{slug}`.

Those are M1/M2 implementation targets, not M0 changes.

## Contract audit

### Surfaces affected

- GraphQL CMS: `createArticle`, `updateArticle`, `createDraft`, `updateDraft`, `publishDraft`, `article`, `articleBySlug`, `revisions`.
- ActivityPub object fetch: future `GET /articles/{slug}` readback plus existing `/objects/{id}` legacy readback.
- Public browser HTML: future Article page content negotiation for browsers.
- ActivityPub federation: outbound Article `Create`, `Update`, `Delete`, and remote readback of referenced Article IDs.
- Sibling repos: `lesser-body` can create/generated drafts; `greater`/`sim` may render authoring UI later; `host` may route or validate deployment readiness later.

### Compatibility classification

M0 is documentation-only. The accepted implementation direction is additive for new Articles and compatibility-preserving for existing `/objects/<uuid>` Articles:

- New published Articles get the canonical ActivityPub ID `https://<domain>/articles/<slug>`.
- Existing Articles whose stored ID is `https://<domain>/objects/<uuid>` keep that ID as their ActivityPub object identity.
- `/objects/<uuid>` compatibility remains until a separate migration/alias plan is accepted and shipped.

No Mastodon REST `/api/v1/*` response shape changes are in M0.

### Consumer impact

- Mastodon clients: no M0 runtime impact. Future Articles still federate as ActivityPub `Article` objects and must not break status/timeline compatibility.
- Remote ActivityPub servers: future runtime impact is limited to fetching `Article` objects by their canonical ID. Remote peers cannot be coordinated directly, so M1/M2 must preserve existing fetch/signature behavior and test de-facto Mastodon compatibility.
- Operators/direct API users: should see the contract in release notes before M1/M2 behavior changes.
- Sibling repos: `lesser-body` must treat generated Article drafts as drafts needing review; `greater`/`sim` must consume server-rendered/sanitized output rather than inventing a separate renderer.

## Article identity contract

### Canonical ID for new Articles

For every newly published Blog/CMS Article after M1:

```text
https://<domain>/articles/<slug>
```

is the canonical ActivityPub object ID, the public article URL, and the GraphQL `Article.id` value.

Rules:

1. The slug is derived from the requested slug when supplied, otherwise from the title, using lesser's canonical slugifier.
2. A publish attempt must fail before federation if the final slug is empty or already claimed in the same tenant/domain.
3. `createArticle` and draft publish must use the same ID constructor. Direct create and draft publish cannot diverge.
4. ActivityPub `Create`/`Update` activities must embed the Article object with `id` equal to the canonical `/articles/<slug>` URL.
5. Browser and ActivityPub readback must resolve that same ID through content negotiation rather than separate object identities.

### Slug immutability

A published Article slug is immutable for the MVP because the slug is part of the ActivityPub object ID.

Allowed:

- Draft slug/title changes before first publish.
- Published title, subtitle, excerpt, body, metadata, and review-status updates that keep the Article ID unchanged.
- Legacy Article metadata cleanup that does not create a second ActivityPub identity.

Not allowed in the MVP:

- Changing the slug of a published `/articles/<slug>` Article.
- Reusing a previously published slug for a different Article in the same tenant/domain.
- Publishing an Article under `/articles/<new-slug>` as a replacement for an existing Article unless a Delete/Tombstone and migration plan is accepted first.

If mutable slugs are later required, they must be introduced as explicit aliases/redirects with Protocol Counsel review. Until that alias system exists, the implementation should reject published slug changes rather than silently changing object identity.

### Legacy `/objects/<uuid>` compatibility

Existing published Articles with IDs under `https://<domain>/objects/<uuid>` remain valid ActivityPub objects. They must not be rewritten in place during the MVP.

Compatibility rules:

1. Existing `/objects/<uuid>` Article IDs remain fetchable and federatable through the existing object read path.
2. GraphQL slug indexes may resolve legacy Articles by slug, but the returned `Article.id` remains the stored `/objects/<uuid>` ID.
3. If a browser-facing `/articles/<slug>` alias is introduced for a legacy Article, it is non-authoritative unless the stored object ID is also `/articles/<slug>`.
4. A future migration must not create duplicate ActivityPub objects for the same published content. Legacy migration/alias behavior is deferred to M7.

## M2 federation hardening decisions

M2 fixes the protocol behavior for Article federation without starting renderer,
sanitizer, UI, RSS, comments, newsletter, or remote-long-form-ingest work.

- Outbound Article `Create` and `Update` activities embed an ActivityPub
  `Article` object. The embedded object ID is the stored Article ID:
  canonical `https://<domain>/articles/<slug>` for new Articles and legacy
  `https://<domain>/objects/<uuid>` for existing Articles. `Update` preserves
  the same Article ID.
- Article delivery uses the same public/private policy as Note delivery:
  public-addressed Article activities fan out to followers and explicit
  recipients; non-public Article activities use recipient delivery without
  public follower fanout. Object-level hidden recipients (`bto`/`bcc`) are not
  serialized in embedded Article federation payloads or readback responses.
- Article `Delete` activities reference the Article object ID directly. Deleting
  a canonical Article creates an enhanced Tombstone with `formerType: "Article"`
  so `GET /articles/<slug>` returns an ActivityPub Tombstone with HTTP 410.
  Legacy Article IDs under `/objects/<uuid>` keep the same delete/tombstone
  behavior and are not rewritten.
- Inbound remote `Article` `Create` and `Update` activities are explicitly
  unsupported in the MVP. After normal inbox authentication/signature handling,
  lesser logs the unsupported Article operation and performs a no-op: it does
  not materialize the remote Article as a disguised status, does not add it to
  timelines, and does not create notifications. Inbound `Delete` for a locally
  stored Article can tombstone that object with `formerType: "Article"`; unknown
  remote Article deletes remain idempotent no-ops.
- Release validation should include a focused fetch probe against a canonical
  Article URL with `Accept: application/activity+json` and confirm the response
  is an ActivityPub `Article` at the canonical URL, with no `bto`/`bcc` fields.

## Renderer authority contract

### Source and output model

The server owns the publication renderer. Clients and agents may provide source, but they do not define the canonical rendered output.

MVP source policy:

- `MARKDOWN` is the normal authoring source format for new Blog/CMS Articles.
- `HTML` input is permitted only as input to the server sanitizer pipeline, not as trusted already-safe output.
- `contentFormat` records the submitted source format; public output is always the server-rendered/sanitized representation.
- Article source content is capped at **256 KiB** before rendering. The rendered/sanitized HTML body is capped at
  **512 KiB**. Exceeding either limit returns a deterministic validation error; lesser must never silently truncate
  Article source or rendered output.

Canonical output policy:

1. Draft preview, public browser HTML, and ActivityPub `Article.content` must be derived by the same server render/sanitize authority.
2. The sanitized HTML emitted to ActivityPub and the sanitized HTML emitted to public browser pages must be semantically consistent. Browser chrome/templates may differ, but the article body must not diverge.
3. Table of contents, word count, reading time, link normalization, media embedding, and unsafe-content handling belong to the same server-side rendering boundary.
4. Renderer output must be deterministic for the same source, metadata, and media inputs so preview/review/public/federated views can be compared.

### Draft preview API contract

M4.5 exposes the canonical Article preview renderer to body and other authenticated GraphQL consumers through:

```graphql
draftPreview(id: ID!): DraftPreview!
```

The resolver uses the same draft ownership/authentication rules as `draft(id:)`: CMS long-form and the draft system must be enabled, the caller must be authenticated, and the draft is loaded for the caller's own author ID. It then renders the draft through `DraftService.PreviewDraft` / `RenderDraftPreview`, which delegates to lesser's canonical Article publication renderer/sanitizer. Consumers must not re-render Markdown/HTML locally or use raw draft content as a public preview.

`DraftPreview` is intentionally small and stable for MCP consumers:

- `renderedHtml`: sanitized server-rendered Article HTML, present only when rendering succeeds;
- `sourceFormat`: the normalized or stored source format used for rendering;
- `sourceBytes` and `renderedBytes`: byte counts body can use for `preview_chars` and `max_output_bytes` controls;
- `errors`: deterministic user-facing renderer errors for unsupported format, invalid UTF-8, source-size, and rendered-size failures.

Authorization/storage lookup failures remain GraphQL errors, matching `draft(id:)`; content rendering failures return a `DraftPreview` with `success: false`, no `renderedHtml`, and a populated `errors` list so body can present review-safe feedback without exposing raw draft source.

### Unsafe content behavior

Unsafe input must never be forwarded to ActivityPub peers or public HTML pages unsanitized.

Minimum MVP sanitizer behavior:

- Remove or reject `<script>`, event-handler attributes, JavaScript URLs, unsafe iframe/embed/object tags, and style constructs that can execute script or exfiltrate data.
- Normalize links and media references through lesser's existing SSRF/media safety boundaries.
- Preserve safe semantic HTML needed for long-form publishing: headings, paragraphs, lists, blockquotes, code/pre, emphasis, links, images already attached/authorized, and tables if the sanitizer supports them safely.
- Return a validation error when sanitization would remove required publication content or when source exceeds configured limits.
- Record enough error context for operators without logging full private drafts, raw credentials, tokens, or signing material.

M3 owns implementation of this renderer/sanitizer boundary.

## Agent-authored draft attribution and review policy

### Identity separation

Generated draft attribution and published Article authorship are separate concepts.

- **Generator identity**: the agent/tool/model that created or materially transformed draft content.
- **Reviewer identity**: the human or authorized account that reviewed the generated draft and approved publication.
- **Publisher/owner identity**: the actor or publication account whose ActivityPub identity owns the final Article (`attributedTo`).

The final ActivityPub Article is attributed to the publisher/owner actor, not automatically to the generator. Generator attribution is metadata and audit context unless a future approved ActivityPub extension says otherwise.

### Required metadata for generated drafts

Before an MCP-generated draft can be published, the draft/revision metadata must be able to record:

- generator actor/account or delegated agent ID;
- generation time;
- model/tool identifier when available;
- delegation scope or authority that allowed draft creation;
- source task/prompt summary safe for audit logs;
- citations or source references when supplied;
- reviewer actor/account ID;
- review decision and review time;
- publisher actor/publication ID;
- revision lineage from generated draft to reviewed/published Article.

The metadata may reuse lesser's existing `AgentPostAttribution` vocabulary where it fits, but M4 owns the storage/API shape needed for CMS drafts and revisions. M0 does not add new fields.

### M4 implemented attribution fields

M4 stores and exposes the minimum attribution fields needed for MCP-first draft review without changing the ActivityPub Article object shape:

- `Draft.generatedBy` / storage `generatedBy`: the agent actor or delegated local actor that generated or materially transformed the draft. Agent-authenticated draft create, update, and autosave paths populate this when it is absent.
- `Draft.reviewedBy` / storage `reviewedBy`: the human actor that reviewed or edited a generated draft through the authenticated CMS workflow.
- `Article.generatedBy`, `Article.reviewedBy`, and `Article.publishedBy` / storage `generatedBy`, `reviewedBy`, and `publishedBy`: attribution copied from the draft at publish time plus the publisher actor that performed publication or post-publish update.
- `Revision.generatedBy`, `Revision.reviewedBy`, and `Revision.publishedBy` / storage `generatedBy`, `reviewedBy`, and `publishedBy`: the attribution snapshot recorded with each Article revision, alongside existing `changedBy`, `changeType`, `changeSummary`, and metadata JSON.

`Article.attributedTo` remains the publisher/owner ActivityPub actor and is the only author identity serialized into the federated Article in M4. Generator and reviewer attribution are additive GraphQL/storage metadata for audit and UI review surfaces; M4 does not introduce a new ActivityPub extension, JSON-LD context, Mastodon REST field, or OpenAPI change. Future federation-visible agent attribution still requires the blocking protocol review gate above.

### M4 schedule and rollback availability

Schedule/cancel and restore are backend MVP capabilities, not rich editor UI work:

- `scheduleDraft` and `cancelScheduledDraft` are available only when the CMS long-form, draft-system, and scheduled-publishing gates are enabled. Any UI exposure must remain hidden unless those gates are enabled and the authenticated author owns the draft.
- `restoreRevision` is available only when the CMS long-form, revision-history, and author-permission gates pass. UI controls must stay hidden unless revision history is enabled and the user can mutate the Article.
- “Rollback” in M4 means restoring local Article state from a recorded revision. It is not a federated recall mechanism: ActivityPub activities already delivered to remote peers cannot be recalled, and any post-restore federation follows the normal Article update path.

### Review gate

An agent-generated draft cannot auto-publish in the MVP. Publication requires an explicit reviewer/publisher action through lesser's authenticated CMS workflow.

Revision rules:

1. The first generated draft state is auditable.
2. Reviewer edits produce revisions that distinguish generated content from human-reviewed changes.
3. The publish revision records who approved and who published.
4. Later Article updates continue to preserve attribution/review history.

## MVP non-goals

The MVP does not include:

- rich WYSIWYG editing;
- Simulacrum retrofit or client redesign;
- newsletter delivery;
- comments system changes;
- search product work beyond existing indexed Article queries required by the MVP;
- RSS/Atom feeds;
- home timeline, trending, or social-feed coupling;
- general document hosting outside ActivityPub Article publishing;
- custom domain/routing changes beyond the canonical Article route required for this project;
- private hosting of non-federated document libraries;
- migration of every legacy `/objects/<uuid>` Article to `/articles/<slug>` in the launch path.

Any of these requires creator re-scoping before implementation.

## Blocking review gates

Protocol Counsel review blocks implementation before landing changes that affect:

- Article ActivityPub object IDs or JSON-LD/object shape;
- `/articles/{slug}` ActivityPub readback and content negotiation;
- slug alias/redirect semantics;
- Delete/Tombstone behavior for Articles;
- inbound remote Article handling;
- any new ActivityPub extension or agent-attribution serialization on Articles.

DevOps review blocks implementation before landing changes that affect:

- CloudFront/API Gateway routing for `/articles/{slug}`;
- cache policy, redirect status, canonical-link, or content-negotiation behavior;
- legacy `/objects/<uuid>` migration, alias generation, or backfill;
- operational storage/backfill strategy for historical Article bodies, media embedding, or scheduler throughput;
- observability dashboards/alarms for Article render/federation failures.

Security review is required for the M3 sanitizer boundary before public HTML or ActivityPub Article output can be considered launch-ready.

## Deferred risks by later milestone

- **M1 identity/routing** ([#1029](https://github.com/equaltoai/lesser/issues/1029)): direct create and draft publish must switch to `/articles/<slug>` together; `/articles/{slug}` route must not shadow existing `/objects/{id}` or `/users/{username}/statuses/{id}` routes.
- **M2 federation hardening** ([#1035](https://github.com/equaltoai/lesser/issues/1035)): Article Create/Update/Delete payloads, Tombstone behavior, inbound remote Articles, and peer fetch probes need explicit tests before live federation.
- **M3 renderer/sanitizer** ([#1040](https://github.com/equaltoai/lesser/issues/1040)): sanitizer strictness may remove author-intended markup; unsafe HTML handling and media/embed policy must be tested before public launch.
- **M4 authoring/attribution** ([#1045](https://github.com/equaltoai/lesser/issues/1045)): current models do not yet carry all generated-draft attribution/review metadata required above.
- **M7 operational/migration** ([#1050](https://github.com/equaltoai/lesser/issues/1050)): legacy `/objects/<uuid>` migration/aliases, body-size strategy, renderer observability, and launch runbook remain launch-readiness risks.

## Release-note text for the eventual implementation release

When M1-M4 ship, release notes should say:

> New Blog/CMS Articles publish with canonical ActivityPub IDs at `https://<domain>/articles/<slug>`. Existing Article objects under `/objects/<uuid>` remain compatible and are not rewritten. Published Article slugs are immutable in the MVP; change title/body/metadata instead. Draft preview, public HTML, and federated ActivityPub content are generated by lesser's server-side renderer/sanitizer, and agent-generated drafts require explicit reviewer/publisher approval before publication.
