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

### Federation Status ✅
The core federation flow is now operational:
1. **Discovery**: Remote server queries `/.well-known/webfinger?resource=acct:alice@example.com`
2. **Profile Fetch**: WebFinger returns actor URL, remote server fetches `GET /users/alice`
3. **Key Exchange**: Actor profile includes public key for HTTP signature verification
4. **Receive Activities**: Remote servers can POST activities to `/users/{username}/inbox`
5. **Create Activities**: Local users can POST activities to `/users/{username}/outbox`
6. **Awaiting Processing**: Activities are stored and ready for the Activity Processor

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
- **Client Authentication**: OAuth 2.0 with PKCE (currently no auth)
- **Activity Delivery**: DynamoDB Streams → Lambda

## Your Task

Continue development by implementing the **Activity Processor** (`cmd/activity-processor`), which will process activities from DynamoDB Streams and complete the federation loop.

### 1. Create Activity Processor Handler
Create `cmd/activity-processor/main.go` that:
- Processes DynamoDB Stream events
- Identifies inbox vs outbox activities
- Routes to appropriate processors
- Handles errors gracefully
- Logs processing results

### 2. Process Inbox Activities
For activities in user inboxes:
- **Follow**: Create pending follow relationship
- **Accept**: Mark follow as accepted, add to followers/following
- **Reject**: Mark follow as rejected, clean up
- **Create**: Store the object (Note, Article, etc.)
- **Like**: Store the like relationship
- **Announce**: Store the boost
- **Undo**: Reverse previous activities

### 3. Process Outbox Activities
For activities in user outboxes:
- Extract all recipients (to, cc, bto, bcc)
- Deduplicate and resolve collections
- Fetch recipient actor profiles to get inbox URLs
- Sign requests with HTTP signatures
- Deliver activities to remote inboxes
- Handle delivery failures with retries

### 4. Activity Delivery Implementation
```go
func deliverActivity(ctx context.Context, activity *activitypub.Activity, recipientInbox string) error {
    // 1. Get sender's private key
    actor, _ := store.GetActor(ctx, extractUsername(activity.Actor))
    privateKey, _ := store.GetActorPrivateKey(ctx, actor.PreferredUsername)
    
    // 2. Marshal activity to JSON
    body, _ := json.Marshal(activity)
    
    // 3. Create HTTP request
    req, _ := http.NewRequestWithContext(ctx, "POST", recipientInbox, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/activity+json")
    
    // 4. Sign the request
    keyID := fmt.Sprintf("%s#main-key", activity.Actor)
    federation.SignHTTPRequest(req, privateKey, keyID)
    
    // 5. Send the request
    resp, err := httpClient.Do(req)
    // Handle response...
}
```

### 5. Implement Storage Operations
Since the processor needs to store relationships and objects:
- Implement `CreateRelationship` for follows
- Implement `UpdateRelationship` for accept/reject
- Implement `CreateObject` for Notes, Articles
- Add these to the storage interface

### 6. Write Comprehensive Tests
- Unit tests for each activity type processor
- Mock DynamoDB stream events
- Mock HTTP client for delivery tests
- Test retry logic with failures
- Test recipient resolution

## Example Implementation Pattern

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
    
    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
    "github.com/aron23/lesser/pkg/activitypub"
    "github.com/aron23/lesser/pkg/common"
    "github.com/aron23/lesser/pkg/federation"
    "github.com/aron23/lesser/pkg/storage"
    "github.com/aron23/lesser/pkg/storage/dynamodb"
    "go.uber.org/zap"
)

var (
    store      storage.Storage
    logger     *zap.Logger
    httpClient *http.Client
)

func init() {
    logger = common.Logger()
    
    var err error
    store, err = dynamodb.New()
    if err != nil {
        logger.Fatal("failed to initialize storage", zap.Error(err))
    }
    
    // HTTP client with timeout for delivery
    httpClient = &http.Client{
        Timeout: 30 * time.Second,
    }
}

func handler(ctx context.Context, event events.DynamoDBEvent) error {
    log := common.WithContext(ctx)
    
    for _, record := range event.Records {
        if record.EventName != "INSERT" && record.EventName != "MODIFY" {
            continue
        }
        
        // Parse the DynamoDB record
        activity, direction, username, err := parseActivityRecord(record.Change.NewImage)
        if err != nil {
            log.Error("failed to parse record", zap.Error(err))
            continue
        }
        
        log.Info("processing activity",
            zap.String("id", activity.ID),
            zap.String("type", activity.Type),
            zap.String("direction", string(direction)),
            zap.String("username", username))
        
        if direction == storage.ActivityDirectionInbox {
            // Process inbox activity
            if err := processInboxActivity(ctx, activity, username); err != nil {
                log.Error("failed to process inbox activity",
                    zap.String("activity_id", activity.ID),
                    zap.Error(err))
            }
        } else {
            // Process outbox activity - deliver to recipients
            if err := processOutboxActivity(ctx, activity); err != nil {
                log.Error("failed to process outbox activity",
                    zap.String("activity_id", activity.ID),
                    zap.Error(err))
            }
        }
    }
    
    return nil
}

func parseActivityRecord(image map[string]types.AttributeValue) (*activitypub.Activity, storage.ActivityDirection, string, error) {
    // Extract activity from DynamoDB record
    // Parse PK to get username
    // Parse GSI1PK to determine if inbox or outbox
    // Return activity, direction, and username
}

func processInboxActivity(ctx context.Context, activity *activitypub.Activity, recipientUsername string) error {
    log := common.WithContext(ctx)
    
    switch activity.Type {
    case activitypub.FollowType:
        // Create pending follow relationship
        return processFollow(ctx, activity, recipientUsername)
        
    case activitypub.AcceptType:
        // Check if this is accepting a follow
        if innerActivity, ok := activity.Object.(map[string]interface{}); ok {
            if innerActivity["type"] == activitypub.FollowType {
                return processFollowAccept(ctx, activity, recipientUsername)
            }
        }
        
    case activitypub.CreateType:
        // Store the created object
        return processCreate(ctx, activity, recipientUsername)
        
    case activitypub.LikeType:
        // Store the like
        return processLike(ctx, activity, recipientUsername)
        
    default:
        log.Warn("unhandled inbox activity type",
            zap.String("type", activity.Type),
            zap.String("id", activity.ID))
    }
    
    return nil
}

func processOutboxActivity(ctx context.Context, activity *activitypub.Activity) error {
    // Extract all recipients
    recipients := extractAllRecipients(activity)
    
    // Deliver to each recipient
    var deliveryErrors []error
    for _, recipient := range recipients {
        if err := deliverToRecipient(ctx, activity, recipient); err != nil {
            deliveryErrors = append(deliveryErrors, err)
        }
    }
    
    if len(deliveryErrors) > 0 {
        return fmt.Errorf("delivery failed to %d recipients", len(deliveryErrors))
    }
    
    return nil
}

func main() {
    lambda.Start(handler)
}
```

## Testing Strategy

Create `cmd/activity-processor/handler_test.go` with:
- Mock DynamoDB stream events for different activity types
- Test inbox processing for each activity type
- Test outbox delivery with mock HTTP client
- Test error handling and retries
- Test recipient resolution and deduplication

## Success Criteria

- [ ] DynamoDB stream events are properly parsed
- [ ] Inbox activities update local state correctly
- [ ] Follow relationships are created/updated
- [ ] Outbox activities are delivered to recipients
- [ ] HTTP signatures work for delivery
- [ ] Failed deliveries are retried appropriately
- [ ] Good test coverage (>80%)

## Next Steps After This Task

1. **GET Outbox Handler**: Serve activities from outbox with pagination
2. **Collections**: Implement followers/following endpoints
3. **OAuth 2.0**: Authentication for local users
4. **Object Storage**: Implement storage for Notes, Articles
5. **Pulumi Infrastructure**: Deploy everything to AWS

The Activity Processor is the most critical component because it:
- Completes the federation loop by delivering activities
- Processes incoming activities to update local state
- Enables follows, likes, and other social features
- Makes Lesser a fully functional ActivityPub server

Begin by creating the handler structure and parsing DynamoDB stream events. Focus on getting basic Follow/Accept working first, then expand to other activity types. 