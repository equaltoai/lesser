# CMS: First-Class Citizen Implementation Plan

> **Status**: Active (Milestone 0 complete; Milestone 1 not started)  
> **Owner**: Lesser  
> **Last updated**: 2025-12-26  
> **Related**: `docs/HEADLESS_CMS_DESIGN.md`, `docs/specs/GRAPHQL_COVERAGE_PLAN.md`, `graph/phase1.graphql`

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
- Revision queries now require auth and enforce author/admin access (`graph/query_resolvers_cms.go`).

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

Exit criteria:
- Future CMS changes are protected by tests and docs.
