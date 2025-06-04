# AI Assistant Prompt for Lesser Development

You are an expert Go developer specializing in serverless architectures and federated social networking protocols. You will be helping to build **Lesser**, a cost-effective serverless ActivityPub implementation using AWS Lambda and DynamoDB.

## Project Overview

Lesser is a serverless ActivityPub server designed to minimize hosting costs while providing full ActivityPub compliance. Instead of traditional always-on servers, it uses:
- AWS Lambda for compute (pay per request)
- DynamoDB for storage (pay per use)
- API Gateway for HTTP endpoints
- S3 for media storage
- Pulumi for infrastructure as code

The goal is to make hosting an ActivityPub instance affordable for individuals and small communities (estimated ~$23/month for 100 users).

## Current Project State

### Completed ✅
1. **Architecture Design** (see DESIGN.md)
   - Single DynamoDB table design with composite keys
   - Lambda per endpoint pattern
   - Event-driven architecture with DynamoDB Streams

2. **Developer Guidelines** (see DEVELOPER_GUIDELINES.md)
   - Technology choices: zap for logging, no heavy frameworks
   - Naming conventions and code organization
   - Testing strategy with examples

3. **Core Packages**
   - `pkg/activitypub/` - ActivityPub types and validation
   - `pkg/config/` - Environment-based configuration
   - `pkg/common/` - Logging, errors, and response utilities

4. **DynamoDB Storage Layer** ✅
   - `pkg/storage/dynamodb/client.go` - Connection pooling, initialization
   - `pkg/storage/dynamodb/actor.go` - Full actor CRUD operations
   - `pkg/storage/dynamodb/activity.go` - Activity storage with pagination
   - Comprehensive unit and integration tests
   - >80% test coverage

5. **HTTP Signatures Package** ✅
   - `pkg/federation/httpsig.go` - HTTP signature verification and generation
   - RSA-SHA256 algorithm support
   - Timestamp validation (±5 minutes)
   - Digest calculation and verification
   - Key management utilities (RSA key generation, PEM encoding/decoding)
   - 87.4% test coverage
   - Ready for integration with endpoints

6. **Actor Profile Endpoint** ✅
   - `cmd/actor/main.go` - GET /users/{username} handler
   - Content negotiation (ActivityStreams JSON vs HTML)
   - Beautiful responsive HTML profile pages
   - Public key serving for HTTP signatures
   - 95.5% test coverage
   - Full DynamoDB integration

7. **WebFinger Endpoint** ✅
   - `cmd/webfinger/main.go` - Discovery endpoint
   - Now connected to DynamoDB storage
   - Complete discovery flow working

8. **Inbox Endpoint** ✅
   - `cmd/inbox/main.go` - POST /users/{username}/inbox handler
   - HTTP signature verification using federation package
   - Activity validation with proper error messages
   - Addressing verification (to, cc, bto, bcc)
   - Storage of activities in DynamoDB
   - Comprehensive test suite with mocked HTTP server
   - 79.5% test coverage

9. **Outbox Endpoint** ✅
   - `cmd/outbox/main.go` - Handles both POST and GET requests
   - POST: Create activities with validation and auto-ID generation
   - GET: Retrieve activities with OrderedCollection/OrderedCollectionPage
   - Cursor-based pagination with configurable limits
   - Public access for GET (no auth required)
   - 81.7% test coverage

10. **Activity Processor** ✅
    - `cmd/activity-processor/main.go` - DynamoDB Streams handler
    - Processes both inbox and outbox activities
    - Inbox: Handles Follow, Accept, Create activities
    - Outbox: Delivers activities to remote servers
    - HTTP signature signing for outgoing requests
    - Recipient resolution and filtering
    - 78.5% test coverage

### Federation Status 🚀
**Lesser is now a functioning ActivityPub server with full outbox support!**

The complete federation flow is operational:
1. **Discovery**: Remote servers can find actors via WebFinger
2. **Profile Exchange**: Actors serve public keys for authentication
3. **Receive Activities**: Inbox accepts and verifies activities
4. **Create Activities**: Outbox accepts activities from local users
5. **Retrieve Activities**: Outbox serves activities with pagination
6. **Process Activities**: Activity Processor handles follows, accepts, etc.
7. **Deliver Activities**: Activities are signed and sent to remote servers

What's working:
- ✅ Remote servers can discover and follow local users
- ✅ Local activities are delivered to followers
- ✅ HTTP signatures authenticate all federation
- ✅ Follow/Accept flow creates relationships
- ✅ Outbox browsing with standard ActivityPub format

### Partially Complete 🚧
1. **Storage Operations**
   - ✅ Actor operations (CRUD)
   - ✅ Activity operations (outbox/inbox with pagination)
   - ❌ Object operations (Notes, Articles, etc.)
   - ❌ Relationship operations (follows) - partially implemented in processor
   - ❌ Collection operations

### Important Architectural Decisions
See `ARCHITECTURE_DECISIONS.md` for details:
- **Private Key Encryption**: AWS KMS (pending implementation)
- **Client Authentication**: OAuth 2.0 with PKCE (currently no auth)
- **Activity Delivery**: DynamoDB Streams → Lambda ✅ IMPLEMENTED

## Your Task

Continue development by implementing the **Collections Endpoints** (`cmd/collections`), which will provide followers and following lists for actors.

### 1. Create Collections Handler
Create `cmd/collections/main.go` that handles:
- `GET /users/{username}/followers` - Returns followers collection
- `GET /users/{username}/following` - Returns following collection
- Support OrderedCollection format
- Implement cursor-based pagination
- Public access (no auth required initially)

### 2. Implement Storage Operations
First, you'll need to implement relationship storage in the storage layer:
```go
// In pkg/storage/interface.go
type Relationship struct {
    FollowerUsername string
    FollowedUsername string
    State           string // "pending", "accepted", "rejected"
    FollowActivityID string
    CreatedAt       time.Time
    AcceptedAt      *time.Time
}

// Add to Storage interface:
GetFollowers(ctx context.Context, username string, cursor string, limit int) ([]*Relationship, string, error)
GetFollowing(ctx context.Context, username string, cursor string, limit int) ([]*Relationship, string, error)
CreateRelationship(ctx context.Context, rel *Relationship) error
UpdateRelationship(ctx context.Context, rel *Relationship) error
```

### 3. DynamoDB Schema for Relationships
Implement in `pkg/storage/dynamodb/relationships.go`:
```go
// Primary access pattern:
PK: FOLLOW#{follower_username}
SK: FOLLOWING#{followed_username}

// Reverse lookup via GSI:
GSI1PK: FOLLOW#{followed_username}
GSI1SK: FOLLOWER#{follower_username}
```

### 4. Collections Response Format
Return proper ActivityStreams collections:
```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://example.com/users/alice/followers",
  "type": "OrderedCollection",
  "totalItems": 150,
  "first": "https://example.com/users/alice/followers?page=true"
}
```

With pagination:
```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://example.com/users/alice/followers?page=true",
  "type": "OrderedCollectionPage",
  "partOf": "https://example.com/users/alice/followers",
  "totalItems": 150,
  "next": "https://example.com/users/alice/followers?page=true&cursor=xyz",
  "orderedItems": [
    "https://remote.example/users/bob",
    "https://another.example/users/carol"
  ]
}
```

### 5. Update Activity Processor
Modify the activity processor to actually create/update relationships:
- When processing Follow in inbox: Create pending relationship
- When processing Accept in inbox: Update relationship to accepted
- When sending Accept from outbox: Update local relationship

### 6. Write Comprehensive Tests
- Test followers and following endpoints
- Test pagination with cursors
- Test empty collections
- Test relationship storage operations
- Mock storage for handler tests

## Example Implementation Pattern

```go
package main

import (
    "context"
    "encoding/json"
    "errors"
    "net/http"
    "strconv"
    
    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aron23/lesser/pkg/activitypub"
    "github.com/aron23/lesser/pkg/common"
    "github.com/aron23/lesser/pkg/config"
    "github.com/aron23/lesser/pkg/storage"
    "github.com/aron23/lesser/pkg/storage/dynamodb"
    "go.uber.org/zap"
)

var (
    cfg    *config.Config
    store  storage.Storage
    logger *zap.Logger
)

func init() {
    cfg = config.Get()
    logger = common.Logger()
    
    var err error
    store, err = dynamodb.New()
    if err != nil {
        logger.Fatal("failed to initialize storage", zap.Error(err))
    }
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    log := common.WithContext(ctx)
    
    // Only accept GET requests
    if request.HTTPMethod != http.MethodGet {
        return common.MethodNotAllowed(request.HTTPMethod), nil
    }
    
    username := request.PathParameters["username"]
    if username == "" {
        return common.BadRequest(errors.New("missing username")), nil
    }
    
    // Determine collection type from path
    collectionType := request.PathParameters["collection"]
    if collectionType != "followers" && collectionType != "following" {
        return common.NotFound(errors.New("unknown collection")), nil
    }
    
    // Check if actor exists
    _, err := store.GetActor(ctx, username)
    if err != nil {
        if common.IsNotFound(err) {
            return common.NotFound(err), nil
        }
        return common.InternalServerError(err), nil
    }
    
    // Parse query parameters
    isPage := request.QueryStringParameters["page"] == "true"
    cursor := request.QueryStringParameters["cursor"]
    limit := 20 // default
    if l := request.QueryStringParameters["limit"]; l != "" {
        if parsed, err := strconv.Atoi(l); err == nil && parsed >= 1 && parsed <= 100 {
            limit = parsed
        }
    }
    
    if !isPage {
        // Return collection metadata
        return returnCollection(username, collectionType)
    }
    
    // Get relationships based on type
    var relationships []*storage.Relationship
    var nextCursor string
    
    if collectionType == "followers" {
        relationships, nextCursor, err = store.GetFollowers(ctx, username, cursor, limit)
    } else {
        relationships, nextCursor, err = store.GetFollowing(ctx, username, cursor, limit)
    }
    
    if err != nil {
        log.Error("failed to get relationships", 
            zap.String("type", collectionType),
            zap.Error(err))
        return common.InternalServerError(err), nil
    }
    
    // Build and return page
    return returnCollectionPage(username, collectionType, relationships, cursor, nextCursor, limit)
}

func main() {
    lambda.Start(handler)
}
```

## Success Criteria

- [ ] Followers and following endpoints working
- [ ] Relationship storage implemented
- [ ] Pagination with cursors
- [ ] OrderedCollection format correct
- [ ] Activity processor creates relationships
- [ ] Good test coverage (>80%)

## Next Steps After This Task

1. **OAuth 2.0**: Implement authentication for local users
2. **Objects**: Store and serve Notes, Articles
3. **GET Inbox**: Allow viewing inbox (with auth)
4. **Media**: Handle image uploads
5. **Pulumi**: Deploy infrastructure

The Collections endpoints are important because they:
- Enable users to see their social graph
- Allow remote servers to discover relationships
- Complete the basic social features
- Required for proper ActivityPub compliance

Begin by implementing the relationship storage operations, then create the handler for serving collections. 