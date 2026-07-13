# CMS Developer Guide

This document describes Lesser’s CMS subsystem from a developer-maintenance point of view: **service ownership**, **data shapes**, **key/index patterns**, and the critical lifecycle rules that clients rely on.

## Architecture overview

CMS functionality lives primarily in:
- **Services**: `pkg/services/cms/*` (canonical business logic)
- **Models**: `pkg/storage/models/*` (DynamoDB single-table shapes + key patterns)
- **Repositories**: `pkg/storage/repositories/*` (persistence and query operations)
- **GraphQL**: `graph/*` + `graph/phase1.graphql` (API surface; resolvers call services)

The service layer is the “source of truth” for CMS writes. If you need to change a lifecycle rule, enrichment behavior, index maintenance, or revision semantics, change it in the CMS services (not inside GraphQL resolvers).

## Agent-safe GraphQL contract

Lesser's agent/CLI GraphQL entrypoint keeps a depth limit of 3. Relay connection wrappers (`edges` and `node`) are structural for this limit, so agent clients can query:

```graphql
articles {
  edges {
    node {
      id
      title
    }
  }
  pageInfo {
    hasNextPage
  }
}
```

Nested business objects still count toward the limit; agents should prefer scalar IDs instead of nested actors where possible. CMS nodes expose:
- `Draft.authorId`: the bare local username / draft author ID used by draft creation/list/update flows (for example, `alice`).
- `Article.authorId`: the full canonical ActivityPub actor URL stored on the article (`attributedTo`, for example, `https://example.com/users/alice`).
- `Draft.author` and `Article.author`: full actor objects for richer GraphQL clients that can afford the nested selection.

`lesser-body` and other MCP clients should treat this schema as the canonical Lesser contract and avoid substituting local CMS/profile storage.

## Service ownership (canonical responsibilities)

### `ArticleService` (`pkg/services/cms/article_service.go`)
Canonical ownership for **article writes**:
- `CreateArticle`, `UpdateArticle`, `DeleteArticle`
- Enrichment on write (`wordCount`, `readingTimeMinutes`, `tableOfContents`) via `enrichArticleContent`
- CMS index maintenance (`models.CMSArticleIndex`) for common list views
- Best-effort series/category `articleCount` counter updates
- Best-effort federation side effects on create/update/delete

### `DraftService` (`pkg/services/cms/draft_service.go`)
Canonical ownership for **draft lifecycle**:
- Draft creation/update/autosave
- Scheduling/canceling scheduled publishing
- Publishing drafts into articles (deterministic IDs + idempotency)
- Status transitions:
  - `draft → scheduled → publishing → published | failed`

### `RevisionService` (`pkg/services/cms/revision_service.go`)
Canonical ownership for **revision history + restore**:
- `CreateRevision` on update paths (service-driven; best-effort in some flows)
- `RestoreRevision` restores both content and CMS metadata, then recomputes enrichment
- Restore audit trail:
  - Backup `UPDATE` revision of the pre-restore state (best-effort)
  - `RESTORE` revision representing the restored state (best-effort)
- Optional retention cap via `CMS_MAX_REVISIONS_PER_OBJECT`

### `SeriesService`, `CategoryService`, `PublicationService`
Canonical ownership for organization primitives and list queries; article list queries must avoid scans and use either:
- `models.CMSArticleIndex` (author/series/category groupings), or
- native article indexes when appropriate.

## Deterministic IDs

CMS object IDs are deterministic (stable URLs):
- Articles: `https://<domain>/articles/<slug>`
- Categories: `https://<domain>/categories/<slug>`
- Publications: `https://<domain>/publications/<slug>`

Publishing must never create multiple distinct articles for the same slug.

## Data shapes and key/index patterns

### Draft (`pkg/storage/models/draft.go`)
Drafts are keyed by author + draft ID:
- PK: `USER#<authorID>#DRAFT`
- SK: `ID#<draftID>`

Important indexes:
- `gsi1`: “drafts for an object” (or “new drafts by author”)
  - `GSI1PK`: `OBJECT#<objectID>#DRAFT` (or `USER#<authorID>#NEWDRAFT`)
  - `GSI1SK`: `TIME#<updatedAt>`
- `gsi4`: scheduled publishing/status index
  - `GSI4PK`: `DRAFT#STATUS#<status>`
  - `GSI4SK`: `TIME#<scheduledAt|updatedAt>#AUTHOR#<authorID>#ID#<draftID>`

### Article (`pkg/storage/models/article.go`)
Articles embed the ActivityPub `Object` and add CMS metadata.

Notable CMS metadata fields:
- Enrichment: `tableOfContents`, `readingTimeMinutes`, `wordCount`
- Org: `seriesID`, `seriesOrder`, `categoryIDs`
- SEO/editorial: `seoTitle`, `seoDescription`, `canonicalUrl`, `ogImage`, `editorNotes`, `reviewStatus`

### Revision (`pkg/storage/models/revision.go`)
Revisions are keyed by object ID + version:
- PK: `OBJECT#<objectID>#REVISION`
- SK: `VERSION#<zero-padded version>`

Revisions store:
- `content` and `contentHash`
- `metadataJSON` (serialized CMS metadata snapshot)
- `changeType` (`create|update|restore`) and `changeSummary`

### CMS article index (`pkg/storage/models/cms_article_index.go`)
`models.CMSArticleIndex` provides scan-free list views grouped by:
- Author actor ID: `CMS#ARTICLE#AUTHOR#<actorID>`
- Series GraphQL ID: `CMS#ARTICLE#SERIES#<seriesID>`
- Category ID: `CMS#ARTICLE#CATEGORY#<categoryID>`

Entries sort newest-first by time:
- SK: `TIME#<publishedAt>#ARTICLE#<articleID>`

Write paths must keep these entries in sync with articles.

## Scheduled publishing

Scheduled publishing is executed by `cmd/cms-scheduler/main.go`, which:
- Queries due drafts via the draft status/time index (`gsi4`)
- Calls `DraftService.PublishDraft` with retries
- Marks non-retryable failures as `failed` and clears `scheduledAt`
- No-ops when instance mode/feature flags disable CMS, or when the instance is locked

## Mode gating + locked semantics

CMS behavior is gated by instance mode and feature flags (`pkg/config/config.go`) and enforced by GraphQL resolvers (`graph/cms_feature_gates.go`).

Locked semantics are “reachable but empty”:
- Public content surfaces return empty collections (or 404 for missing objects) as appropriate
- CMS background publishing must never create content while locked (scheduler no-ops)

## Testing guidance

CMS unit tests should be runnable in CI without AWS dependencies:
- Prefer service-level tests with stubbed repositories/services for deterministic behavior.
- Treat index and counter maintenance as best-effort side effects unless a test is specifically validating those invariants.
