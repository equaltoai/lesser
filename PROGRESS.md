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
  - TODO: Connect to actual storage

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
  - 84.3% test coverage achieved

- [x] **pkg/federation/README.md** - Package documentation
  - Usage examples for incoming/outgoing requests
  - Security considerations
  - Integration with ActivityPub
  - Future enhancements

## In Progress 🚧

### Next Steps (Phase 1: Core ActivityPub)
1. [x] ~~**DynamoDB Storage Implementation**~~ ✅ COMPLETED
   - ~~Implement the storage interface for DynamoDB~~
   - ~~Create helper functions for key generation~~
   - ~~Add connection pooling~~

2. [x] ~~**HTTP Signatures Package**~~ ✅ COMPLETED
   - ~~Signature verification for incoming requests~~
   - ~~Signature generation for outgoing requests~~
   - ~~Key management~~

3. [ ] **Remaining Storage Operations**
   - Object storage (Notes, Articles, etc.)
   - Relationship operations (follows)
   - Collection operations
   - DynamoDB transactions for atomic operations

4. [ ] **Actor Profile Endpoint** (cmd/actor)
   - GET handler for actor profiles
   - Content negotiation (HTML vs ActivityStreams)
   - Public key serving
   - Connect to DynamoDB storage

5. [ ] **Inbox Endpoint** (cmd/inbox)
   - POST handler for receiving activities
   - HTTP signature verification using federation package
   - Activity validation
   - Queue activities for processing

6. [ ] **Outbox Endpoint** (cmd/outbox)
   - GET handler for public activities
   - POST handler for creating activities
   - Activity validation

7. [ ] **Pulumi Infrastructure**
   - DynamoDB table definitions
   - Lambda function deployments
   - API Gateway configuration
   - IAM roles and policies

## Architecture Decisions 📋

### Key Design Choices Made:
1. **Single DynamoDB Table Design** - Using composite keys for efficient queries
2. **Lambda Per Endpoint** - Each ActivityPub endpoint gets its own Lambda for isolation
3. **Background Processing via SQS** - Decouple activity processing from HTTP requests
4. **JWT for Client Auth** - Simple client authentication separate from federation
5. **Zap for Logging** - Fast, structured logging optimized for Lambda
6. **No Heavy Frameworks** - Direct Lambda handlers to minimize cold starts
7. **Table-Driven Tests** - Comprehensive test coverage with clear test cases
8. **RSA-SHA256 for HTTP Signatures** - Industry standard algorithm for federation

### Open Questions:
1. Should we use DynamoDB Streams or SQS for activity delivery?
2. How should we handle media storage lifecycle policies?
3. Should we implement a shared inbox for efficiency?
4. Should we add support for Ed25519 signatures (more efficient)?

## Testing Strategy 🧪

### Unit Tests Completed:
- [x] ActivityPub validation functions
- [x] WebFinger parsing
- [x] URL validation
- [x] HTML sanitization
- [x] **DynamoDB Actor Operations** - All CRUD operations with mocked client
- [x] **DynamoDB Activity Operations** - Create and query operations
- [x] **HTTP Signature Operations** - Parsing, verification, generation, key management

### Unit Tests Needed:
- [ ] ActivityPub type marshaling/unmarshaling
- [ ] DynamoDB object and relationship operations
- [ ] Common response utilities

### Integration Tests Completed:
- [x] **DynamoDB Actor Lifecycle** - Full CRUD cycle against local DynamoDB
- [x] **DynamoDB Activity Pagination** - Cursor-based pagination testing
- [x] **HTTP Signature End-to-End** - Sign and verify full cycle

### Integration Tests Needed:
- [ ] End-to-end federation test with Mastodon
- [ ] Activity delivery reliability
- [ ] Media upload and serving

## Code Quality 📊

### Standards Established:
- **Naming Conventions**: camelCase for functions/variables, PascalCase for types
- **Error Handling**: Domain-specific error types with proper error wrapping
- **Logging**: Structured logging with request IDs and context
- **Testing**: Table-driven tests with descriptive names
- **Documentation**: Comprehensive godoc comments

### Linting & Formatting:
- `gofmt` for consistent formatting
- `golangci-lint` for code quality checks
- All tests passing ✅

## Known Limitations 🚨

1. **Partial Storage Implementation** - Actor and Activity storage complete, but Object/Relationship/Collection operations pending
2. **No Authentication** - No way to create or authenticate users
3. **No Activity Processing** - Activities are received but not processed
4. **No Federation** - Can't actually communicate with other servers yet (but HTTP signatures are ready!)
5. **No KMS Encryption** - Private keys stored in plaintext (TODO: AWS KMS integration)
6. **GetActivity Uses Scan** - Should optimize with better key design or GSI
7. **HTTP Signatures RSA Only** - Ed25519 support planned for future

## Resources 📚

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [WebFinger RFC 7033](https://tools.ietf.org/html/rfc7033)
- [HTTP Signatures Draft](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures)
- [Mastodon ActivityPub Implementation](https://docs.joinmastodon.org/spec/activitypub/) 