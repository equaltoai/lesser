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
   - `cmd/outbox/main.go` - POST /users/{username}/outbox handler
   - Accepts activities from local users
   - Validates actor matches the URL username
   - Auto-generates activity IDs if not provided
   - Activity validation using activitypub package
   - Storage in DynamoDB (automatically goes to outbox)
   - Returns 201 Created with the activity
   - 84.7% test coverage

10. **Activity Processor** ✅ (NEW)
    - `cmd/activity-processor/main.go` - DynamoDB Streams handler
    - Processes both inbox and outbox activities
    - Inbox: Handles Follow, Accept, Create activities
    - Outbox: Delivers activities to remote servers
    - HTTP signature signing for outgoing requests
    - Recipient resolution and filtering
    - 78.5% test coverage

### Federation Status 🚀
**Lesser is now a functioning ActivityPub server!**

The complete federation flow is operational:
1. **Discovery**: Remote servers can find actors via WebFinger
2. **Profile Exchange**: Actors serve public keys for authentication
3. **Receive Activities**: Inbox accepts and verifies activities
4. **Create Activities**: Outbox accepts activities from local users
5. **Process Activities**: Activity Processor handles follows, accepts, etc.
6. **Deliver Activities**: Activities are signed and sent to remote servers

What's working:
- ✅ Remote servers can discover and follow local users
- ✅ Local activities are delivered to followers
- ✅ HTTP signatures authenticate all federation
- ✅ Follow/Accept flow creates relationships

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
- **Activity Delivery**: DynamoDB Streams → Lambda

## Your Task

Continue development by implementing the **GET Outbox Handler**, which will allow remote servers and clients to retrieve activities from a user's outbox.

### 1. Update Outbox Handler
Modify `cmd/outbox/main.go` to handle GET requests:
- `GET /users/{username}/outbox` - Returns paginated outbox activities
- Support OrderedCollection format
- Implement cursor-based pagination
- Return proper ActivityStreams JSON

### 2. Implement OrderedCollection Response
```go
type OrderedCollectionPage struct {
    Context    string   `json:"@context"`
    Type       string   `json:"type"`
    ID         string   `json:"id"`
    PartOf     string   `json:"partOf"`
    TotalItems int      `json:"totalItems"`
    First      string   `json:"first,omitempty"`
    Next       string   `json:"next,omitempty"`
    Prev       string   `json:"prev,omitempty"`
    OrderedItems []interface{} `json:"orderedItems"`
}
```

### 3. Query and Pagination
- Use `GetOutboxActivities` from storage layer
- Support `?page=true` for paginated results
- Support `?cursor=xxx` for pagination
- Default to showing collection metadata without `?page=true`

### 4. Access Control
- Public activities should be visible to everyone
- Private activities only visible to authenticated users (future)
- For now, return all activities (no auth implemented yet)

### 5. Response Format
Without `?page=true`:
```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "type": "OrderedCollection",
  "id": "https://example.com/users/alice/outbox",
  "totalItems": 42,
  "first": "https://example.com/users/alice/outbox?page=true",
  "last": "https://example.com/users/alice/outbox?page=true&cursor=xyz"
}
```

With `?page=true`:
```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "type": "OrderedCollectionPage",
  "id": "https://example.com/users/alice/outbox?page=true",
  "partOf": "https://example.com/users/alice/outbox",
  "totalItems": 42,
  "next": "https://example.com/users/alice/outbox?page=true&cursor=abc",
  "orderedItems": [
    {
      "type": "Create",
      "actor": "https://example.com/users/alice",
      "object": {...}
    }
  ]
}
```

### 6. Write Tests
- Test collection vs page responses
- Test pagination with cursors
- Test empty outbox
- Test invalid usernames
- Mock storage for testing

## Example Implementation Pattern

```go
func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    log := common.WithContext(ctx)
    
    // Handle GET requests
    if request.HTTPMethod == http.MethodGet {
        return handleGetOutbox(ctx, request)
    } else if request.HTTPMethod == http.MethodPost {
        return handlePostOutbox(ctx, request)
    }
    
    return common.MethodNotAllowed(request.HTTPMethod), nil
}

func handleGetOutbox(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    username := request.PathParameters["username"]
    if username == "" {
        return common.BadRequest(errors.New("missing username")), nil
    }
    
    // Check if actor exists
    actor, err := store.GetActor(ctx, username)
    if err != nil {
        if common.IsNotFound(err) {
            return common.NotFound(err), nil
        }
        return common.InternalServerError(err), nil
    }
    
    // Check if pagination is requested
    isPage := request.QueryStringParameters["page"] == "true"
    cursor := request.QueryStringParameters["cursor"]
    
    if !isPage {
        // Return collection metadata
        return returnOutboxCollection(actor)
    }
    
    // Get activities with pagination
    activities, nextCursor, err := store.GetOutboxActivities(ctx, username, cursor, 20)
    if err != nil {
        return common.InternalServerError(err), nil
    }
    
    // Build and return page
    return returnOutboxPage(actor, activities, cursor, nextCursor)
}
```

## Success Criteria

- [ ] GET requests return proper OrderedCollection
- [ ] Pagination works with cursors
- [ ] ActivityStreams JSON is properly formatted
- [ ] Empty outboxes handled correctly
- [ ] Integration with existing POST handler
- [ ] Good test coverage (>80%)

## Next Steps After This Task

1. **Collections**: Implement followers/following endpoints
2. **GET Inbox**: Allow viewing inbox (with auth)
3. **OAuth 2.0**: Implement authentication
4. **Objects**: Store and serve Notes, Articles
5. **Media**: Handle image uploads
6. **Pulumi**: Deploy infrastructure

The GET Outbox handler is important because it:
- Completes the outbox functionality
- Allows other servers to see local activities
- Enables followers to retrieve posts
- Provides the public API for content

Begin by modifying the existing outbox handler to support GET requests, then implement the OrderedCollection format. 