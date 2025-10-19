# Prompt: Complete Media Metadata Support (sensitive/spoiler/mediaType)

## Context
- `UploadMediaInput` exposes `sensitive`, `spoilerText`, and `mediaType` (`graph/schema.graphql:530`, `graph/model/models_gen.go:1454`).
- The GraphQL resolver drops these fields when building `media.UploadMediaCommand` (`graph/mutation_resolvers_media.go:86-117`).
- `pkg/services/media.UploadMediaCommand` / `models.Media` do not store or persist the additional metadata (`pkg/services/media/service.go:115`, `pkg/storage/models/media.go:24-118`).
- GraphQL clients cannot read the metadata back because `type Media` lacks corresponding fields (`graph/schema.graphql:345-359`, `graph/model/models_gen.go:706-726`).

## Mission
Implement end-to-end handling for `sensitive`, `spoilerText`, and `mediaType` so GraphQL uploads persist these flags, and downstream queries expose them consistently with REST behavior.

## Key Tasks
1. **Schema & Models**
   - Add `sensitive`, `spoilerText`, and an enum/field for media category (if not already present) to `type Media`.
   - Regenerate gqlgen models (`graph/model/models_gen.go`) after updating `graph/schema.graphql`.

2. **Command & Service Layer**
   - Extend `media.UploadMediaCommand` with the new fields (`pkg/services/media/service.go:115`).
   - Persist metadata in `models.Media` (`pkg/storage/models/media.go`). Add sync with validation (e.g., spoiler length, mapping to `IsNSFW` if appropriate).
   - Ensure `Service.UploadMedia` sets the new properties and validates them.
   - Update update paths or moderation logic if they need to read the new values (e.g., any NSFW gating that should also look at `Sensitive`).

3. **Resolvers & Conversion**
   - Pass the new fields from `UploadMediaInput` into the command (`graph/mutation_resolvers_media.go:71-117`).
   - Map stored values back in `convertMediaToGraphQL` (`graph/schema.resolvers.go:453-491`).
   - Update query resolvers so `mediaLibrary` and other media queries return the metadata.

4. **Storage & Repository**
   - If Dynamo serialization requires explicit struct tags or index updates, adjust `pkg/storage/repositories/media_repository.go`.
   - Confirm any moderation workflows that rely on `IsNSFW` or labels remain correct when `sensitive` is toggled.

5. **WebSocket & REST Parity**
   - Update WebSocket handler payload parsing to accept the fields and forward them (`pkg/streaming/handlers/system_commands.go:320-420`).
   - Align REST handler (`cmd/api/lift/media.go`) if these flags should be honored there as well.

6. **Documentation & Tests**
   - Update `docs/api-reference.md` to document the new response fields and mutation parameters.
   - Add/extend unit tests:
     * GraphQL resolver tests (if available) or service tests to assert metadata persistence.
     * WebSocket handler tests (`pkg/streaming/handlers/system_commands_media_test.go`) to cover the new fields.
     * Service tests verifying validation and storage of spoiler text / sensitive flag.

## Validation Checklist
- GraphQL `uploadMedia` accepts and persists the fields; returned payload shows expected values.
- Queries (`mediaLibrary`, `media`) expose the metadata.
- WebSocket uploads behave the same as GraphQL.
- Documentation reflects the new behavior.
- All tests (`go test ./...`) pass.
