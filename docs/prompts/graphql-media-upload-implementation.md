# Prompt: Implement Full GraphQL Media Upload Support

## Mission
Bring media ingestion to parity across GraphQL and WebSocket transports, following the scope documented in `docs/graphql-media-upload-plan.md`, so the platform supports uploads without relying on the REST API.

## Prerequisites
1. Review `docs/graphql-media-upload-plan.md` for background, scope, and cross-cutting concerns.
2. Confirm access to S3/dev buckets and any secrets needed for media processing in your environment.
3. Ensure the repository builds (`go test ./...`) before beginning.

## High-Level Objectives
1. Add GraphQL schema/resolver support for media upload using the `Upload` scalar.
2. Enable multipart handling in the GraphQL HTTP layer with size/type validations.
3. Integrate uploads into the media service, observability, and quota logic shared with REST.
4. Implement WebSocket upload support aligned with the new GraphQL pathway.
5. Enhance media pagination and admin/moderation workflows to account for new upload sources.
6. Update documentation, tests, and tooling to reflect the complete feature set.

## Step-by-Step Plan

### Phase A: Schema & Codegen
1. Introduce `scalar Upload` plus `UploadMediaInput` and `UploadMediaPayload` (rename as needed) in `graph/schema.graphql`.
2. Configure gqlgen to map `Upload` to `graphql.Upload` in `gqlgen.yml`.
3. Run `go run github.com/99designs/gqlgen generate` and resolve formatting or build issues.

### Phase B: Resolver Implementation
1. Implement `uploadMedia` resolver in `graph/mutation_resolvers_media.go` (or a dedicated file):
   - Authenticate via `requireAuth`.
   - Translate GraphQL input to `pkg/services/media.UploadMediaCommand`.
   - Stream file data and pass metadata to the media service.
   - Record cost metrics (`trackDynamoOperation`, etc.) matching REST behavior.
2. Update helper functions as needed to convert service results into GraphQL models.
3. Add error handling for validation failures (file size, MIME type, unsupported focus coordinates).

### Phase C: HTTP Handler & Validation
1. Update the GraphQL HTTP server configuration to enable multipart requests (`EnableTransport(transport.MultipartForm{})`, etc.).
2. Enforce `MAX_UPLOAD_SIZE` and MIME-type checks before invoking resolvers.
3. Add tracing/logging hooks so uploads appear in observability dashboards.

### Phase D: Service Layer Alignment
1. Review `pkg/services/media.Service.UploadMedia` to ensure it can accept the reader from gqlgen uploads.
2. Mirror validation performed in `cmd/api/lift/media.go` and consolidate shared logic where practical.
3. Verify downstream S3, moderation, and transcoding flows receive the same data regardless of transport.

### Phase E: WebSocket Upload Support
1. Implement chunked or base64 upload handling in `pkg/streaming/handlers/system_commands.go`:
   - Validate payload, enforce limits, and authenticate.
   - Reuse the service layer to process uploads and return status IDs.
2. Update command definitions in `pkg/streaming/command_types.go` and any related schemas.
3. Add streaming tests to cover success, validation errors, and user quota enforcement.

### Phase F: GraphQL Media Pagination Enhancements
1. Audit media-related queries (e.g., `graph/query_resolvers_media.go`) for pagination gaps.
2. Implement filters for owner, type, and time ranges; ensure indexes exist or add them in storage.
3. Confirm new uploads appear immediately in query results and add tests.

### Phase G: Admin & Moderation Workflow Integration
1. Ensure moderation services trigger on uploads from all transports (look at `pkg/moderation` pipelines).
2. Update admin dashboards/APIs as necessary to expose provenance and status for newly uploaded media.
3. Verify audit logging captures GraphQL/WebSocket uploads uniformly.

### Phase H: Testing & QA
1. Add unit tests for new GraphQL resolvers, validators, and service helpers.
2. Add integration tests uploading sample files via GraphQL and WebSocket (consider existing API test harness).
3. Run performance checks focusing on large uploads.
4. Execute `go test ./...` and any linters/formatters required by the repo.

### Phase I: Documentation & Developer Experience
1. Extend `docs/api-reference.md` with:
   - GraphQL upload mutation example.
   - WebSocket command example with payload schema.
   - Updated pagination and moderation behavior notes.
2. Update runbooks and developer guides (`docs/README.md`, `docs/architecture.md` if relevant).
3. Verify inline comments and logging provide enough context for supporting the feature post-launch.

### Phase J: Final Validation
1. Conduct end-to-end manual verification using GraphQL clients (e.g., Apollo Sandbox) and WebSocket tools.
2. Confirm metrics, logs, and costs reflect the new paths.
3. Prepare a final status report summarizing work, tests run, and any follow-up items.

## Acceptance Criteria
- GraphQL `uploadMedia` mutation successfully uploads files, enforces limits, and returns `Media` data.
- WebSocket upload command processes files and returns status equivalent to REST/GraphQL.
- Media queries provide paginated results including recent uploads with relevant filters.
- Moderation/admin flows react to GraphQL/WebSocket uploads identically to REST.
- Documentation and tests cover all new functionality.
- `go test ./...` passes without regressions.

## Deliverables
- Code changes implementing the feature set.
- Updated docs (API reference, guides, runbooks).
- Test results and any new helper scripts.
- Summary report covering acceptance criteria and residual risks.

## Notes
- Since the product is pre-release, no staged rollout is required, but maintain feature flags or configuration toggles if they aid testing.
- Coordinate with future client work by leaving clear TODOs or integration notes where necessary.
