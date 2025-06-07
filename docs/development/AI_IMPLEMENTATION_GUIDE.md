# AI Assistant Implementation Guide for Lesser

## Project Context
Lesser is a serverless ActivityPub implementation written in Go, deployed on AWS using Lambda, DynamoDB, and API Gateway. The goal is to achieve Mastodon API compatibility so that existing Mastodon clients (like Ivory, Elk.zone) can connect and function properly.

## Current State
- Basic OAuth flow works
- Users can authenticate with Mastodon clients
- Posts can be created but don't appear in timelines due to DynamoDB storage format issues
- Many Mastodon API endpoints are missing or incomplete

## Critical Implementation Priorities

### 🚨 Priority 1: Fix Core Functionality (Required for basic client usage)

1. **Fix DynamoDB Object Storage Format**
   - Location: `pkg/storage/dynamodb/objects.go`
   - Issue: Objects are stored in raw DynamoDB format instead of properly unmarshaled structs
   - Impact: Posts don't appear in timelines even though they're created
   - Fix: Already identified in code - use local `ObjectRecord` type instead of `storage.ObjectRecord`

2. **Implement Media Upload**
   - Endpoints: `POST /api/v1/media`, `GET /api/v1/media/:id`, `PUT /api/v1/media/:id`
   - Location: Create new handler in `cmd/api/handlers/media.go` or use existing `cmd/media/`
   - Critical because: Users can't post images/videos without this
   - Implementation notes:
     - Media uploads should go to S3 bucket
     - Return proper MediaAttachment response
     - Generate preview URLs and blurhash

3. **Complete Timeline Endpoints**
   - Missing: `GET /api/v1/timelines/tag/:hashtag`, `GET /api/v1/conversations`
   - Critical because: Clients expect these for basic navigation
   - Implementation in: `cmd/api/handlers/timelines.go`

4. **Implement Bookmarks**
   - Endpoints: `POST /api/v1/statuses/:id/bookmark`, `POST /api/v1/statuses/:id/unbookmark`, `GET /api/v1/bookmarks`
   - Critical because: Most clients show bookmark buttons prominently
   - Storage: Add bookmark records to DynamoDB

### 📌 Priority 2: Enhanced User Experience (Highly desired features)

5. **Lists Management**
   - Full CRUD for lists and list memberships
   - Timeline endpoint: `GET /api/v1/timelines/list/:list_id`
   - Storage design needed for list memberships

6. **Notifications Improvements**
   - Implement actual notification storage and retrieval
   - Add notification types: mention, follow, favourite, reblog
   - Implement clear/dismiss functionality

7. **Account Relationships**
   - `GET /api/v1/accounts/relationships` - Check relationships with multiple accounts
   - Critical for showing follow/block status in UI

8. **Follow Requests** (if implementing locked accounts)
   - `GET /api/v1/follow_requests`
   - `POST /api/v1/follow_requests/:id/authorize`
   - `POST /api/v1/follow_requests/:id/reject`

9. **Mutes Implementation**
   - Account muting: `POST /api/v1/accounts/:id/mute`, `POST /api/v1/accounts/:id/unmute`
   - Status muting: `POST /api/v1/statuses/:id/mute`, `POST /api/v1/statuses/:id/unmute`
   - List muted accounts: `GET /api/v1/mutes`

### 🎯 Priority 3: Advanced Features (Nice to have)

10. **Search v2**
    - Implement `GET /api/v2/search` with proper pagination
    - Add full-text search capabilities
    - Consider using DynamoDB streams to populate a search index

11. **Filters/Keyword Muting**
    - Full filter CRUD operations
    - Apply filters to timelines

12. **Polls**
    - Create polls in statuses
    - Vote on polls
    - Show poll results

13. **Scheduled Statuses**
    - Store scheduled posts
    - Implement scheduler (Lambda + CloudWatch Events)

14. **Custom Emojis**
    - Upload and manage custom emojis
    - Include in status rendering

## Implementation Guidelines

### Code Structure
```
cmd/api/handlers/
├── existing_handler.go  # Reference these for patterns
└── new_feature.go       # Create new handlers here

pkg/storage/dynamodb/
└── new_feature.go       # Add storage methods here
```

### DynamoDB Patterns
- Primary Key (PK) patterns:
  - `USER#<username>` for user data
  - `OBJECT#<object-id>` for posts/notes
  - `TIMELINE#<type>#<id>` for timelines
  - `BOOKMARK#<username>` for bookmarks (suggested)
  - `LIST#<list-id>` for lists (suggested)

### Response Patterns
- Always use `common.OK(data)` for successful responses (includes CORS headers)
- Use `common.BadRequest()`, `common.Unauthorized()`, etc. for errors
- Follow existing Mastodon API response formats exactly

### Testing
1. Use the included `test_api_automated.py` script to verify endpoints
2. Test with real Mastodon clients (Ivory, Elk.zone)
3. Check Lambda logs in CloudWatch for debugging

### Example Implementation Pattern
```go
// In cmd/api/main.go - Add route
if path == "/bookmarks" && method == http.MethodGet {
    return handler.HandleGetBookmarks(ctx, request)
}

// In cmd/api/handlers/bookmarks.go - Create handler
func (h *Handler) HandleGetBookmarks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
    // 1. Validate auth token
    // 2. Get bookmarks from storage
    // 3. Convert to Mastodon format
    // 4. Return with common.OK()
}

// In pkg/storage/dynamodb/bookmarks.go - Add storage
func (c *Client) GetBookmarks(ctx context.Context, username string, limit int) ([]Bookmark, error) {
    // Query DynamoDB
}
```

## Current Blockers
1. **Posts not appearing**: Fix the DynamoDB storage format issue first
2. **Media uploads**: Implement before working on polls or other media-dependent features
3. **Search limitations**: Current implementation is very basic

## Success Criteria
- Ivory app can: authenticate, post statuses with images, view timelines, bookmark posts
- Elk.zone can: complete sign-in flow, navigate between timelines, search for users
- Posts appear correctly in all timelines
- No errors in CloudWatch logs during normal usage

## Getting Started
1. Fix the DynamoDB storage issue (Priority 1, item 1)
2. Deploy: `cd infra && pulumi up`
3. Test with: `python test_api_automated.py`
4. Verify posts now appear in clients
5. Move on to media uploads (Priority 1, item 2)

Remember: Mastodon clients are very particular about response formats. When in doubt, check what Mastodon's API actually returns and match it exactly. 