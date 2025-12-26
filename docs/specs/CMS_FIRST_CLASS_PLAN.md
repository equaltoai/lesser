# CMS: First-Class Citizen Implementation Plan

> **Status**: Complete (Milestone 6 complete)  
> **Owner**: Lesser  
> **Last updated**: 2025-12-26  
> **Related**: `docs/HEADLESS_CMS_DESIGN.md`, `docs/CMS_DEVELOPER_GUIDE.md`, `docs/specs/GRAPHQL_COVERAGE_PLAN.md`, `graph/phase1.graphql`

This plan turns the current CMS GraphQL surface into a **first-class, production-grade CMS subsystem** in Lesser: consistent lifecycle rules, service-layer ownership, scheduled publishing, robust revisions, and deterministic public behavior.

## Definition: “first-class CMS”

CMS is considered first-class when:
- CMS features have a **canonical service layer** (not resolver-only logic).
- Draft lifecycle is **correct, deterministic, and recoverable** (status transitions and IDs).
- Scheduled publishing is **actually executed** (not just stored metadata).
- Revisions are **complete, permissioned, and restorable**.
- Article enrichment (TOC/reading time/word count) is **consistent**.
- Content organization is efficient (no scans) and **counts are correct** (or explicitly compute-on-read).
- CMS behavior integrates cleanly with Lesser instance state (e.g., “locked but reachable” rules).

## Scope

In scope:
- CMS domain services + storage behavior (drafts, articles, revisions, series, categories, publications).
- GraphQL parity for CMS operations (`graph/phase1.graphql`).
- Scheduled publishing execution path.
- Permissioning for CMS reads/writes.
- Content enrichment and indexing work necessary for practical clients.

Out of scope (for this plan):
- Auth setup wizard UI implementation (separate project).
- OAuth/OIDC flows.
- Protocol endpoints (`/.well-known/*`, `/nodeinfo/*`, ActivityPub) beyond ensuring CMS updates can federate.

## Invariants (must hold)

- **Deterministic IDs** (as already adopted by CMS GraphQL):
  - Articles: `https://<domain>/articles/<slug>`
  - Categories: `https://<domain>/categories/<slug>`
  - Publications: `https://<domain>/publications/<slug>`
- **Draft lifecycle**:
  - `DRAFT → SCHEDULED → PUBLISHING → PUBLISHED | FAILED`
  - `cancelScheduledDraft` returns to `DRAFT` and clears `scheduledAt`.
  - Publish operations must be idempotent (safe retries) and never create duplicate published objects for the same slug.
- **Permissions**:
  - Writes require auth and must enforce author/publication role rules.
  - Revision history is not publicly enumerable unless explicitly enabled (default: author/admin only).

## Milestones

### Milestone 0 — Hardening + Canonical Service Ownership

**Goal**: Eliminate resolver-only CMS logic and fix lifecycle correctness issues.

Deliverables:
- Draft lifecycle correctness in `pkg/services/cms/draft_service.go`:
  - Scheduling updates status (`SCHEDULED`) and canceling restores (`DRAFT`).
  - Publishing uses deterministic article IDs and updates draft status appropriately (`PUBLISHING`, then `PUBLISHED`/`FAILED`).
- Article lifecycle in `pkg/services/cms/article_service.go` becomes canonical:
  - Add `UpdateArticle`, `DeleteArticle`, and shared helpers for enrichment + revision creation.
  - Ensure updates/deletes can federate (follow-up in later milestones as needed).
- GraphQL CMS resolvers call service methods (not repositories directly) where appropriate.

Progress (2025-12-26):
- Draft updates now persist `status`/`scheduledAt` correctly via full-model updates (`pkg/storage/repositories/draft_repository.go`).
- `publishDraft` and `cancelScheduledDraft` now route through `pkg/services/cms/draft_service.go` (domain is derived from service config; no per-call domain parameter).
- Publishing is now idempotent and safe to retry:
  - Article creation uses a conditional write to prevent overwriting existing articles (`pkg/storage/repositories/article_repository.go`).
  - Draft publishing handles “already exists” as a successful publish when the existing article belongs to the same author (`pkg/services/cms/draft_service.go`).
- Article updates/deletes now route through `pkg/services/cms/article_service.go` (with best-effort federation + revision snapshot on update).
- Series mutations that change article metadata now update articles through `pkg/services/cms/article_service.go` (so revisions + federation happen consistently).

Exit criteria:
- CMS GraphQL ops rely on services for core writes (create/update/delete/publish).
- `make fmt`, `make lint`, `make test` all pass.

### Milestone 1 — Scheduled Publishing Execution

**Goal**: A scheduled draft actually publishes at the requested time.

Deliverables:
- A worker/Lambda to:
  - Find drafts with `status=SCHEDULED` and `scheduledAt <= now`
  - Publish them safely with retries
  - Mark failures as `FAILED` with a recoverable path (re-schedule or manual publish)
- Infra wiring (CDK) to run the worker on a schedule in the deployment region.

Progress (2025-12-26):
- Drafts now maintain a status/time index (`gsi4`) for efficient scheduled publishing queries (`pkg/storage/models/draft.go`).
- Scheduler queries due drafts via `DraftRepository.ListScheduledDraftsDuePaginated` (`pkg/storage/repositories/draft_repository.go`).
- Added the scheduled publisher Lambda `cmd/cms-scheduler/main.go` (runs every minute; capped batch; retries transient failures; marks non-retryable failures as `FAILED`).
- Wired Lambda packaging + CDK schedule:
  - `Makefile` includes `cms-scheduler` in `LAMBDAS`.
  - `infra/cdk/inventory/lambdas.go` declares `cms-scheduler` as `processor-scheduled` with `rate(1 minute)`.

Exit criteria:
- A scheduled draft publishes without manual intervention.
- Failure states are visible and recoverable.

### Milestone 2 — Revisions: Complete Snapshots + Restore Semantics

**Goal**: Revisions are useful for CMS workflows and safe by default.

Deliverables:
- Revision snapshots include CMS metadata (title/subtitle/excerpt/featured image/series/categories/SEO/editorial fields), not just content.
- Restores record a `RESTORE` revision and preserve auditability.
- Revision queries default to author/admin permission (no public enumeration).
- Optional retention cap (configurable max revisions per object).

Progress (2025-12-26):
- Revisions now snapshot and restore CMS metadata (incl. featured image summary) and store `contentHash` (`pkg/services/cms/revision_service.go`).
- Revision snapshots now include `tableOfContents`, `readingTimeMinutes`, and `wordCount` (`pkg/services/cms/revision_service.go`).
- Revision queries now require auth and enforce author/admin access (`graph/query_resolvers_cms.go`).
- Restore now records:
  - An `UPDATE` backup revision of the pre-restore state
  - A `RESTORE` revision representing the restored state (`pkg/services/cms/revision_service.go`)
- Optional retention cap is supported via `CMS_MAX_REVISIONS_PER_OBJECT` and enforced best-effort on writes (`pkg/config/config.go`, `pkg/services/cms/revision_service.go`).

Exit criteria:
- Restore fully rehydrates article state.
- Revision history is permissioned correctly.

### Milestone 3 — Content Enrichment Pipeline

**Goal**: CMS data is consistently enriched for clients.

Deliverables:
- Compute and persist:
  - `wordCount`
  - `readingTimeMinutes`
  - `tableOfContents`
- Enrichment occurs on create/update/publish in the canonical service.

Progress (2025-12-26):
- Added content enrichment utilities (`pkg/services/cms/enrichment.go`) to compute `wordCount`, `readingTimeMinutes`, and `tableOfContents` for markdown and HTML.
- Wired enrichment into canonical write paths:
  - `CreateArticle` / `UpdateArticle` (`pkg/services/cms/article_service.go`)
  - `RestoreRevision` (`pkg/services/cms/revision_service.go`)
- Added unit tests for enrichment behavior (`pkg/services/cms/enrichment_test.go`).

Exit criteria:
- Clients can rely on these fields without doing client-side parsing.

### Milestone 4 — Indexing + Organization (No Scans)

**Goal**: Replace scans with indexed queries for common CMS views.

Deliverables (minimum viable):
- Efficient list operations for:
  - Articles by series
  - Articles by category
  - Articles by publication (if/when publication ownership is introduced for articles)
- Clear policy for `articleCount` (maintained counters vs compute-on-read) and implement the chosen approach.

Progress (2025-12-26):
- Added a single-table CMS article index item (`pkg/storage/models/cms_article_index.go`) for listing articles by:
  - author (`CMS#ARTICLE#AUTHOR#<actorID>`)
  - series (`CMS#ARTICLE#SERIES#<seriesID>`)
  - category (`CMS#ARTICLE#CATEGORY#<categoryID>`)
- Implemented indexed list methods on `pkg/storage/repositories/article_repository.go`:
  - `ListArticlesByAuthorPaginated`
  - `ListArticlesBySeriesPaginated`
  - `ListArticlesByCategoryPaginated`
- Updated canonical write paths to maintain indexes + counts:
  - Article create/update/delete (`pkg/services/cms/article_service.go`)
  - Revision restore (`pkg/services/cms/revision_service.go`)
- Policy: `articleCount` is maintained as atomic counters (best-effort) via `UpdateBuilder().Add`:
  - `pkg/storage/repositories/series_repository.go`
  - `pkg/storage/repositories/category_repository.go`
- GraphQL `articles(...)` query now selects the most specific index and avoids scan-like behavior (`graph/query_resolvers_cms.go`, `graph/cms_article_query_helpers.go`).
- Publication-based article listing is deferred until articles gain publication ownership.

Exit criteria:
- No DynamoDB scans for routine CMS list queries at expected scale.

### Milestone 5 — Mode Gating + Locked Semantics

**Goal**: CMS respects instance state and “locked but reachable” behavior.

Deliverables:
- Configuration gates for CMS capabilities (mode/features) as described in `docs/HEADLESS_CMS_DESIGN.md`.
- Consistent public behavior:
  - Empty lists for content collections where appropriate
  - 404 for missing objects
  - Locked instance rules applied consistently for CMS + federation boundaries

Locked semantics (explicit):
- While `InstanceState.locked=true`, the instance must be **reachable but empty**:
  - REST content collections (timelines, trends, status search, account status listings) return `200` with an empty list/object.
  - REST single-status reads return `404` (treat content as absent).
  - Federation object reads (`/objects/:id`) return `404` (no public objects while locked).
  - Background publishers (CMS scheduled publishing) must **no-op** while locked (never create content).

Progress (2025-12-26):
- Added instance `mode` + CMS feature flags (`INSTANCE_MODE`, `CMS_*_ENABLED`) (`pkg/config/config.go`).
- CMS GraphQL resolvers now enforce feature gates and return clear errors when disabled (`graph/cms_feature_gates.go`, `graph/query_resolvers_cms.go`, `graph/mutation_resolvers_cms.go`).
- CMS scheduler now no-ops when the instance is locked or scheduled publishing is disabled (`cmd/cms-scheduler/main.go`).
- Locked instance now suppresses public content reads:
  - Mastodon REST content surfaces return empty lists / 404 (`cmd/api/middleware.go`).
  - ActivityPub object reads 404, outbox/liked collections return empty (`cmd/objects/main.go`, `cmd/outbox/main.go`, `cmd/collections/main.go`).

Exit criteria:
- Locked/unlocked behavior matches the deployment/setup requirements.

### Milestone 6 — Documentation + Tests

**Goal**: Make CMS maintainable and safe to evolve.

Deliverables:
- CMS developer docs: architecture + service ownership + data shapes.
- Tests for:
  - draft schedule/publish/cancel
  - revision snapshot/restore correctness
  - permissions for reads/writes
- Add a small “CMS smoke” test path suitable for CI (no AWS dependencies).

Progress (2025-12-26):
- Added CMS developer docs (`docs/CMS_DEVELOPER_GUIDE.md`).
- Added CMS service unit tests (`pkg/services/cms/draft_service_test.go`, `pkg/services/cms/revision_service_test.go`) including a smoke test.
- Added GraphQL CMS gating/permission unit tests (`graph/cms_feature_gates_test.go`, `graph/cms_permissions_test.go`).

Exit criteria:
- Future CMS changes are protected by tests and docs.
