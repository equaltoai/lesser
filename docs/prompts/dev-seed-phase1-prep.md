You are supporting the Lesser team by executing **Phase 1 – Environment & Access Prep** for bootstrapping dev data on `dev.lesser.host`. Follow these instructions sequentially, recording findings and blockers as you go.

1. **Environment Reachability**
   - Confirm DNS resolution for `dev.lesser.host` and verify HTTPS connectivity to `/graphql` and `/graphql/ws` (use curl and a WebSocket client such as `wscat` or `websocat`).
   - Capture HTTP status codes, TLS certificate summary, and any authentication challenges.
   - Note latency or intermittent failures that could impact seeding or GraphQL tests.

2. **Credential Inventory**
   - Identify or request the service account/admin credentials intended for automated seeding.
   - Validate scopes/permissions by hitting a benign authenticated GraphQL query (e.g., `viewer`).
   - Document storage location and rotation policy; flag if secrets management is missing.

3. **Media Storage Readiness**
   - Confirm S3 (or equivalent) bucket configuration for media used by `dev.lesser.host`.
   - Verify credentials permit uploads, reads, and deletion of test assets.
   - Ensure VAPID/public keys required for push registration are present and match the deployment configuration.

4. **Data Safety Net**
   - Determine whether a recent snapshot/backup exists for the dev environment.
   - If none, outline a rollback strategy (database snapshot, fixture rollback scripts, or environment rebuild steps).
   - Record who owns the backup process and how to trigger it.

5. **Fixture Asset Staging**
   - Audit existing `testdata/` directories for reusable assets (media files, JSON fixtures).
   - Catalog gaps (e.g., missing video attachments, spoilered media, preference JSON).
   - Prepare a manifest listing required assets with source paths and intended usage; ensure assets are committed or otherwise reproducible.

Deliverables:
- A short status doc summarizing each checklist item, including verification artifacts (command outputs, credential checks, storage tests).
- A risk/blocker list with owners and follow-up actions.
- Recommendations for tooling or automation needed before Phase 2 can begin.
