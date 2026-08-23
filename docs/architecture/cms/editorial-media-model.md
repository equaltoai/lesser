# Editorial media model

Status: M1 contract for issue #1444

This contract adds first-class media to unpublished CMS drafts. It builds on
M0's exact-byte S3 upload and canonical `sha256:<hex>` digest. It does not bind
media into review hashes or publication eligibility; that byte-bound approval
work belongs to M2.

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
   `KMS_KEY_ID`, rather than the public/social SSE-S3 posture;
2. receives no unsigned `CDNUrl`;
3. fails closed before object or database creation if the KMS key or internal
   object-store capability is unavailable.

The existing CloudFront origin may address media-bucket keys, but its service
principal has no decrypt grant on the instance KMS key. Therefore guessing an
internal object's key does not make its bytes world-readable. Lesser's
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
positions remain structured instead of being synthesized into raw draft source.
M1 does not copy the hero into a published `Article`, promote an object to the
public CDN posture, extend `DraftReviewContentHash`, or change publish gates.

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
- no PK, SK, GSI, projection, version, TTL, table, or stream routing changes;
- missing visibility on historical media continues to mean the existing public
  posture, so no migration or backfill is required.

The GraphQL changes are additive. Mastodon REST, OpenAPI, ActivityPub actor and
object shapes, JSON-LD, WebFinger, federation signing, and streaming contracts
are unchanged. lesser-body will consume this draft association in M3 rather
than reimplementing authorization or provenance storage.

AppTheory v3.3.0 and TableTheory v3.0.6 remain pinned. The design uses their
existing Lambda/KMS roles and additive TableTheory attributes; no framework
fork, raw DynamoDB bypass, or local substitute is introduced.
