# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Lesser is a complete serverless ActivityPub implementation. It provides:
- Full Mastodon API compatibility (100% of v1 endpoints)
- Complete ActivityPub federation protocol implementation
- GraphQL API with 60 operations
- WebSocket streaming support
- 1/100th the operational cost of traditional hosting solutions

## Key Development Commands

### Building
```bash
make build          # Build all Lambda functions
make build-lambdas  # Build Lambda deployment packages
make generate       # Generate GraphQL code
```

### Testing
```bash
make test           # Run all Go unit tests
make test-api       # Run Python API tests
make test-federation # Run federation tests
make test-search    # Run search tests
make test-ai        # Run AI integration tests
make test-auth      # Run authentication tests
make test-load      # Run k6 load tests
```

### Development
```bash
make dev            # Run local development server
make fmt            # Format Go code
make lint           # Run linters
make clean          # Clean build artifacts
```

### Deployment
```bash
make deploy         # Deploy with Pulumi
make logs           # Tail Lambda logs
make pulumi-up      # Deploy infrastructure
make pulumi-destroy # Destroy infrastructure
```

### Load Testing
```bash
make k6-auth        # Test auth endpoints
make k6-timeline    # Test timeline performance
make k6-posting     # Test post creation
make k6-federation  # Test federation
```

## Architecture Overview

### Serverless Design
- **23 Lambda Functions**: Each function handles specific responsibilities
- **DynamoDB**: Single-table design with 8 GSIs for efficient queries
- **S3**: Media storage with CloudFront CDN
- **SQS**: Async job processing

### Key Lambda Functions
- `api`: Main REST API handler (Mastodon-compatible)
- `graphql`: GraphQL API server
- `auth` & `auth-api`: Authentication services
- `inbox`/`outbox`: ActivityPub federation endpoints
- `processor-*`: Async processors for various tasks

### Directory Structure
```
/cmd/          # Lambda function entry points (23 services)
/pkg/          # Core business logic and packages
  /activitypub/  # Protocol implementation
  /storage/      # DynamoDB data layer
  /auth/         # Authentication (OAuth, WebAuthn, wallet)
  /federation/   # Federation routing and delivery
  /ai/           # AWS Bedrock integration
/infra/        # Pulumi infrastructure as code
/tests/        # Python integration tests
/graph/        # GraphQL schema and resolvers
/docs/         # Documentation
```

## Important Design Patterns

### DynamoDB Single-Table Design
- All data in one table with composite keys
- 8 GSIs for different access patterns
- Careful attention to hot partition avoidance
- Cost tracking on every DB operation

### Cost Tracking
- Every DynamoDB operation tracks consumed capacity
- Real-time cost monitoring via context
- Aggregated cost reporting
- Target: < $0.01 per user per month

### Lambda Considerations
- **Stateless**: No shared memory between invocations
- **Cold Starts**: Keep Lambda packages small
- **Timeouts**: 30s API Gateway limit
- **Memory**: Cost vs performance optimization

## API Information

### REST API (Mastodon v1)
- Base path: `/api/v1/`
- Full compatibility with Mastodon clients
- OAuth 2.0 authentication
- WebSocket streaming at `/api/v1/streaming`

### GraphQL API
- Endpoint: `/graphql`
- 60 operations (queries, mutations, subscriptions)
- DataLoader for N+1 query prevention
- Real-time subscriptions

### Key Endpoints
- `/inbox` - ActivityPub inbox (federation)
- `/outbox` - ActivityPub outbox
- `/.well-known/webfinger` - WebFinger discovery
- `/nodeinfo` - Instance information

## Development Guidelines

### When Making Changes
1. **Lambda Functions**: Keep them focused and small
2. **DynamoDB**: Always consider cost and hot partitions
3. **Federation**: Test with real ActivityPub instances
4. **Authentication**: Support OAuth, WebAuthn, and wallet
5. **Cost Tracking**: Add tracking to new DB operations

### Common Pitfalls
- Don't assume Lambda persistence between invocations
- Always handle DynamoDB throttling gracefully
- Test federation with multiple server implementations
- Consider API Gateway 30s timeout limit
- Remember S3 eventual consistency

### Security Considerations
- Never log sensitive data (tokens, keys)
- Always validate ActivityPub signatures
- Use CSRF protection on state-changing operations
- Sanitize all user input
- Rate limit by user, not IP (Lambda shares IPs)

## Environment Configuration

Key environment variables needed:
- `DOMAIN_NAME`: Your instance domain
- `AWS_REGION`: AWS region for resources
- `DYNAMODB_TABLE`: Main DynamoDB table name
- `PRIVATE_KEY_SECRET`: ActivityPub signing key
- `OAUTH_*`: OAuth provider credentials

## AI Development Methodology

Lesser was built using a "chunking" methodology:
1. **Deep-First**: Complete one feature entirely before moving to the next
2. **Three-Tab Model**: README (vision), STATE (progress), ARCHITECTURE (design)
3. **No Placeholders**: Every function works or doesn't exist
4. **Continuous Testing**: API tests run after each chunk

## Current Git Status

Branch: main
Modified files:
- pkg/auth/webauthn.go
- pkg/storage/dynamodb/auth.go
- docs/greater/ (untracked)

## Project References

- **docs/greater**: The client application developed independent of greater, this is available for reference only

## Testing Notes

- Python tests use pytest and require `pip install -r requirements.txt`
- Load tests use k6 and test real performance characteristics
- Federation tests require ngrok or public endpoint
- All tests can run against local or deployed instances

## Storage Access Patterns (CRITICAL)

### DynamORM/Lift Migration Status
We are in Phase 4 of migrating from direct DynamoDB SDK usage to DynamORM with Lift framework. This is a critical architectural change.

### Correct Storage Implementation Patterns

**NEVER use direct DynamoDB SDK:**
```go
// ❌ WRONG - Never do this
import "github.com/aws/aws-sdk-go-v2/service/dynamodb"
result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{...})
```

**ALWAYS use DynamORM patterns:**
```go
// ✅ CORRECT - Use DynamORM
var model models.User
err := r.db.WithContext(ctx).Model(&models.User{}).
    Where("PK", "=", fmt.Sprintf("USER#%s", username)).
    Where("SK", "=", "PROFILE").
    First(&model)
```

### Key Patterns That Must Be Preserved
- **Users**: PK=`USER#username`, SK=`PROFILE`
- **Actors**: PK=`ACTOR#username`, SK=`PROFILE`
- **Objects**: PK=`object#id`, SK=`object#id`
- **DNS Cache**: PK=`DNSCACHE#hostname`, SK=`ENTRY`
- **Reputation**: PK=`ACTOR#username`, SK=`REP#timestamp`
- **Vouch**: PK=`VOUCH#id`, SK=`METADATA`
- **Trust**: PK=`TRUST#trusterID#category`, SK=`TRUSTEE#trusteeID`
- **Account Pins**: PK=`ACCOUNT_PIN#username`, SK=`PIN#pinnedActorID`
- **Account Notes**: PK=`ACCOUNT_NOTE#username`, SK=`NOTE#targetActorID`

### Repository Pattern Requirements
1. All storage access goes through repository interfaces in StorageAdapter
2. Repositories use DynamORM models with proper tags
3. StorageAdapter bridges storage.Storage interface to repositories
4. NO direct calls to originalStorage in StorageAdapter (this is an architectural violation)

## Working with AI Agents (lift-dynamorm-expert)

### CRITICAL: Agent Implementation Verification

When using the lift-dynamorm-expert agent for ANY implementation:

#### 1. Pre-Implementation Instructions Must Include:
- Exact legacy file paths to analyze
- Exact key patterns to preserve (with case sensitivity)
- Complete list of methods needing implementation
- Warning about NO AWS SDK usage
- Requirement to match legacy behavior exactly

#### 2. Post-Implementation Verification (MANDATORY):
```bash
# Verify no AWS SDK usage
grep -n "github.com/aws/aws-sdk-go" <file> | wc -l  # Must be 0
grep -n "dynamodb\." <file> | grep -v "//" | wc -l  # Must be 0

# Verify no originalStorage delegation
grep -n "originalStorage\." adapter.go | wc -l  # Must be 0

# Check model exists if creating new feature
ls pkg/storage/models/<feature>.go  # Must exist

# Check compilation
go build ./pkg/storage/...
```

#### 3. Key Pattern Verification:
- Compare EVERY key generation with legacy implementation
- Verify GSI keys match exactly (including case)
- Check TTL fields are preserved where used
- Ensure composite keys use correct separators

#### 4. Functionality Verification:
- Error handling must match legacy (nil vs error)
- Not found cases must return same as legacy
- All struct fields must be mapped correctly
- GSI queries must use correct index names

### Common Agent Mistakes to ALWAYS Check:
1. **Wrong Key Case**: `actor#id` instead of `ACTOR#id`
2. **Missing UpdateKeys()**: Not updating GSI keys in models
3. **AWS SDK Usage**: Using AWS SDK instead of DynamORM
4. **Missing TTL**: Not preserving TTL/expiration logic
5. **Wrong Error Returns**: Returning error where legacy returns nil
6. **Interface Mismatch**: Repository methods don't match adapter calls
7. **Missing Fields**: Model missing fields that legacy uses
8. **Wrong GSI Names**: Using incorrect GSI index names

### Proper Agent Instruction Template:
```
CRITICAL: Implement [Feature] using DynamORM/Lift patterns ONLY

1. Analyze legacy implementation:
   - File: /pkg/storage/dynamodb/[file].go
   - Document ALL key patterns used
   - List ALL DynamoDB operations

2. Create model at: /pkg/storage/models/[name].go
   - Use EXACT key patterns: PK=X, SK=Y (preserve case!)
   - Include ALL fields from legacy
   - Add UpdateKeys() method if GSIs used
   - Use proper DynamORM tags

3. Add methods to [Repository] interface in adapter.go
   - Match exact signatures from Storage interface

4. Implement in /pkg/storage/repositories/[repo].go
   - Use DynamORM ONLY (no AWS SDK imports)
   - Match legacy logic EXACTLY
   - Preserve ALL error handling behavior
   - Use zap.Logger for logging

5. Verify implementation:
   - No AWS imports
   - No dynamodb. usage  
   - Compilation succeeds
   - Keys match legacy exactly
   - All methods implemented
```

### Post-Agent Review Checklist:
- [ ] Read the implementation line by line
- [ ] Compare with legacy implementation
- [ ] Run all verification commands
- [ ] Check key patterns match exactly
- [ ] Verify no AWS SDK usage
- [ ] Ensure compilation succeeds
- [ ] Confirm all requested methods implemented
- [ ] Check error handling matches legacy

NEVER trust agent output without verification. ALWAYS compare with legacy implementation.