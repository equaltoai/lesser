# Promotional packages (M4)

Status: M4 contract for issue #1446 (parent #1442, media tree)

A promo package is the reviewed unit of promotion: outbound post text plus an
exact, ordered set of approved media assets, promoting a published article
through a public or unlisted post. The approved-asset library built by
M0–M3 (byte-verified admission, byte-bound review, durable published serving)
becomes usable for promotion without re-uploading bytes: the package reuses the
bound asset identity — the PUBLISHED media record and its M2 `publishedUrl`
serving.

## Byte-visibility decision (explicit, per issue #1446)

**Promo attachment is scoped to PUBLIC and UNLISTED posts only.** This is the
issue's own wording, and the scope is deliberate: the assets eligible for
promo attachment are exactly the M2 PUBLISHED assets, whose published URLs are
already world-served by design (the publish transition mints durable public
serving of the approved bytes once, and those bytes are on the public CDN
regardless of the status that later references them). Attaching an asset to a
public/unlisted post therefore does not widen any private byte surface.

**Internal/unpublished assets never attach to outbound posts — structurally,
not merely by policy.** The package compose path (`buildPromoPackageContent` in
`pkg/services/cms/promo_package.go`) rejects any asset that is not in the
PUBLISHED durable state (`Media.IsPublished()`: internal visibility plus
minted `publishedS3Key`, `publishedURL`, and `publishedAt`), and the release
seam re-verifies every asset against the same guard at the outbound surface
(`preparePromoPublishedAttachments` in `pkg/services/notes/promo.go`) plus the
digest bound into the reviewed package. A pre-publication internal asset's
bytes are SSE-KMS-encrypted, receive no unsigned CDN URL, and are never
referenced by a promo package, so they cannot leak through this lane.

Pre-release promo package *records* are not world-readable either: they resolve
only for the owner and holders of an active review grant (same posture as M2
drafts). The published *bytes* remain world-served by design; the *package
composition* (post text, visibility, asset selection) is private until release.

## Package model

`PromoPackage` is an additive TableTheory record on the main table:

- keys: `USER#{ownerID}#PROMO#PACKAGE` / `PACKAGE#{packageID}` (owner-scoped);
- `articleID` — the published article's canonical object URL;
- `postText` — the outbound post content (≤ 5000 bytes, the notes limit);
- `visibility` — `public` or `unlisted` only;
- `assets` — the ordered binding set: `mediaId` plus the canonical
  `sha256:<hex>` digest **as bound at review time**, plus the M2
  `publishedUrl` snapshot for display;
- `contentHash` — M2's length-prefixed canonicalization over post text,
  visibility, article reference, and ordered asset digests (binding order is
  the attachment order, so reordering re-requires review);
- `status` / `releasedStatusId` / `releasedAt` — stamped exactly once by the
  release transition (CAS); re-release and post-release composition are
  refused;
- `modelVersion` — version token for the CAS field-scoped content and release
  writes.

No new GSIs: the reviewer queue reuses GSI2's sparse key attributes with the
`PROMO#REVIEWER#` prefix. `PromoReviewGrant` (7-day bounded expiry, fail-closed
like the M2 grants) and `PromoReviewVerdict` (immutable, hash-bound) mirror the
draft-review records.

## Gate: internal review before release

Release reuses the draft-review machinery's shape, with the M4 release gate
governed by the **operator content doctrine** (2026-08-24, binding):

> "No content shall be published by agents without principal approval;
> additional approvals, once requested, are also required."

1. The owner composes the package (`composePromoPackage`); every content change
   re-hashes and makes prior verdicts stale through the verdict-vs-hash
   comparison.
2. The owner shares the package with reviewers (bounded grants) and reviewers
   submit hash-bound verdicts (`submitPromoPackageReview`), each carrying the
   content hash the reviewer actually inspected (a recomposed package rejects
   the submit with a conflict instead of blessing unseen content).
3. **Requested = required.** Release requires a current approving verdict from
   every reviewer who holds an active grant, **and from every reviewer who has
   ever recorded a verdict on the package — even if their grant was later
   revoked or expired. Revocation cannot delete a required approval.**
4. **Principal floor.** If the releasing actor is NOT the instance principal
   (an agent/act-as release), an active current principal approval is REQUIRED
   — regardless of asset provenance. The principal releasing themselves is the
   implicit approval (their action is the approval).
5. The doctrine matrix: principal releaser + zero ever-granted reviewers →
   allowed; principal releaser + granted reviewers → all required; non-principal
   releaser → principal required, plus all requested approvals. Until the
   applicable approvals are current, release is blocked with explicit reasons
   (`REVIEW_APPROVAL_REQUIRED` / `PRINCIPAL_APPROVAL_REQUIRED` and friends).
6. The release gate re-verifies each asset is still PUBLISHED and still carries
   the digest bound into the reviewed package (no substitution): `ASSET_MISSING`
   / `ASSET_NOT_PUBLISHED` / `ASSET_DIGEST_CHANGED` block release until the
   package is re-composed or re-reviewed.

## Release

`releasePromoPackage` creates the outbound Status through the notes promo lane
(`CreatePromoNote`): the post is created at public/unlisted visibility with the
exact approved assets attached, each attachment referencing the media record's
M2 `publishedUrl` (the durable serving minted from the approved bytes — no
re-upload, no unguessable-media fallback, no caller-supplied URL). The release
creates the post and nothing else: no boosts, likes, or synthetic engagement.

**AI-authorship disclosure mechanism.** The article surface discloses AI
authorship through `generatedBy`/`reviewedBy` attribution plus the
principal-approval gate. The post surface expresses the same disclosure via
`Note.AgentAttribution` (exposed as `agent_attribution` on the Mastodon REST
response): when any bound asset is AI-origin per provenance, the release sets
`TriggerType: "manual"` and `ApprovedBy` to the instance principal's actor URI
(the authorization recorded by the gate), preserving the disclosed-AI posture
on the outbound post. Packages with no AI-origin assets release without
attribution. Media-level provenance remains private editorial evidence on the
media records.

## Contract

Additive GraphQL only (contentus is a deployed consumer): new
`PromoPackage*` types, `composePromoPackage`,
`sharePromoPackageForReview` / `revokePromoPackageReview` /
`submitPromoPackageReview`, `releasePromoPackage`, and
`promoPackage` / `promoPackages` / `sharedPromoPackageReviews` queries, plus
the additive `includeAccessUrls` argument on `draftPreview`/`draftReview`
(fold-in: per-read access-URL minting is scoped to explicit exact-asset reads;
see the access-URL fold-in below). Mastodon REST, OpenAPI, ActivityPub,
JSON-LD, and federation contracts are unchanged — the release reuses the
existing note-creation pipeline.

## M4 fold-ins

- **Access-URL minting scoping.** `draftReview`/`draftPreview` previously
  minted a short-lived S3 URL per bound internal asset on every read. The
  minting is now gated behind `includeAccessUrls` (default `false`); ordinary
  reads are URL-free, and exact-asset reads use the existing
  `draftEditorialMediaAccess(draftId:, mediaId:)` lane (the media_read lane,
  which body's corrected docs already point to). The projection shape is
  unchanged (additive argument); body-leg coordinates by passing
  `includeAccessUrls: true` or using the exact-asset lane.
- **setDraftEditorialMedia CAS.** The editorial-media association writer is
  now version-conditioned: `attribute_exists(PK) AND
  (attribute_not_exists(modelVersion) OR modelVersion = read)` with a version
  bump, closing the lost-update seam where two concurrent media-set calls
  last-write-won. Pre-M4 draft rows migrate on their first media write
  (stamping version 1) instead of failing, and a concurrent loser surfaces a
  CONFLICT the body surface can render additively. The Draft `modelVersion`
  attribute is deliberately NOT `theorydb:"version"`-tagged: that tag would arm
  TableTheory's automatic optimistic lock on every full-model content write,
  whose condition fails in real DynamoDB for pre-M4 rows that never carried the
  attribute — every existing draft would break on its next content save.

## Persistence and compatibility

Additive TableTheory attributes/records only: no PK/SK/GSI/projection/version
changes to existing models (the promo records use their own key spaces and the
existing GSI2 sparse keys), no migration or backfill. Existing `Media`,
`Draft`, and draft-review rows are untouched; pre-M4 draft rows gain the
version attribute lazily on their first media write.
