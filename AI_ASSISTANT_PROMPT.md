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

5. **HTTP Signatures Package** (NEW ✅)
   - `pkg/federation/httpsig.go` - HTTP signature verification and generation
   - RSA-SHA256 algorithm support
   - Timestamp validation (±5 minutes)
   - Digest calculation and verification
   - Key management utilities (RSA key generation, PEM encoding/decoding)
   - 87.4% test coverage
   - Ready for integration with endpoints

6. **First Lambda Function**
   - `cmd/webfinger/` - WebFinger discovery endpoint (needs storage connection)

### Partially Complete 🚧
1. **Storage Operations**
   - ✅ Actor operations (CRUD)
   - ✅ Activity operations (outbox/inbox with pagination)
   - ❌ Object operations (Notes, Articles, etc.)
   - ❌ Relationship operations (follows)
   - ❌ Collection operations

### Important Architectural Decisions
See `ARCHITECTURE_DECISIONS.md` for details:
- **Private Key Encryption**: AWS KMS (pending implementation)
- **Client Authentication**: OAuth 2.0 with PKCE
- **Activity Delivery**: DynamoDB Streams → Lambda

## Your Task

Continue development by implementing the **Actor Profile Endpoint** (`cmd/actor`), which is the next critical component for federation. This endpoint will:
- Serve actor profiles with public keys (required for HTTP signature verification)
- Connect the DynamoDB storage to actual HTTP endpoints
- Support content negotiation (HTML vs ActivityStreams)

### 1. Create Actor Profile Handler
Create `cmd/actor/main.go` that handles:
- `GET /users/{username}` - Returns actor profile
- Content negotiation based on `Accept` header
- Integration with DynamoDB storage
- Proper error handling with common utilities

### 2. Implement Content Negotiation
The endpoint must support two response types:
1. **ActivityStreams JSON** (for federation)
   - When `Accept: application/activity+json` or `application/ld+json`
   - Include public key for HTTP signature verification
   - Follow ActivityPub actor format

2. **HTML** (for browsers)
   - Basic HTML profile page
   - Can be minimal for MVP
   - Include meta tags for discoverability

### 3. Connect to Storage
- Initialize DynamoDB storage client
- Fetch actor from storage
- Handle not found errors appropriately
- Use common error responses

### 4. Include Public Key
Actor responses must include the public key for federation:
```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "id": "https://example.com/users/alice",
  "type": "Person",
  "preferredUsername": "alice",
  "inbox": "https://example.com/users/alice/inbox",
  "outbox": "https://example.com/users/alice/outbox",
  "publicKey": {
    "id": "https://example.com/users/alice#main-key",
    "owner": "https://example.com/users/alice",
    "publicKeyPem": "-----BEGIN PUBLIC KEY-----\n..."
  }
}
```

### 5. Update WebFinger
Once the actor endpoint is working:
- Update `cmd/webfinger/main.go` to use real storage
- Ensure WebFinger returns correct actor URLs
- Test the discovery flow

## Example Implementation Pattern

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    
    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aron23/lesser/pkg/activitypub"
    "github.com/aron23/lesser/pkg/common"
    "github.com/aron23/lesser/pkg/config"
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
    
    // Initialize storage
    var err error
    store, err = dynamodb.New()
    if err != nil {
        logger.Fatal("failed to initialize storage", zap.Error(err))
    }
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    log := common.WithContext(ctx)
    
    // Extract username from path
    username := request.PathParameters["username"]
    if username == "" {
        return common.BadRequest(errors.New("missing username")), nil
    }
    
    log.Info("fetching actor profile",
        zap.String("username", username),
        zap.String("accept", request.Headers["Accept"]))
    
    // Get actor from storage
    actor, err := store.GetActor(ctx, username)
    if err != nil {
        if common.IsNotFound(err) {
            return common.NotFound(err), nil
        }
        log.Error("failed to get actor", zap.Error(err))
        return common.InternalServerError(err), nil
    }
    
    // Content negotiation
    accept := request.Headers["Accept"]
    if strings.Contains(accept, "application/activity+json") || 
       strings.Contains(accept, "application/ld+json") {
        // Return ActivityStreams JSON
        return common.ActivityPubResponse(http.StatusOK, actor), nil
    }
    
    // Return HTML (simplified for MVP)
    html := fmt.Sprintf(`
        <html>
        <head>
            <title>%s (@%s@%s)</title>
            <meta property="og:type" content="profile">
            <meta property="og:title" content="%s">
            <meta property="og:url" content="%s">
        </head>
        <body>
            <h1>%s</h1>
            <p>@%s@%s</p>
            <p>%s</p>
        </body>
        </html>`,
        actor.Name, actor.PreferredUsername, cfg.Domain,
        actor.Name, actor.ID, actor.Name,
        actor.PreferredUsername, cfg.Domain,
        actor.Summary)
    
    return events.APIGatewayProxyResponse{
        StatusCode: http.StatusOK,
        Headers: map[string]string{
            "Content-Type": "text/html; charset=utf-8",
        },
        Body: html,
    }, nil
}

func main() {
    lambda.Start(handler)
}
```

## Testing Strategy

Create `cmd/actor/handler_test.go` with:
- Unit tests for username extraction
- Tests for both JSON and HTML responses
- Error handling tests
- Mock storage for testing

## Success Criteria

- [ ] Actor profiles served correctly in ActivityStreams format
- [ ] Public key included in actor response
- [ ] Content negotiation working (JSON vs HTML)
- [ ] Storage integration working
- [ ] Error handling with proper status codes
- [ ] WebFinger updated to use real storage
- [ ] Unit tests with good coverage
- [ ] Manual testing with `curl` commands

## Next Steps After This Task

1. **Implement Inbox Handler**: Receive federated activities with HTTP signature verification
2. **Implement Outbox Handler**: Create and serve local activities
3. **Activity Processor**: Background processing with DynamoDB Streams
4. **Complete Storage Operations**: Objects, relationships, collections
5. **OAuth 2.0 Implementation**: Client authentication for posting

The Actor Profile endpoint is crucial because it:
- Enables other servers to discover your actors
- Provides public keys for signature verification
- Completes the basic federation discovery flow

Begin by creating the handler structure and connecting to the DynamoDB storage that's already implemented. 