# GraphQL Media Upload Implementation Plan

## Goal
Deliver first-class GraphQL support for media ingestion so GraphQL clients can upload, manage, and stream media without falling back to the Mastodon REST API.

## Current Gaps
- No `Upload` scalar or multipart handling in the GraphQL gateway (`graph/schema.graphql`, `graph/model/scalars.go`).
- GraphQL mutations only support metadata updates (`updateMedia`) and streaming helpers (`requestStreamingUrl`, `preloadMedia`); there is no mutation that accepts binary payloads.
- HTTP handler for `/graphql` is not configured to process multipart requests or enforce upload-specific limits.
- Service telemetry and cost tracking are only wired through the REST controller (`cmd/api/lift/media.go`) and may miss GraphQL usages.
- WebSocket command handler advertises GraphQL uploads but the functionality is absent (`pkg/streaming/handlers/system_commands.go:282`).

## Scope of Work

### 1. Schema & Codegen
- Introduce `scalar Upload` and `mutation uploadMedia(input: UploadMediaInput!): MediaUploadPayload!` (name TBD).
- Define an input type that captures filename, description, focus point, spoiler flag, media type, and optional alternative text.
- Add an explicit payload type to carry the `Media` object plus relevant IDs and warnings.
- Update gqlgen config to map the `Upload` scalar to `graphql.Upload` and regenerate `graph/generated.go`.

### 2. Resolver Implementation
- Add resolver implementation in `graph/mutation_resolvers_media.go` (or a new file) to:
  - Enforce auth via `requireAuth`.
  - Translate the GraphQL input into `pkg/services/media.UploadMediaCommand`.
  - Stream file contents into the media service, handling size and MIME detection.
  - Track costs/metrics analogous to the REST handler.
- Update conversion helpers so the new payload returns consistent GraphQL `Media` objects.

### 3. HTTP Layer Enhancements
- Enable multipart support in the GraphQL HTTP handler (most likely via `handler.NewDefaultServer` options).
- Ensure `MAX_UPLOAD_SIZE` and accepted MIME types are enforced at the transport layer, mirroring `HandleUploadMediaLift`.
- Add request logging and rate limiting hooks if missing.

### 4. Service Layer Adjustments
- Audit `pkg/services/media.Service.UploadMedia` for any assumptions tied to REST forms; add helper to accept `io.Reader` from GraphQL uploads.
- Confirm existing validation (`validateUploadCommand`) accommodates metadata provided over GraphQL and add new validation errors if necessary.
- Ensure S3 key generation, moderation triggers, and transcoding pipelines are unaffected by the new ingestion path.

### 5. Observability & Quotas
- Extend cost tracking to mark GraphQL-sourced uploads separately (if needed) for analytics dashboards.
- Integrate upload counters/metrics with existing monitoring (`pkg/observability/constants.go`, CloudWatch EMF).
- Verify quota enforcement (per-user hourly/monthly limits) is consistent regardless of transport.

### 6. Client & Documentation Updates
- Document the new mutation in `docs/api-reference.md` and provide sample multipart GraphQL requests (including `curl` and Apollo examples).
- Update README or developer guides to highlight 100% GraphQL coverage for media.
- Clarify WebSocket docs so they no longer redirect users to a non-existent GraphQL path.

### 7. WebSocket Upload Support
- Extend the WebSocket command handler (`pkg/streaming/handlers/system_commands.go`) to support chunked or base64-encoded uploads, matching the new GraphQL pathway.
- Define payload format, maximum size, and authentication/authorization checks consistent with REST and GraphQL.
- Ensure responses include IDs and processing status so clients can correlate uploads.
- Add integration tests for WebSocket uploads (including failure conditions for invalid payloads or oversized uploads).

### 8. GraphQL Media Pagination Enhancements
- Review existing media queries to ensure newly uploaded assets are discoverable with filters for owner, type, and upload time.
- Add pagination cursors and total counts where missing to support client-side galleries.
- Update resolvers to leverage existing storage indices for efficient queries, adding new indexes if necessary.

### 9. Admin & Moderation Workflow Alignment
- Confirm moderation services receive the same signals (hashing, flagged metadata) when media originates from GraphQL or WebSocket uploads.
- Update any admin dashboards or moderation review queues to surface GraphQL-uploaded assets with correct provenance.
- Add tests covering automatic moderation triggers and audit logging for uploads across all transports.

### 10. Testing Strategy
- Unit tests for the new resolver covering happy path, unsupported MIME types, file too large, and unauthenticated users.
- Service tests ensuring `UploadMedia` handles GraphQL-provided metadata consistently.
- Integration/e2e tests uploading real files through GraphQL and WebSocket transports.
- Load/performance evaluation for large uploads to assure multipart handling and WebSocket pathways meet latency targets.

### 11. Documentation & Developer Enablement
- Document the new GraphQL mutation, WebSocket command, and updated pagination in `docs/api-reference.md` with sample requests.
- Update developer guides and README to state that media ingestion is fully supported via GraphQL and WebSocket.
- Ensure internal runbooks cover operational steps for debugging uploads across all transports.

## Post-Implementation Focus
- Coordinate with client teams to build GraphQL/WebSocket upload flows.
- Audit official SDKs or CLI tools for alignment with the new transport options.
- Verify documentation and developer tooling are sufficient before public launch (no staged rollout necessary due to pre-release status).

## Next Steps
1. Align on mutation naming and payload structure with product/SDK consumers.
2. Implement schema + resolver + transport changes behind a short-lived feature branch.
3. Add tests, regenerate gqlgen artifacts, and update documentation.
4. Conduct validation in staging with representative file types and sizes.
5. Announce availability to stakeholders and update roadmap trackers.
