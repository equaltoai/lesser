# API Alignment Implementation Checklist

This checklist follows the service-first architecture to minimize duplication across REST, GraphQL, and WebSocket APIs. Each task builds on the architectural foundation described in [API_ALIGNMENT_ARCHITECTURE.md](../architecture/API_ALIGNMENT_ARCHITECTURE.md).

**Pre-Release Advantage**: No backward compatibility needed. We can break anything to achieve the ideal architecture.

## Phase 1: Foundation (Days 1-2)

### 1.1 Event Publisher Infrastructure
- [ ] Create `pkg/streaming/publisher.go`
  - [ ] Define `Publisher` interface with `PublishToUser`, `PublishToStream`, `PublishToConversation`
  - [ ] Implement `Event` struct with Type, Stream, Payload, Timestamp
  - [ ] Create `apiGatewayPublisher` implementation using API Gateway Management API
  - [ ] Add `mockPublisher` for testing
  - [ ] Write unit tests for publisher

- [ ] Create `pkg/streaming/events.go`
  - [ ] Define event type constants (e.g., `StatusCreated = "status.created"`)
  - [ ] Define stream name constants (e.g., `UserStream = "user"`, `PublicStream = "public"`)
  - [ ] Create event builder helpers

### 1.2 Service Registry
- [ ] Create `pkg/services/registry.go`
  - [ ] Define `Registry` struct with all service fields
  - [ ] Add `NewRegistry` constructor with dependency injection
  - [ ] Add `WithPublisher`, `WithStorage` option functions
  - [ ] Write tests for registry initialization

### 1.3 Repository Interfaces
- [ ] Create `pkg/storage/interfaces/repositories.go`
  - [ ] Define `NoteRepository` interface
  - [ ] Define `AccountRepository` interface
  - [ ] Define `RelationshipRepository` interface
  - [ ] Define `MediaRepository` interface
  - [ ] Define `ConversationRepository` interface
  - [ ] Define `ListRepository` interface
  - [ ] Define `FilterRepository` interface
  - [ ] Define `NotificationRepository` interface

- [ ] Implement repositories in `pkg/storage/dynamodb/`
  - [ ] `notes_repository.go` wrapping existing dynamorm tables
  - [ ] `accounts_repository.go`
  - [ ] `relationships_repository.go`
  - [ ] Add remaining repositories

## Phase 2: Core Domain Services (Days 3-5)

### 2.1 Notes Service
- [ ] Create `pkg/services/notes/service.go`
  - [ ] Define `Service` struct with repo, publisher, federation dependencies
  - [ ] Define command structs: `CreateNoteCommand`, `UpdateNoteCommand`, `DeleteNoteCommand`
  - [ ] Define result structs: `NoteResult` with Note and Events
  - [ ] Implement `CreateNote` method
    - [ ] Validate input
    - [ ] Create note in repository
    - [ ] Emit events (user stream, public stream if public, hashtag streams)
    - [ ] Queue federation delivery
  - [ ] Implement `UpdateNote` method
  - [ ] Implement `DeleteNote` method
  - [ ] Implement `GetNote` method
  - [ ] Implement `ListNotes` with timeline logic
  - [ ] Write comprehensive tests

### 2.2 Accounts Service
- [ ] Create `pkg/services/accounts/service.go`
  - [ ] Define `Service` struct
  - [ ] Define commands: `UpdateProfileCommand`, `UpdatePreferencesCommand`
  - [ ] Implement `UpdateProfile` method
    - [ ] Validate changes
    - [ ] Update repository
    - [ ] Emit profile.updated event
    - [ ] Queue federation Update activity
  - [ ] Implement `UpdatePreferences` method
  - [ ] Implement `GetAccount` method
  - [ ] Implement `SearchAccounts` method
  - [ ] Write tests

### 2.3 Relationships Service
- [ ] Create `pkg/services/relationships/service.go`
  - [ ] Define `Service` struct
  - [ ] Define commands: `FollowCommand`, `UnfollowCommand`, `BlockCommand`, `MuteCommand`
  - [ ] Implement `Follow` method
    - [ ] Check existing relationship
    - [ ] Create follow relationship
    - [ ] Emit relationship.follow event
    - [ ] Queue federation Follow activity
  - [ ] Implement `Unfollow` method
  - [ ] Implement `Block` method
  - [ ] Implement `Mute` method
  - [ ] Implement `GetRelationship` method
  - [ ] Write tests

### 2.4 Conversations Service
- [ ] Create `pkg/services/conversations/service.go`
  - [ ] Define `Service` struct
  - [ ] Define commands: `SendDirectMessageCommand`, `MarkConversationReadCommand`
  - [ ] Implement `SendDirectMessage` method
    - [ ] Create/update conversation
    - [ ] Create message (note with direct visibility)
    - [ ] Emit conversation.message event to all participants
    - [ ] Queue federation if remote participants
  - [ ] Implement `MarkConversationRead` method
  - [ ] Implement `ListConversations` method
  - [ ] Implement `GetConversation` method
  - [ ] Write tests

### 2.5 Media Service
- [ ] Create `pkg/services/media/service.go`
  - [ ] Define `Service` struct
  - [ ] Define commands: `UploadMediaCommand`, `UpdateMediaCommand`
  - [ ] Implement `UploadMedia` method
    - [ ] Validate file type and size
    - [ ] Upload to S3
    - [ ] Queue processing (thumbnail, blurhash)
    - [ ] Create media record
    - [ ] Emit media.uploaded event
  - [ ] Implement `UpdateMedia` method (alt text, focus)
  - [ ] Implement `GetMedia` method
  - [ ] Write tests

### 2.6 Lists Service
- [ ] Create `pkg/services/lists/service.go`
  - [ ] Define `Service` struct
  - [ ] Define commands: `CreateListCommand`, `AddToListCommand`, `RemoveFromListCommand`
  - [ ] Implement CRUD operations
  - [ ] Emit list events for timeline updates
  - [ ] Write tests

### 2.7 Notifications Service
- [ ] Create `pkg/services/notifications/service.go`
  - [ ] Define `Service` struct
  - [ ] Define commands: `CreateNotificationCommand`, `MarkAsReadCommand`, `ClearCommand`
  - [ ] Implement notification creation (called by other services)
  - [ ] Implement marking as read
  - [ ] Implement clearing
  - [ ] Emit notification events for real-time delivery
  - [ ] Write tests

## Phase 3: Replace REST Handlers (Days 6-7)

### 3.1 Rewrite Lift Handlers
- [ ] Replace `cmd/api/lift/statuses.go`
  - [ ] Delete existing implementation
  - [ ] Create new handler using only `notes.Service`
  - [ ] No direct repository access
  - [ ] Break endpoints if needed for consistency

- [ ] Replace `cmd/api/lift/accounts.go`
  - [ ] Delete existing implementation
  - [ ] Create new handler using only `accounts.Service`
  - [ ] Align endpoint behavior with service design

- [ ] Replace `cmd/api/lift/relationships.go`
  - [ ] Delete existing implementation
  - [ ] Create new handler using only `relationships.Service`

- [ ] Replace `cmd/api/lift/conversations.go`
  - [ ] Delete existing implementation
  - [ ] Create new handler using only `conversations.Service`

- [ ] Replace `cmd/api/lift/media.go`
  - [ ] Delete existing implementation
  - [ ] Create new handler using only `media.Service`

- [ ] Replace `cmd/api/lift/lists.go`
  - [ ] Delete existing implementation
  - [ ] Create new handler using only `lists.Service`

- [ ] Replace `cmd/api/lift/notifications.go`
  - [ ] Delete existing implementation
  - [ ] Create new handler using only `notifications.Service`

### 3.2 Update Main Handler Initialization
- [ ] Update `cmd/api/main.go`
  - [ ] Create service registry
  - [ ] Pass registry to handler constructors
  - [ ] Wire up publisher

## Phase 4: Add GraphQL Support (Days 8-10)

### 4.1 Extend GraphQL Schema
- [ ] Update `graph/schema.graphql`
  - [ ] Add Conversation type and queries/mutations
  - [ ] Add List type and operations
  - [ ] Add Media mutations
  - [ ] Add Notification queries and mutations
  - [ ] Add relationship mutations (follow, unfollow, block, mute)
  - [ ] Add admin operations

### 4.2 Implement GraphQL Resolvers
- [ ] Update `graph/schema.resolvers.go`
  - [ ] Inject service registry in resolver struct
  - [ ] Implement status mutations using `notes.Service`
  - [ ] Implement account mutations using `accounts.Service`
  - [ ] Implement relationship mutations using `relationships.Service`
  - [ ] Implement conversation queries/mutations using `conversations.Service`
  - [ ] Implement list operations using `lists.Service`
  - [ ] Implement notification operations using `notifications.Service`

### 4.3 GraphQL Subscriptions
- [ ] Update `graph/subscriptions.graphql`
  - [ ] Add subscription types for all event types
  - [ ] Define subscription filters (by user, by stream, etc.)

- [ ] Implement subscription resolvers
  - [ ] Create subscription channels for each event type
  - [ ] Connect to event publisher
  - [ ] Handle filtering based on subscription parameters

## Phase 5: WebSocket Command Support (Days 11-12)

### 5.1 Command Handler Infrastructure
- [ ] Create `cmd/streaming/handlers/command_handler.go`
  - [ ] Define `CommandHandler` struct with service registry
  - [ ] Add command routing based on message type
  - [ ] Add error handling and response helpers

### 5.2 Implement Command Handlers
- [ ] Create `cmd/streaming/handlers/status_commands.go`
  - [ ] Handle `status.create` command using `notes.Service`
  - [ ] Handle `status.update` command
  - [ ] Handle `status.delete` command
  - [ ] Handle `status.boost` command
  - [ ] Handle `status.favorite` command

- [ ] Create `cmd/streaming/handlers/account_commands.go`
  - [ ] Handle `account.update` command using `accounts.Service`
  - [ ] Handle `preferences.update` command

- [ ] Create `cmd/streaming/handlers/relationship_commands.go`
  - [ ] Handle `account.follow` command using `relationships.Service`
  - [ ] Handle `account.unfollow` command
  - [ ] Handle `account.block` command
  - [ ] Handle `account.mute` command

- [ ] Create `cmd/streaming/handlers/conversation_commands.go`
  - [ ] Handle `conversation.send` command using `conversations.Service`
  - [ ] Handle `conversation.markRead` command

- [ ] Create `cmd/streaming/handlers/list_commands.go`
  - [ ] Handle list CRUD commands using `lists.Service`
  - [ ] Handle list membership commands

### 5.3 Update Stream Router
- [ ] Update `cmd/stream-router/main.go`
  - [ ] Add command handler routing
  - [ ] Maintain backward compatibility with existing streaming
  - [ ] Add command acknowledgment responses

## Phase 6: Long-Running Operations (Days 13-14)

### 6.1 Import/Export Service
- [ ] Create `pkg/services/import/service.go`
  - [ ] Define `StartImportCommand` returning job ID
  - [ ] Create import job record in DynamoDB
  - [ ] Emit import.started event

- [ ] Create `cmd/import-processor/main.go`
  - [ ] Listen to DynamoDB streams for new import jobs
  - [ ] Process imports (can run up to 15 minutes)
  - [ ] Emit progress events periodically
  - [ ] Emit completion event

### 6.2 Bulk Operations Service
- [ ] Create `pkg/services/bulk/service.go`
  - [ ] Define bulk operations (delete all posts, block list import, etc.)
  - [ ] Create job records
  - [ ] Emit job events

- [ ] Create `cmd/bulk-processor/main.go`
  - [ ] Process bulk operations asynchronously
  - [ ] Emit progress updates

### 6.3 Federation Sync Service
- [ ] Update `pkg/services/federation/service.go`
  - [ ] Add instance refresh operations
  - [ ] Add remote account sync
  - [ ] Return job IDs for long operations

- [ ] Update `cmd/federation-aggregator/main.go`
  - [ ] Process long-running federation tasks
  - [ ] Emit progress events

## Phase 7: Testing & Documentation (Day 15)

### 7.1 Integration Tests
- [ ] Create `tests/integration/service_parity_test.go`
  - [ ] Test same operation via REST, GraphQL, and WebSocket
  - [ ] Verify identical results
  - [ ] Verify events are emitted

### 7.2 Load Tests
- [ ] Update `tests/load/` scripts
  - [ ] Add GraphQL load tests
  - [ ] Add WebSocket command load tests
  - [ ] Compare performance across interfaces

### 7.3 Documentation Updates
- [ ] Update `docs/api/API_REFERENCE.md`
  - [ ] Document service layer
  - [ ] Add examples for all three interfaces

- [ ] Update `docs/api/GRAPHQL_API.md`
  - [ ] Document all new operations
  - [ ] Add subscription examples

- [ ] Create `docs/api/WEBSOCKET_COMMANDS.md`
  - [ ] Document command/response format
  - [ ] List all supported commands
  - [ ] Add examples

- [ ] Update `docs/api/GRAPHQL_WEBSOCKET_COVERAGE.md`
  - [ ] Remove "Why WebSocket is Limited" section
  - [ ] Add "Command + Event Architecture" section
  - [ ] Update coverage matrix to show full parity

## Phase 8: Delete Legacy Code (Day 16)

### 8.1 Remove All Old Code
- [ ] Delete old handler implementations that bypass services
- [ ] Remove unused repository methods
- [ ] Delete deprecated models and types
- [ ] Remove old test files for deleted code
- [ ] Clean up unused dependencies in go.mod

### 8.2 Enforce Architecture
- [ ] Add linting rules to prevent direct DB access in handlers
- [ ] Document that all business logic MUST be in services
- [ ] Create architecture tests that fail if handlers import storage packages
- [ ] Set up CI checks to enforce architecture boundaries

### 8.3 Final Cleanup
- [ ] Run `go mod tidy` to remove unused dependencies
- [ ] Delete any TODO comments related to backward compatibility
- [ ] Remove any feature flags or migration code
- [ ] Ensure zero code duplication across the codebase

## Success Criteria

- [ ] All REST endpoints have GraphQL equivalents
- [ ] All mutations can be performed via WebSocket commands
- [ ] All events are published to appropriate streams
- [ ] No business logic duplication across interfaces
- [ ] All services have >80% test coverage
- [ ] Load tests show comparable performance across interfaces
- [ ] Documentation is complete and accurate
- [ ] All legacy code deleted

## Notes

- Since we're pre-release, we can break anything that improves the architecture
- Delete old code aggressively - no deprecation needed
- Design services for the ideal API, not current implementation
- Use this opportunity to fix any API inconsistencies
- Consider adding OpenTelemetry tracing from the start
