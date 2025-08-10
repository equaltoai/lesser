# Lesser Completeness Audit (Pre-Release)

This audit identifies implemented areas, partials, and gaps to resolve before public release. Scope includes ActivityPub compliance, Mastodon API compatibility, federation, media, moderation, auth, notifications, search, observability, deployment, and explicit exclusions (no email/SMS support).

## ActivityPub & Federation
- Implemented
  - Inbox/outbox processing for core Activities: Create, Update, Delete, Follow, Accept/Reject, Like, Announce, Undo, Block, Flag, Add/Remove.
  - WebFinger (`/.well-known/webfinger`) and NodeInfo (`/.well-known/nodeinfo`, `/nodeinfo/2.0`, `/nodeinfo/2.1`) endpoints with tests.
  - Delivery service with HTTP Signatures, sync/async (SQS) queuing, exponential backoff, target health assessment, and DLQ integration points.
  - Remote actor discovery with WebFinger + caching.
  - Authorized fetch enforcement and signature verification (hs2019, RSA, ECDSA, Ed25519) tests present.
- Gaps / Risks
  - Some inbox/outbox handler methods log and return without full enforcement or side-effects (admin/timeline updates, relationship mutations) in `cmd/activity-processor/handler.go`.
  - Federation repository has TODOs around route optimization/circuit repositories and certain remote lookups.
  - Host-meta (`/.well-known/host-meta`) endpoint not found; not strictly required but improves interop.

## Mastodon API Compatibility
- Implemented
  - Broad v1 coverage via `cmd/api/lift/*` with Mastodon-shaped responses using converter utilities.
  - Admin status listing/counting, filters, bookmarks, favourites, markers, conversations, trends, push subscriptions, instance info.
  - Streaming WebSocket support and cost headers.
- Gaps / Risks
  - Some handlers include TODOs for timeline and conversation repository integrations (e.g., home/direct timelines).
  - SSE endpoint mentioned in docs; explicit code for SSE streaming not found.
  - Documentation claims 100% REST; internal matrix says ~85% coverage. Reconcile and generate definitive, tested status list.

## GraphQL API
- Implemented
  - Lambda-based GraphQL with schema, resolvers, DataLoader scaffolding, cost tracking.
- Gaps / Risks
  - Archive docs indicate many resolvers/mutations/subscriptions were initially stubbed. Verify current generated resolvers for unimplemented paths and panics in `graph/generated.go`/`schema.resolvers.go` and ensure no runtime panics.

## Authentication & Authorization
- Implemented
  - OAuth2 with PKCE, WebAuthn passkeys, crypto wallet auth scaffolding, CSRF protection, refresh token rotation, rate limiting.
  - No email or SMS flows by design; docs emphasize email-free auth.
- Gaps / Risks
  - Ensure `.well-known/oauth-authorization-server` discovery endpoint is actually served by auth Lambda (referenced in docs and tests, code path not clearly located).
  - Remove/guard any comments suggesting SMS/email notifications in recovery flows.

## Media Processing
- Implemented
  - Image processing, multiple sizes, blurhash, EXIF stripping; video/audio processing with async jobs and cost tracking.
  - Media processor Lambda updates media records, returns blurhash/preview/sizes.
- Gaps / Risks
  - Confirm MediaConvert/transcoding helpers wiring and IAM for production; ensure graceful fallback when MediaConvert absent.

## Notifications & Push Delivery
- Implemented
  - Notification persistence, grouping, read state, WebSocket delivery, Web Push delivery with VAPID keys management.
  - Push delivery Lambda pulls user subscriptions and sends encrypted payloads.
- Gaps / Risks
  - Ensure VAPID key generation and retrieval paths are enforced in instance configuration; placeholder key fallback exists in API if keys missing.
  - Verify fanout triggers are hooked from activity processing to notification creation for all types (mention/fav/reblog/follow).

## Search & Indexing
- Implemented
  - Search indexer on DynamoDB streams extracts indexable content; repositories for search, trending, hashtags.
- Gaps / Risks
  - Token validation in `pkg/middleware/search_privacy.go` has TODO for proper validation; fix before release.
  - Confirm advanced strategies (semantic search) flags and fallbacks when AI providers are not configured.

## Moderation & Trust
- Implemented
  - Pattern-based and AI-assisted moderation engine with decision recording and enforcement processor; reports, filters, mutes/blocks.
- Gaps / Risks
  - Ensure enforcement paths for suspend/silence/remove fully propagate to timelines, search, streaming, and federation (Undo/Block/Reject, Delete/Tombstone fanout).

## Observability & Cost Tracking
- Implemented
  - EMF metrics, standardized metric names, dashboards, alert thresholds documented; cost tracking headers per request.
- Gaps / Risks
  - Verify alert wiring (CloudWatch alarms, SNS topics) and environment configuration for all Lambdas.

## Deployment & Infrastructure
- Implemented
  - IaC (CDK directory present), API routing for well-known endpoints, CloudFront caching policies, instance configuration tooling including VAPID.
- Gaps / Risks
  - Ensure all new endpoints are registered in infra routing; verify CORS/security headers across services.
  - Confirm DLQ wiring for federation and push delivery processors.

## Explicit Non-Goals
- Email and SMS are not supported and should not be introduced. Remove dead references or examples implying email/SMS notifications.

## High-Priority Fix List (Pre-Release)
1. Implement proper token validation in search privacy middleware; remove TODO.
2. Complete inbox/outbox handlers where they currently log and return without state mutations (follows, accepts, rejects, blocks, moves, add/remove).
3. Reconcile Mastodon API coverage: generate authoritative checklist from `cmd/api/lift` routes and test against clients; document any unsupported endpoints.
4. Verify GraphQL resolvers: remove any panic paths; ensure no unimplemented operations remain.
5. Ensure OAuth discovery endpoint is live and tested; align docs and infra.
6. Enforce VAPID keys existence in prod; fail-fast or disable push features when absent.
7. Verify moderation enforcement propagation to timelines/search/streaming and federation Undo/Delete flow.
8. Add host-meta endpoint for broader compatibility if desired.
9. Audit infra alarms and DLQ wiring across federation/push/media processors.
10. Remove or guard any stray email/SMS references.


