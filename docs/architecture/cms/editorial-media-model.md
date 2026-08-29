# Editorial media model

Status: M2 contract for issues #1444 (M1) and #1445 (M2); M3 composition for issue #1512/#1513.

This contract adds first-class media to unpublished CMS drafts. It builds on
M0's exact-byte S3 upload and canonical `sha256:<hex>` digest. M1 models the
association, provenance, and grant-scoped reviewer reads; M2 binds media bytes
into review hashes and publication eligibility and mints durable published
serving at the publish transition.

## Draft association

`Draft.editorialMedia` is an ordered list of modeled `DraftMediaUsage` values.
Raw HTML or Markdown is not scanned to infer media.

| Role | Cardinality | Placement |
| --- | --- | --- |
| `HERO` | zero or one | Draft-side source for the existing `Article.featuredImage` seam |
| `INLINE` | zero or more | `inlinePosition` is a zero-based insertion point in the consumer's ordered article-block preview |
| `SOCIAL_CARD` | zero or one | Promotional/social-card presentation |

One asset can occur only once in a draft, and inline positions must be unique.
Each usage carries its own `caption`, reader-facing `creditLine`, `altText`, and
`focus`. `effectiveAltText` uses the per-usage value when present and otherwise
falls back to the media-global description.

`setDraftEditorialMedia` replaces the full association. Every newly attached
asset must exist, be owned by the draft author, be internal, and have provenance
bound to its media content hash. Associations can later resolve as `MISSING` if
an asset disappears; preview does not silently drop them.

The editorial-media association is written exclusively by its field-scoped
writer. Full-model content writers, including update, autosave, publish, and
schedule paths, never write the association in either direction; they cannot
restore a stale binding list or clear a concurrently replaced one.

## Provenance and attribution

Internal `Media.provenance` records:

- origin: AI-generated, AI-edited, photographed, illustrated, or supplied;
- generating/editing tool (required for AI origins);
- the responsible local actor;
- source/reference materials;
- rights and license notes;
- optional source creation/update timestamps and the server recording time;
- `contentIntegrity`, copied from M0's canonical SHA-256 identifier.

The provenance `createdAt` and `updatedAt` values are caller-supplied claims;
Lesser validates only their ordering. `recordedAt` is set by the server when it
records the provenance.

Provenance is private editorial evidence. It is not reader-facing attribution
and is exposed only on authorized draft/review representations. The usage's
`creditLine` is the reader-facing attribution surface. Neither field is added
to Mastodon REST or ActivityPub in M1.

## Internal byte posture

M1 editorial uploads are image assets; non-image editorial uploads fail
validation until a private derivative pipeline exists. Editorial uploads begin
with `visibility=internal`. Their original object:

1. is stored in the existing media bucket with SSE-KMS under the instance's
   `KMS_KEY_ID` when it arrives through the byte-path `uploadMedia` pipeline
   (see `uploadOriginal`), rather than the public/social SSE-S3 posture;
2. receives no unsigned `CDNUrl`;
3. fails closed before object or database creation if the KMS key or internal
   object-store capability is unavailable.

Objects that arrive through the presigned-companion PUT transport (see the next
section) are the exception to point 1: the presigned PUT signs no SSE
parameters, so those objects land encrypted at rest under the media bucket's
default encryption (SSE-S3, `BucketEncryption_S3_MANAGED`), exactly like every
other object in the bucket.

The existing CloudFront origin may address media-bucket keys, but its service
principal has no decrypt grant on the instance KMS key. Therefore guessing a
byte-path internal object's key does not make its bytes world-readable. For
presigned-companion objects (SSE-S3), the confidentiality basis is the
unguessable key: every object key embeds a random media ID minted with the
grant, and the bucket enforces `BlockPublicAccess_BLOCK_ALL`, so no object is
publicly listable or readable without an authorized signed URL — the same
posture as every other media-bucket object. Lesser's
encryption Lambda role can upload it. The current asynchronous media processor
writes public CDN derivatives, so internal editorial images are validated and
dimensioned synchronously and do not enter that derivative pipeline. This
prevents a thumbnail or re-encoded original from weakening the internal
posture. Existing published/social uploads keep their SSE-S3 storage, unsigned
CDN URL, and asynchronous processing behavior unchanged.

Owners and active `DraftReviewGrant` holders receive five-minute S3 URLs only
after `draftEditorialMediaAccess(draftId:, mediaId:)` proves that the exact
asset is bound to that exact authorized draft. A grant never authorizes a media
library listing or an unbound media ID. Revocation takes effect on the next
access request. A URL already issued before revocation remains a bearer
credential for at most five minutes; S3 access logs are the audit control for
use during that residual window. The ordinary `media(id:)` path rejects
internal media for every non-owner, including unauthenticated readers.

### Presigned-companion PUT contract

`mintUploadGrant` returns `UploadGrant.presignedUrl`, a SigV4 presigned PUT
whose signature binds the object key and signs only the `host` header
(`X-Amz-SignedHeaders=host`). The declared sha256 checksum is hoisted into the
URL as the `X-Amz-Checksum-Sha256` query parameter; S3 validates the body
against it. No server-side-encryption parameters are signed into the URL, so
the PUT needs no headers to echo and the object lands encrypted at rest under
the media bucket's default encryption (`BucketEncryption_S3_MANAGED`). A
client holding nothing but the URL completes the upload with:

- `Content-Type: <declared contentType>` (not signed, but finalize rejects a
  stored type that does not match the declaration, so a client that omits it
  stores `binary/octet-stream` and fails finalize with FAILED_DIGEST);
- the exact declared bytes as the body.

Sending a body that does not hash to the declared sha256 fails with
`BadDigest`. Client-added headers that are not signed (for example
`Cache-Control`) do not invalidate the signature. There is no `signedHeaders`
field on the grant: the earlier SSE-KMS presign that required echoing the
instance KMS key id (issue #1472) was removed because the undisclosed signed
values made every client PUT fail with `403 SignatureDoesNotMatch`; the
presigned URL is now self-sufficient.

## Preview contract

`Draft`, `DraftReview`, and `DraftPreview` expose `editorialMedia`. The canonical
preview preserves association order and returns:

- role and inline position;
- caption, credit, supplied alt, and effective alt;
- dimensions, MIME type, and content hash when the record exists;
- internal provenance;
- a short-lived exact-byte URL for the authorized caller;
- a conspicuous state: `MISSING`, `PROCESSING`, `READY`, or `REJECTED`.

`HERO` is the preview representation of the future featured image. `INLINE`
positions remain structured on the draft (M1/M2 contract) and compose into the
preview's `renderedHtml` from M3 on, using caller-authorized short-lived access
URLs when the read opts into URL minting.

## M2: byte-bound revision integrity

M2 binds review verdicts and publication to the exact media bytes.

### Revision hash

`draftReviewContentHash` now covers the ordered bound media set in canonical
order (hero, inline by `InlinePosition`, social card). Each usage contributes
its canonical content digest (`Media.ContentHash`, bound to provenance
`contentIntegrity`), role, inline position, caption, credit line, alt text, and
focus — all length-prefixed so boundaries stay unambiguous. The digest is
resolved per usage at verdict submit, review-state read, and the publish/schedule
gates (bounded at 100 usages per draft); an unresolvable asset contributes an
empty digest deterministically. Replacing, removing, reordering, re-cropping,
re-captioning, or re-crediting bound media therefore changes the hash and makes
prior verdicts and principal authorization stale through the existing
verdict-vs-hash comparison — publication stays blocked until the changed
revision is re-reviewed and re-authorized.

### Explicit editorial lifecycle

`Media.editorialState` is an editorial lifecycle distinct from the
processing-pipeline `Status` enum: `available` (default), `withdrawn`,
`superseded` (must name `supersededByMediaID`), and `unavailable`. The draft
preview/review surface exposes these as conspicuous states (`WITHDRAWN`,
`SUPERSEDED`, `UNAVAILABLE`) alongside the existing `MISSING` / `PROCESSING` /
`READY` / `REJECTED` flags, and the review state reports blocking reasons
(`BOUND_MEDIA_MISSING`, `BOUND_MEDIA_NOT_READY`, `BOUND_MEDIA_WITHDRAWN`,
`BOUND_MEDIA_SUPERSEDED`, `BOUND_MEDIA_UNAVAILABLE`). The publish gate requires
every required bound asset to be ready, internal, integrity-bound, and
lifecycle-available; otherwise publish fails with an explicit reason
(`ErrDraftReviewMediaRequired`).

### Durable published serving

The publish transition is the single point where durable public serving is
minted. For each bound asset, `PublishMediaDurably` copies the exact original
bytes (SSE-KMS internal source for byte-path arrivals; bucket-default SSE-S3
for presigned-companion/grant-path arrivals) to a `published/` key with SSE-S3
so the
unsigned CloudFront origin can serve them, records `publishedS3Key`,
`publishedURL`, and `publishedAt` on the media record, and the CMS service
verifies the minted digest equals the digest bound into the approved revision
hash — the exact bytes hashed at review are the bytes served at publish. The
published article's hero binding flows into `Article.featuredImage` (a durable
serving snapshot) and the social card into `Article.ogImage`. Published article
history cannot silently change from an external URL swap because the URLs are
minted once from the approved bytes. Pre-publish, internal assets still expose
no unsigned URL through the application contract.

### M3: composition and Article persistence

M3 (issue #1512/#1513) composes the bound media into rendered article HTML and
persists the bindings on the Article so the images survive the draft deletion
that follows publish.

**Canonical renderer composition.** `pkg/cmsrender` composes media descriptors
(URL, alt, caption, credit, dimensions, role, inline position) into the
sanitized article body:

- INLINE usages render as `<figure><img ...></figure>` (alt from the binding's
  `altText`, optional caption and reader-facing credit in the `figcaption`) at
  their bound `inlinePosition` — a zero-based insertion point before the Nth
  top-level block of the rendered article; positions at or past the block count
  append at the end.
- HERO composes as the article's leading image **in draft previews only**;
  published article HTML never duplicates the hero (it lives on
  `Article.featuredImage`).
- SOCIAL_CARD media never composes into the body.
- The renderer stays pure: it accepts descriptors and never resolves, mints, or
  authorizes URLs. All composed HTML passes through the canonical sanitizer, so
  only minted published URLs (or caller-authorized short-lived preview URLs)
  survive; width/height are emitted within the sanitizer's dimension range and
  alt/caption/credit are inserted as text.

**Preview composition.** `article_draft_preview` / `draftReview` compose the
draft's bound media into `renderedHtml` using the caller-authorized short-lived
access lane (the same `draftEditorialMediaAccess`-style URLs). URL minting stays
strictly opt-in (`includeAccessUrls` / `includeMediaAccess`): a default read
never mints bearer URLs and renders content only; an opted-in read mints each
bound asset exactly once and the same URLs feed both the structured
`editorialMedia` surface and the composed `renderedHtml`. Missing or
non-internal assets never compose (the structured surface still reports their
conspicuous state).

**Publish persistence.** `applyPublishedDraftMedia` additionally writes the full
bound set — hero, inline, social card — onto the Article as the additive
`Article.editorialMedia` attribute, in canonical order, each carrying its
minted public serving (URL, content type, content hash, dimensions) plus the
usage's caption, credit, alt, focus, and inline position. Because the draft is
deleted after publish, the article record is the surviving source for composed
media; a follow-up publish replaces the persisted set (never merges) and a
revision with no media clears it. A binding without a mint is a fail-closed
skip — the publish gate requires every required asset to serve the exact
approved bytes, so an unminted binding cannot occur on a committed article.

**Read paths compose.** Every `RenderArticleContent` read path composes from
the article's persisted media:

- GraphQL `Article.renderedHtml` composes inline media into the canonical
  sanitized HTML.
- ActivityPub article objects (federation Create/Update and object fetch) carry
  the composed content and attach the minted servings as `Document`
  attachments; the REST/Mastodon status representation built from those objects
  therefore carries the images in `content` and `media_attachments`.
- The CMS validation, enrichment/TOC, and object-repository conversions render
  through the same composition step.

**Schema impact.** Additive TableTheory attribute only: `Article.editorialMedia`
(a list of the published binding records). No PK, SK, GSI, projection, version,
TTL, table, or stream-routing changes; no migration or backfill is required.
Contract impact is additive and backward-compatible: GraphQL `renderedHtml`
gains composed `<figure>` elements, AP Article objects gain `attachment`
Documents, and REST statuses of articles with bound media gain composed
`content` and `media_attachments`. Articles without bound media render exactly
as before.

### Bounded grant expiry

`DraftReviewGrant.expiresAt` bounds every grant (7 days, refreshed on
re-share). Expired grants fail closed: they authorize no reviewer reads, URL
minting, or approval, and are surfaced with status `EXPIRED` in the review
surface. Grants recorded without an expiry are treated as expired so pre-M2
rows cannot authorize indefinitely.

### Schema impact

Additive TableTheory attributes only: `Media.editorialState`,
`Media.supersededByMediaID`, `Media.publishedS3Key`, `Media.publishedURL`,
`Media.publishedAt`, `DraftReviewGrant.expiresAt`, and (M3)
`Article.editorialMedia`. No PK, SK, GSI, projection, version, TTL, table, or
stream-routing changes; no migration or backfill is required. GraphQL changes
are additive (new lifecycle enum, state values, `publishedUrl`/`publishedAt`/
`expiresAt` fields, and the `updateEditorialMediaLifecycle` mutation).
Mastodon REST, OpenAPI, ActivityPub actor and object shapes, JSON-LD, WebFinger,
federation signing, and streaming contracts are unchanged; M3's composed
`renderedHtml`, AP `attachment` Documents, and REST `media_attachments` are
additive and backward-compatible.

## GraphQL exercise

The following abbreviated sequence is the dev-instance acceptance path:

```graphql
mutation Upload($file: Upload!) {
  uploadMedia(input: {
    file: $file
    description: "global fallback"
    editorialProvenance: {
      origin: ILLUSTRATED
      tool: "illustration-suite"
      rightsLicenseNotes: "commissioned"
    }
  }) { uploadId media { id url } }
}

mutation Attach($draft: ID!, $media: ID!) {
  setDraftEditorialMedia(draftId: $draft, media: [{
    mediaId: $media
    role: HERO
    caption: "Launch artwork"
    creditLine: "Illustration by Alice"
    altText: "A rocket leaving a violet planet"
  }]) { id editorialMedia { mediaId role caption effectiveAltText state } }
}

query Review($draft: ID!, $media: ID!) {
  draftPreview(id: $draft) {
    renderedHtml
    editorialMedia { mediaId role inlinePosition caption creditLine effectiveAltText state accessUrl }
  }
  draftEditorialMediaAccess(draftId: $draft, mediaId: $media) {
    url expiresAt contentHash
  }
}
```

The review query succeeds for the owner or an active reviewer-grant holder and
fails for an unrelated or unauthenticated caller. Direct unauthenticated fetch
through `media(id:)` also fails.

## Persistence and compatibility

This is an additive TableTheory model change:

- existing `Media` rows gain optional `visibility` and `provenance` attributes;
- existing `Draft` rows gain optional `editorialMedia`;
- existing `Article` rows gain optional `editorialMedia` (M3);
- no PK, SK, GSI, projection, version, TTL, table, or stream routing changes;
- missing visibility on historical media continues to mean the existing public
  posture, so no migration or backfill is required.

The GraphQL changes are additive. Mastodon REST, OpenAPI, ActivityPub actor and
object shapes, JSON-LD, WebFinger, federation signing, and streaming contracts
are unchanged. lesser-body will consume this draft association in M3 rather
than reimplementing authorization or provenance storage; for composed preview
`renderedHtml`, the MCP preview read should request `includeAccessUrls: true`
so the caller-authorized short-lived media URLs are minted for composition.

AppTheory v3.3.0 and TableTheory v3.0.6 remain pinned. The design uses their
existing Lambda/KMS roles and additive TableTheory attributes; no framework
fork, raw DynamoDB bypass, or local substitute is introduced.
