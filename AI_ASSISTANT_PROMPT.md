# AI Assistant Prompt for Lesser Development

## 🎯 CURRENT PRIORITY: Implement Conversation Threading
**Next Step**: Add conversation tracking to enable proper reply threads in Mastodon clients.

You are helping develop Lesser, a serverless ActivityPub implementation that aims to be compatible with Mastodon clients. The project is written in Go and deployed on AWS using Lambda, DynamoDB, and API Gateway.

## Project Overview

Lesser uses a modular architecture with:
- **Converter Pattern**: `pkg/mastodon/converter.go` - Centralizes ActivityPub to Mastodon API conversions
- **Service Layer**: `pkg/mastodon/actor_service.go` - Higher-level business logic
- **Clean Handlers**: HTTP handlers use shared converter instance
- **DynamoDB Storage**: All data persisted in single-table design

## Project Structure
- `/cmd/api/` - API Lambda handlers
- `/pkg/storage/dynamodb/` - DynamoDB storage layer
- `/pkg/activitypub/` - ActivityPub types and logic
- `/pkg/mastodon/` - Mastodon API converters and services
- `/infra/` - Pulumi infrastructure code
- `test_api_automated.py` - API test script

## Current Status

✅ **Completed Features**:
- OAuth 2.0 authentication with scopes
- Account operations (follow/unfollow, block/unblock, update profile)
- Creating, editing, deleting statuses
- Favorites and reblogs
- Media upload to S3
- Bookmarks with pagination
- Basic timelines (home, public, hashtag)
- Search functionality
- Modular converter architecture
- Home timeline fan-out on write pattern
- **Hashtag extraction and timeline fan-out** ✨
- **Lists management** ✨ NEW!
  - Full CRUD operations for lists
  - Managing list memberships
  - List timelines with proper fan-out
  - Respects replies policies (none, followed, list)

❌ **Known Issues**:
- Conversation participant indexing incomplete
- Media CDN URLs need proper generation
- Timeline trimming for inactive users not implemented

🚧 **Recently Implemented (Ready for Testing)**:
- **Notifications System**: Complete implementation with all Mastodon API endpoints ✨ NEW!
  - `cmd/api/handlers/misc.go` - Full notification API handlers
  - `pkg/storage/dynamodb/notifications.go` - DynamoDB storage implementation
  - `cmd/activity-processor/main.go` - Notification creation on activities
  - Supports all core notification types: follow, mention, favourite, reblog
  - Filtering by type and pagination support
  - Auto-TTL after 30 days for storage optimization
  - Ready for testing with Mastodon clients!

- **Lists Feature**: Complete implementation with all Mastodon API endpoints
  - `cmd/api/handlers/lists.go` - Full API handlers
  - `pkg/storage/dynamodb/lists.go` - DynamoDB storage implementation
  - Timeline fan-out includes list timelines
  - Supports replies policies for filtering content
  - Ready for testing with Mastodon clients (especially Ivory)

- **Test Infrastructure**: Centralized mock storage
  - All tests now use `internal/testutil/mocks/MockStorage`
  - Prevents breaking tests when Storage interface changes
  - Much easier to maintain and extend

## 🎯 NEXT IMPLEMENTATION PRIORITIES

### ⭐ CURRENT FOCUS: Conversation Threading (HIGH PRIORITY) 🔴

**Why**: Critical for proper reply chains and discussion threads. Many clients rely on this for thread views.

**Implementation Plan**:
1. **Update DynamoDB schema** for conversation tracking:
   - Add conversation ID to status objects
   - Create conversation participant index
   - Track conversation originator

2. **Enhance status creation** in `cmd/api/handlers/statuses.go`:
   - Generate conversation IDs for new posts
   - Inherit conversation ID from parent when replying
   - Update participant tracking

3. **Implement conversation endpoints**:
   - `GET /api/v1/conversations` - List conversations
   - `DELETE /api/v1/conversations/:id` - Remove from conversations
   - `POST /api/v1/conversations/:id/read` - Mark as read

4. **Update status context** handler to use conversation data

**Key Files to Modify**:
- `pkg/storage/dynamodb/objects.go` - Add conversation tracking
- `cmd/api/handlers/conversations.go` - Enhance existing stub
- `cmd/api/handlers/statuses.go` - Update create/reply logic

### 2. Push Notifications (MEDIUM PRIORITY) 🟠
**Why**: Real-time updates improve user engagement

- Web Push protocol implementation
- Subscription management in DynamoDB
- Notification delivery via SQS/Lambda
- VAPID key generation and management

### 3. Account Search & Discovery (MEDIUM PRIORITY) 🟠
**Why**: Users need to find accounts to follow

- `GET /api/v1/accounts/search` - Search for accounts
- `GET /api/v1/accounts/familiar_followers` - Find mutual connections
- Implement proper search indexing

### ✅ Recently Completed:
1. **Account Relationships Batch** - Batch endpoint for checking multiple relationships at once!

## Implementation Guidelines

### 🎯 Start with Conversation Threading!
Conversation threading is the next critical feature because:
1. **Essential for thread views** - clients need proper reply chains
2. **Improves discussion UX** - users can follow conversations
3. **Foundation for future features** - muting, pinning conversations
4. **Many clients depend on it** - Mastodon web UI, Ivory, etc.

### Use Established Patterns

1. **Handlers**: Keep thin, delegate to storage layer
   ```go
   func (h *Handler) HandleGetNotifications(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
       // 1. Validate auth & extract pagination params
       // 2. Call storage.GetNotifications()
       // 3. Convert to Mastodon API format
       // 4. Return paginated response
   }
   ```

2. **Storage**: Follow single-table patterns
   ```go
   func (s *dynamoDBStorage) CreateNotification(ctx context.Context, notification *storage.Notification) error {
       // Use timestamp-based SK for chronological ordering
       // Set TTL for auto-cleanup after 30 days
       // Return notification object
   }
   ```

3. **Models**: Match Mastodon API exactly
   ```go
   type Notification struct {
       ID        string    `json:"id"`
       Type      string    `json:"type"`
       CreatedAt time.Time `json:"created_at"`
       Account   *Account  `json:"account"`
       Status    *Status   `json:"status,omitempty"`
   }
   ```

### Testing Workflow

```bash
# Build
make build-api

# Deploy
cd infra && pulumi up

# Test individual features
python test_api_automated.py

# Test with clients
# - Ivory (iOS) - Great list support
# - Elk.zone (Web) - Modern UI
# - Mastodon official app
```

## Recent Wins 🎉

1. **Account Relationships Batch API Complete**: Batch endpoint for checking multiple relationships at once!
2. **Notifications System Complete**: All core notification types implemented with proper fan-out!
3. **Lists Feature Complete**: All CRUD operations, memberships, and list timelines working!
4. **Timeline Fan-out Enhanced**: Now includes list timelines with proper replies policies
5. **Test Infrastructure Improved**: Centralized mocks make development much smoother
6. **Home Timeline & Hashtags**: Both working perfectly with proper fan-out

## Quick Start for Conversation Threading

```go
// 1. Update object creation in pkg/storage/dynamodb/objects.go
func (s *dynamoDBStorage) CreateObject(ctx context.Context, obj *activitypub.Object) error {
    // Generate conversation ID for new posts
    if obj.InReplyTo == "" {
        obj.ConversationID = generateConversationID()
    } else {
        // Inherit conversation ID from parent
        parent, _ := s.GetObject(ctx, obj.InReplyTo)
        obj.ConversationID = parent.ConversationID
    }
    // ... existing code ...
}

// 2. Add conversation tracking methods
func (s *dynamoDBStorage) GetConversations(ctx context.Context, username string) ([]*Conversation, error) {
    // Query conversations where user is a participant
    // Sort by last activity
}

// 3. Update handlers/conversations.go
func (h *Handler) HandleGetConversations(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
    // Get user's conversations with latest status in each
    // Include unread counts
    // Support pagination
}
```

## What's Next?

### 🚀 IMMEDIATE ACTION: Implement Conversation Threading

**Step 1: Start Here** 👇
```bash
# Begin by examining the current conversation stub
cat cmd/api/handlers/conversations.go

# Check how statuses are created
grep -n "CreateObject\|InReplyTo" pkg/storage/dynamodb/objects.go
```

**Step 2: Key Implementation Tasks**
1. ✅ Add `ConversationID` field to object storage
2. ✅ Generate conversation IDs for root posts
3. ✅ Inherit conversation ID when replying
4. ✅ Create participant tracking index
5. ✅ Implement conversation list endpoint
6. ✅ Add conversation read/unread tracking

**Step 3: Test Your Implementation**
- Create a post and verify it gets a conversation ID
- Reply to the post and verify ID inheritance
- Check `/api/v1/conversations` returns the thread
- Test conversation muting and deletion

**Already Completed**: 
- ✅ Account Relationships Batch - Ready for testing!
- ✅ Notifications System - All types working!
- ✅ Lists Management - Full CRUD + timelines!

## Additional Resources

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [Mastodon API Documentation](https://docs.joinmastodon.org/api/)
- [Project Design Document](DESIGN.md)
- [Developer Guidelines](DEVELOPER_GUIDELINES.md) 