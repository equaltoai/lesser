# Lesser - Implementation Progress

## Completed ✅

### 1. Project Setup
- [x] Created comprehensive design document (DESIGN.md)
- [x] Updated README with project overview
- [x] Initialized Go module structure
- [x] Created directory structure for Lambda functions and packages
- [x] Added AWS SDK dependencies
- [x] **Created Developer Guidelines** (DEVELOPER_GUIDELINES.md)
  - Technology stack decisions (zap for logging, no heavy frameworks)
  - Naming conventions and file structure
  - Testing strategy with examples
  - Security and performance guidelines

### 2. Core Packages
- [x] **pkg/activitypub/types.go** - Complete ActivityPub type definitions
  - Actor, Activity, Object types
  - Collections and OrderedCollections
  - WebFinger types
  - Helper functions and constants

- [x] **pkg/activitypub/validation.go** - Input validation
  - Username validation with regex
  - URL validation
  - Actor, Activity, and Note validation
  - Basic HTML sanitization
  - Comprehensive test coverage
  
- [x] **pkg/config/config.go** - Configuration management
  - Environment variable handling
  - URL generation helpers
  - Instance configuration

- [x] **pkg/storage/interface.go** - Storage interface definition
  - Actor operations
  - Activity operations
  - Relationship operations
  - DynamoDB record types

- [x] **pkg/common/** - Common utilities
  - **logging.go** - Zap logger setup with Lambda context
  - **errors.go** - Domain-specific error types
  - **response.go** - Consistent Lambda response helpers

### 3. Lambda Functions
- [x] **cmd/webfinger/main.go** - WebFinger discovery endpoint
  - Handles .well-known/webfinger requests
  - Parses acct: URIs
  - Returns proper WebFinger responses
  - Refactored to use common utilities
  - Structured logging with zap
  - ✅ NOW CONNECTED to DynamoDB storage

### 4. DynamoDB Storage Implementation ✅
- [x] **pkg/storage/dynamodb/client.go** - Core infrastructure
  - Connection pooling for Lambda reuse
  - DynamoDB client initialization in init()
  - Interface-based design for testability
  - Helper functions for building DynamoDB keys

- [x] **pkg/storage/dynamodb/actor.go** - Actor storage operations
  - CreateActor with encrypted private key storage
  - GetActor by username
  - GetActorPrivateKey retrieval
  - UpdateActor with optimistic locking
  - DeleteActor with proper error handling

- [x] **pkg/storage/dynamodb/activity.go** - Activity storage operations  
  - CreateActivity supporting both outbox and inbox
  - GetActivity by ID (uses scan for now)
  - GetOutboxActivities with cursor-based pagination
  - GetInboxActivities using GSI1 with pagination
  - Helper functions for username extraction and cursor encoding

- [x] **pkg/storage/dynamodb/actor_test.go** - Comprehensive unit tests
  - Table-driven tests for all actor operations
  - Mocked DynamoDB client for isolation
  - Error case coverage
  - Test helpers for creating test data

- [x] **pkg/storage/dynamodb/integration_test.go** - Integration tests
  - Tests against local DynamoDB instance
  - Actor lifecycle testing
  - Activity pagination testing
  - Build tags for selective execution

- [x] **pkg/storage/dynamodb/README.md** - Package documentation
  - Schema documentation
  - Usage examples
  - Testing instructions
  - Performance considerations

### 5. HTTP Signatures Package ✅
- [x] **pkg/federation/httpsig.go** - HTTP Signatures implementation
  - Signature parsing and verification (VerifyHTTPSignature)
  - Signature generation for outgoing requests (SignHTTPRequest)  
  - Support for RSA-SHA256 algorithm
  - Timestamp validation (±5 minutes)
  - Digest calculation and verification
  - Key management utilities (PEM encoding/decoding)

- [x] **pkg/federation/httpsig_test.go** - Comprehensive test suite
  - Unit tests for all functions
  - Integration tests with end-to-end signing/verification
  - Edge case coverage
  - 87.4% test coverage achieved

- [x] **pkg/federation/README.md** - Package documentation
  - Usage examples for incoming/outgoing requests
  - Security considerations
  - Integration with ActivityPub
  - Future enhancements

### 6. Actor Profile Endpoint ✅
- [x] **cmd/actor/main.go** - Actor profile handler
  - GET /users/{username} endpoint
  - Content negotiation (JSON vs HTML)
  - Returns ActivityStreams JSON for federation
  - Beautiful HTML profile pages for browsers
  - Public key serving for HTTP signatures
  - Connected to DynamoDB storage
  - 95.5% test coverage

- [x] **cmd/actor/handler_test.go** - Comprehensive unit tests
  - Mock storage implementation
  - Tests for both JSON and HTML responses
  - Error handling coverage
  - Content negotiation testing

- [x] **cmd/actor/README.md** - Endpoint documentation
  - API documentation
  - Content type examples
  - Testing instructions
  - Integration with WebFinger

- [x] **WebFinger Integration**
  - Updated cmd/webfinger to use real storage
  - Complete discovery flow working
  - WebFinger → Actor Profile → Public Key

### 7. Inbox Endpoint ✅ (NEW)
- [x] **cmd/inbox/main.go** - Inbox handler
  - POST /users/{username}/inbox endpoint
  - HTTP signature verification for authentication
  - Fetches sender's public key from actor profile
  - Activity validation (ID, Actor, Type required)
  - Addressing verification (to, cc, bto, bcc)
  - Stores activities in DynamoDB
  - Returns 202 Accepted for valid activities

- [x] **cmd/inbox/handler_test.go** - Comprehensive test suite
  - Mock HTTP server for testing actor fetching
  - Tests for signature verification
  - Tests for activity validation
  - Tests for addressing verification
  - 79.5% test coverage

### 8. Outbox Endpoint ✅ (NEW)
- [x] **cmd/outbox/main.go** - Outbox handler
  - POST /users/{username}/outbox endpoint
  - Accepts activities from local users
  - Validates actor matches authenticated user
  - Auto-generates activity IDs if not provided
  - Activity validation using activitypub package
  - Stores activities in DynamoDB
  - Returns 201 Created with the activity

- [x] **cmd/outbox/handler_test.go** - Comprehensive test suite
  - Mock storage implementation
  - Tests for activity creation
  - Tests for auto-generated IDs
  - Tests for auto-filled actor
  - Tests for validation errors
  - 84.7% test coverage

## In Progress 🚧

### Next: Activity Processor 🎯
This is the critical next step to enable processing and delivering federated activities:

1. [ ] **Activity Processor** (cmd/activity-processor)
   - Background Lambda triggered by DynamoDB Streams
   - Process incoming inbox activities (Accept follows, etc.)
   - Deliver outbox activities to remote servers
   - HTTP signature signing for outgoing requests
   - Handle retries and failures

### Phase 1: Core ActivityPub (Remaining)

2. [ ] **GET Outbox Handler**
   - Retrieve activities from user's outbox
   - Pagination support
   - Public/authenticated access control

3. [ ] **Activity Delivery System**
   - Sign requests with HTTP signatures
   - Deliver to recipient inboxes
   - Handle failures and retries
   - Track delivery status

4. [ ] **Remaining Storage Operations**
   - Object storage (Notes, Articles, etc.)
   - Relationship operations (follows)
   - Collection operations
   - DynamoDB transactions for atomic operations

5. [ ] **Collections Endpoint** (cmd/collections)
   - GET handlers for followers/following collections
   - Pagination support
   - Proper authorization

6. [ ] **OAuth 2.0 Implementation**
   - Authorization endpoint
   - Token endpoint
   - Client registration
   - Scopes and permissions
   - PKCE support

7. [ ] **Pulumi Infrastructure**
   - DynamoDB table definitions
   - Lambda function deployments
   - API Gateway configuration
   - IAM roles and policies
   - DynamoDB Streams setup

## Architecture Decisions 📋

### Key Design Choices Made:
1. **Single DynamoDB Table Design** - Using composite keys for efficient queries
2. **Lambda Per Endpoint** - Each ActivityPub endpoint gets its own Lambda for isolation
3. **Background Processing via DynamoDB Streams** - Decouple activity processing from HTTP requests
4. **OAuth 2.0 for Client Auth** - Compatibility with existing ActivityPub clients (not JWT)
5. **Zap for Logging** - Fast, structured logging optimized for Lambda
6. **No Heavy Frameworks** - Direct Lambda handlers to minimize cold starts
7. **Table-Driven Tests** - Comprehensive test coverage with clear test cases
8. **RSA-SHA256 for HTTP Signatures** - Industry standard algorithm for federation

### Open Questions:
1. ~~Should we use DynamoDB Streams or SQS for activity delivery?~~ → DynamoDB Streams chosen
2. How should we handle media storage lifecycle policies?
3. Should we implement a shared inbox for efficiency?
4. Should we add support for Ed25519 signatures (more efficient)?

## Testing Strategy 🧪

### Unit Tests Completed:
- [x] ActivityPub validation functions
- [x] WebFinger parsing
- [x] URL validation
- [x] HTML sanitization
- [x] DynamoDB Actor Operations - All CRUD operations with mocked client
- [x] DynamoDB Activity Operations - Create and query operations
- [x] HTTP Signature Operations - Parsing, verification, generation, key management
- [x] Actor Profile Handler - Content negotiation, error handling
- [x] Inbox Handler - Signature verification, activity validation

### Unit Tests Needed:
- [ ] ActivityPub type marshaling/unmarshaling
- [ ] DynamoDB object and relationship operations
- [ ] Outbox handler
- [ ] Activity processor logic
- [ ] OAuth 2.0 flows

### Integration Tests Completed:
- [x] DynamoDB Actor Lifecycle - Full CRUD cycle against local DynamoDB
- [x] DynamoDB Activity Pagination - Cursor-based pagination testing
- [x] HTTP Signature End-to-End - Sign and verify full cycle
- [x] WebFinger → Actor Profile Discovery - Complete flow working
- [x] Inbox HTTP Signature Verification - Full verification flow

### Integration Tests Needed:
- [ ] End-to-end federation test with Mastodon
- [ ] Activity delivery reliability
- [ ] Media upload and serving
- [ ] Full activity flow (create → outbox → delivery → inbox)

## Code Quality 📊

### Standards Established:
- **Naming Conventions**: camelCase for functions/variables, PascalCase for types
- **Error Handling**: Domain-specific error types with proper error wrapping
- **Logging**: Structured logging with request IDs and context
- **Testing**: Table-driven tests with descriptive names
- **Documentation**: Comprehensive godoc comments

### Test Coverage:
- `pkg/federation`: 87.4%
- `pkg/storage/dynamodb`: >80%
- `cmd/actor`: 95.5%
- `cmd/inbox`: 79.5%
- `cmd/outbox`: 84.7%
- `cmd/webfinger`: Needs unit tests

### Linting & Formatting:
- `gofmt` for consistent formatting
- `golangci-lint` for code quality checks
- All tests passing ✅

## Known Limitations 🚨

1. **Partial Storage Implementation** - Actor and Activity storage complete, but Object/Relationship/Collection operations pending
2. **No Authentication** - No OAuth 2.0 implementation yet
3. **No Activity Processing** - Activities are received but not processed
4. **No Activity Delivery** - Can't send activities to other servers yet
5. **No KMS Encryption** - Private keys stored in plaintext (TODO: AWS KMS integration)
6. **GetActivity Uses Scan** - Should optimize with GSI2
7. **HTTP Signatures RSA Only** - Ed25519 support planned for future

## Federation Status 🌐

### What's Working:
- ✅ **Discovery**: WebFinger endpoint returns correct actor URLs
- ✅ **Actor Profiles**: Serving valid ActivityPub actor objects
- ✅ **Public Keys**: Actors include public keys for signature verification
- ✅ **Content Negotiation**: Proper JSON/HTML responses
- ✅ **HTTP Signatures**: Can verify and sign federation requests
- ✅ **Receiving Activities**: Inbox can receive and verify activities from other servers
- ✅ **Creating Activities**: Outbox can accept and store activities from local users

### What's Missing:
- ❌ **GET Outbox**: Can't retrieve/serve local activities yet
- ❌ **Activity Processing**: Received activities aren't processed (follows, likes, etc.)
- ❌ **Activity Delivery**: Can't send activities to other servers
- ❌ **Collections**: No followers/following lists
- ❌ **Authentication**: No way for local users to authenticate
- ❌ **Media**: No support for images/attachments

## Resources 📚

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [WebFinger RFC 7033](https://tools.ietf.org/html/rfc7033)
- [HTTP Signatures Draft](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures)
- [Mastodon ActivityPub Implementation](https://docs.joinmastodon.org/spec/activitypub/) 