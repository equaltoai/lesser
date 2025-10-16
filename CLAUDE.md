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

# GraphQL 100% Completion Project - Management Framework

**Project Status**: Active Implementation  
**Current Completion**: 70% (60+ operations implemented)  
**Target**: 100% (36 missing operations)  
**Estimated Timeline**: 7 weeks (1 dev) or 5 weeks (2 devs)

---

## 📋 Project Structure

### Architecture Overview
```
graph/
├── schema.graphql          # Core + Lesser features (~90% complete)
├── phase2.graphql          # Federation features (~40% complete)
├── phase3.graphql          # Visualization/analytics (~20% complete)
├── schema.resolvers.go     # Auto-generated (12,662 lines) - Edit stubs here
├── resolver.go             # Root resolver setup
├── dataloader.go           # N+1 prevention
└── subscriptions.go        # WebSocket/real-time

pkg/services/              # Service layer (DI via Registry pattern)
├── accounts/             # Account queries
├── notes/                # Post/note operations
├── lists/                # List management
├── media/                # Media handling
├── notifications/        # Notification system
├── conversations/        # DM conversations
├── relationships/        # Follow/block relationships
├── quotes/               # Quote post feature
├── emoji/                # Custom emoji management
├── search/               # Full-text search
└── [NEW: hashtags/]      # To be created
└── [NEW: threads/]       # To be created
└── [NEW: severance/]     # To be created

pkg/storage/
├── models/               # DynamoDB data models
├── repositories/         # Database access layer
└── dynamorm/            # ORM implementation
```

### Tech Stack
- **GraphQL Engine**: gqlgen (v0.17.77)
- **Database**: DynamoDB + DynamORM
- **Real-time**: WebSocket subscriptions
- **Dependency Injection**: services.Registry pattern
- **Cost Tracking**: Integrated throughout
- **Error Handling**: Custom error types per package

---

## 🎯 Implementation Phases

### PHASE 1: Mastodon Parity (6-9 days) - STARTING NOW
**Goal**: Core Mastodon compatibility  
**Critical for**: User engagement, federation

#### 1.1 Hashtag Following System (2-3 days)
- **Operations**: 10 total (8 missing + 2 partial)
- **Status**: Stubs only - needs full implementation
- **Services Needed**: hashtags/service.go (create)
- **Key Files**:
  - Create: `pkg/services/hashtags/service.go`
  - Models: HashtagFollow, HashtagMute, HashtagStats
  - Repository: GetFollowedHashtags, FollowHashtag, etc.

#### 1.2 Thread Synchronization (3-4 days)
- **Operations**: 3 total (all missing)
- **Status**: Schema defined, no impl
- **Services Needed**: threads/service.go (create)
- **Key Files**:
  - Create: `pkg/services/threads/service.go`
  - Needs: Remote thread fetching via ActivityPub
  - Sync tracking & job queue

### PHASE 2: Federation & Monitoring (8-11 days)
**Goal**: Complete federation features  
**Critical for**: Instance health, cost tracking

Key areas:
- Phase 2 Alert Subscriptions (2 days)
- Media Streaming Completion (4-5 days)
- Severed Relationships (3-4 days)
- Advanced Moderation ML (3-4 days)

### PHASE 3: Visualization & Analytics (10-13 days)
**Goal**: Complete analytics and dashboards  
**Critical for**: Admin visibility

Key areas:
- Federation Graph Visualization (5-6 days)
- Streaming Analytics (3-4 days)
- Performance Monitoring (2-3 days)
- Moderation Dashboard (3-4 days)

---

## 🔧 Development Workflow

### For Agents: How to Implement Features

#### Step 1: Understand Resolver Pattern
```go
// Pattern in schema.resolvers.go
func (r *queryResolver) FeatureName(ctx context.Context, arg1 string) (*model.ReturnType, error) {
    // 1. Get service from registry
    svc := r.Registry.YourService()
    
    // 2. Validate inputs
    if err := common.ValidateRequiredParam("arg1", arg1); err != nil {
        return nil, err
    }
    
    // 3. Call service
    result, err := svc.DoSomething(ctx, arg1)
    if err != nil {
        r.Logger.Error("Failed", zap.Error(err))
        return nil, err
    }
    
    // 4. Return (converted if needed)
    return result, nil
}
```

#### Step 2: Create/Update Service
```go
// pkg/services/yourservice/service.go
type Service interface {
    DoSomething(ctx context.Context, arg string) (*Result, error)
}

type serviceImpl struct {
    repos repositories.Container
    // other deps
}

func (s *serviceImpl) DoSomething(ctx context.Context, arg string) (*Result, error) {
    // Implementation
}
```

#### Step 3: Add Storage Models
Models go in `pkg/storage/models/` and implement:
- DynamoDB key structure (PK/SK)
- Tags for field mapping
- Marshal/Unmarshal as needed

#### Step 4: Add Repository Methods
In `pkg/storage/repositories/` - use existing patterns:
- BaseModel for common CRUD
- Custom queries as needed
- Cost tracking via repositories

#### Step 5: Register in Service Registry
In `pkg/services/registry.go`, add method to return your service.

---

## 📊 Gap Analysis Summary

### Critical Gaps (Blocking 100%)
| Area | Ops | Status | Priority |
|------|-----|--------|----------|
| Hashtag Following | 8 | Stub | 🔴 CRITICAL |
| Thread Sync | 3 | Missing | 🔴 CRITICAL |
| Severed Relationships | 4 | Missing | 🔴 CRITICAL |
| Media Streaming | 3 | Partial | 🟡 HIGH |
| Moderation ML | 2 | Partial | 🟡 HIGH |
| Phase 2 Subscriptions | 4 | Missing | 🟡 HIGH |
| Federation Visualization | 3 | Empty | 🟢 MEDIUM |
| Streaming Analytics | 3 | Missing | 🟢 MEDIUM |
| Moderation Dashboard | 3 | Missing | 🟢 MEDIUM |
| Performance Monitoring | 3 | Stub | 🟢 MEDIUM |

**Total**: 36 missing/partial operations

---

## 🚀 How to Use This Framework

### For Project Manager (AI Assistant)
1. **Plan Phase**: Review gaps, prioritize work
2. **Create Agent Prompts**: Break work into specific tasks
3. **Review Progress**: Validate implementations against acceptance criteria
4. **Report Status**: Update this document and plan next phase
5. **Iterate**: Continue until 100% complete

### For Development Agent
1. **Receive Prompt**: Get specific feature to implement
2. **Reference**: This document + schema + existing patterns
3. **Implement**: Follow resolver pattern guide
4. **Test**: Add unit + integration tests
5. **Report**: Show code + test results

### Key Files to Reference
- Schema: `/graph/schema.graphql` (core) + `/graph/phase2.graphql` + `/graph/phase3.graphql`
- Example Resolvers: Look at `Hashtag`, `Timeline`, `Notifications` in schema.resolvers.go
- Example Service: `/pkg/services/notes/service.go`
- Example Models: `/pkg/storage/models/note.go`
- Error Pattern: `/pkg/services/errors.go`

---

## ✅ Acceptance Criteria Format

Each feature should meet:
```
FUNCTIONALITY
- [ ] All operations implemented
- [ ] Parameters validated
- [ ] Error handling complete
- [ ] Edge cases handled

TESTING
- [ ] Unit tests (80%+ coverage)
- [ ] Integration tests
- [ ] Edge case tests
- [ ] Error scenario tests

CODE QUALITY
- [ ] Follows resolver pattern
- [ ] Uses service registry
- [ ] Proper error types
- [ ] Cost tracking integrated
- [ ] Logging in place

DOCUMENTATION
- [ ] Comments added
- [ ] Schema docs updated
- [ ] Integration guide written (if needed)
```

---

## 📝 Session Management

### Starting a New Implementation Task

**Agent Prompt Template:**
```
Implement [Feature Name] in Lesser GraphQL.

CONTEXT:
- Current phase: [Phase]
- Estimate: [Time]
- Dependencies: [List]
- Operations: [Count + Names]

REQUIREMENTS:
- [Specific req 1]
- [Specific req 2]
- ...

ACCEPTANCE CRITERIA:
- [ ] All X operations working
- [ ] Tests with Y% coverage
- [ ] No regressions

REFERENCE FILES:
- Schema: /graph/schema.graphql
- Example service: /pkg/services/notes/service.go
- Error handling: /pkg/services/errors.go
```

### Reporting Progress

After implementation:
```
IMPLEMENTATION COMPLETE: [Feature Name]

CHANGES:
- Created/Modified: [List of files]
- Operations implemented: [Count]
- Tests added: [Count]

VERIFICATION:
- Unit tests: ✅ Pass (X/X)
- Integration tests: ✅ Pass (X/X)
- Coverage: X%

STATUS: Ready for phase [X]/next feature
```

---

## 🔍 Code Review Checklist

When evaluating implementations:
- [ ] Follows gqlgen resolver pattern
- [ ] Uses services.Registry for DI
- [ ] Proper error handling with custom types
- [ ] Input validation via common package
- [ ] Logging at appropriate levels
- [ ] Cost tracking integrated if applicable
- [ ] Tests cover success + error cases
- [ ] No N+1 query issues (uses dataloaders)
- [ ] Pagination handled correctly (cursor-based)
- [ ] Schema matches implementation

---

## 📚 Quick Reference

### Resolver Signatures
```go
// Queries
func (r *queryResolver) Field(ctx context.Context, args...) (*Type, error)

// Mutations  
func (r *mutationResolver) Action(ctx context.Context, input Input) (*Payload, error)

// Subscriptions
func (r *subscriptionResolver) Stream(ctx context.Context, args...) (<-chan *Update, error)
```

### Service Pattern
```go
type Service interface {
    Method(ctx context.Context, args...) (*Result, error)
}

func New(repos repositories.Container, logger *zap.Logger) Service {
    return &serviceImpl{repos: repos, logger: logger}
}
```

### Model Pattern
```go
type Model struct {
    PK       string `json:"PK" dynamodbav:"PK"`      // Partition key
    SK       string `json:"SK" dynamodbav:"SK"`      // Sort key
    Data     string `json:"data" dynamodbav:"data"`
    CreatedAt time.Time `json:"createdAt" dynamodbav:"createdAt"`
}
```

---

## 🎓 Learning Resources

- **ActivityPub Spec**: https://www.w3.org/TR/activitypub/
- **Mastodon API**: https://docs.joinmastodon.org/api/
- **GraphQL Best Practices**: https://graphql.org/learn/best-practices/
- **gqlgen Docs**: https://gqlgen.com/
- **Existing Examples**: See schema.resolvers.go for 60+ implemented operations
- **Architecture**: Read `/docs/architecture/SYSTEM_DESIGN.md`

---

## 📞 Status Dashboard (Updated October 15, 2025)

### Phase 1: Mastodon Parity
| Feature | Status | Work Breakdown | Timeline |
|---------|--------|---|---|
| 1.1a Hashtag Following | 🟡 In Progress | Issues #1-4 remediation | 2.5-3 hours |
| 1.1b Hashtag Subs | 🟡 In Progress | Phase 1.1.1 Blocker | 4-6 hours |
| Thread Sync | 🔴 Ready to Start | 3 operations | 3-4 days |
| **TOTAL Phase 1** | **🟡 75%** | **5 operations done, 3 queued** | **~1 week remaining** |

### Phase 2: Federation & Monitoring
| Feature | Status | Completed | Estimate |
|---------|--------|-----------|----------|
| Phase 2 Subscriptions | 🔴 Not Started | 0% | 2d |
| Media Streaming | 🟡 Partial | 40% | 4-5d |
| Severed Relationships | 🔴 Not Started | 0% | 3-4d |
| Moderation ML | 🟡 Partial | 20% | 3-4d |
| **TOTAL** | | **15%** | **12-17d** |

### Phase 3: Visualization & Analytics
| Feature | Status | Completed | Estimate |
|---------|--------|-----------|----------|
| Federation Graph | 🔴 Not Started | 0% | 5-6d |
| Streaming Analytics | 🔴 Not Started | 0% | 3-4d |
| Performance Monitoring | 🟡 Partial | 20% | 2-3d |
| Moderation Dashboard | 🔴 Not Started | 0% | 3-4d |
| **TOTAL** | | **5%** | **13-17d** |

### Overall Progress
- **Core (70%)**: ✅ Complete
- **Phase 1 (77%)**: 🟢 In Progress (10 of 13 operations - 5 done, 5 in remediation/blocking, 3 queued)
- **Phase 2 (15%)**: 🟡 Partial
- **Phase 3 (5%)**: 🔴 Not Started
- **Overall**: 70% → 75% (10 operations complete, 26 remaining)

---

## 📧 Contact & Escalation

For blockers or questions:
1. Check existing patterns in `schema.resolvers.go`
2. Review similar service implementation
3. Check schema files for type definitions
4. Review error handling patterns in pkg files