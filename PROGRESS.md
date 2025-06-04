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

## In Progress 🚧

### Next Steps (Phase 1: Core ActivityPub)
1. [ ] **DynamoDB Storage Implementation**
   - Implement the storage interface for DynamoDB
   - Create helper functions for key generation
   - Add connection pooling

2. [ ] **HTTP Signatures Package**
   - Signature verification for incoming requests
   - Signature generation for outgoing requests
   - Key management

3. [ ] **Actor Profile Endpoint** (cmd/actor)
   - GET handler for actor profiles
   - Content negotiation (HTML vs ActivityStreams)
   - Public key serving

4. [ ] **Inbox Endpoint** (cmd/inbox)
   - POST handler for receiving activities
   - HTTP signature verification
   - Activity validation
   - Queue activities for processing

5. [ ] **Outbox Endpoint** (cmd/outbox)
   - GET handler for public activities
   - POST handler for creating activities
   - Activity validation

6. [ ] **Pulumi Infrastructure**
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

### Open Questions:
1. Should we use DynamoDB Streams or SQS for activity delivery?
2. How should we handle media storage lifecycle policies?
3. Should we implement a shared inbox for efficiency?

## Testing Strategy 🧪

### Unit Tests Completed:
- [x] ActivityPub validation functions
- [x] WebFinger parsing
- [x] URL validation
- [x] HTML sanitization

### Unit Tests Needed:
- [ ] ActivityPub type marshaling/unmarshaling
- [ ] HTTP signature verification
- [ ] DynamoDB key generation
- [ ] Common response utilities

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

1. **No Real Storage Yet** - All endpoints return mock data
2. **No Authentication** - No way to create or authenticate users
3. **No Activity Processing** - Activities are received but not processed
4. **No Federation** - Can't actually communicate with other servers yet

## Resources 📚

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [WebFinger RFC 7033](https://tools.ietf.org/html/rfc7033)
- [HTTP Signatures Draft](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures)
- [Mastodon ActivityPub Implementation](https://docs.joinmastodon.org/spec/activitypub/) 