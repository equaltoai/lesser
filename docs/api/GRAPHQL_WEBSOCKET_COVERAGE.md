# GraphQL and WebSocket Coverage Analysis

## Executive Summary

**No, not all Mastodon functionality is accessible via GraphQL and WebSocket.** While Lesser provides extensive GraphQL and WebSocket capabilities, they serve complementary roles rather than complete replacements for the REST API.

## GraphQL Coverage

### What's Available in GraphQL

#### Core Queries (Partial Mastodon Coverage)
- `actor(id, username)` - Get user profiles
- `object(id)` - Get posts/statuses
- `timeline(type, hashtag, listId)` - Various timeline types
- `search(query, type)` - Search functionality
- `notifications(types, excludeTypes)` - Notification streams
- `hashtag(name)` - Hashtag information
- `followedHashtags()` - User's followed hashtags

#### Core Mutations (Partial Mastodon Coverage)
- `createNote(input)` - Create posts
- `deleteObject(id)` - Delete posts
- `likeObject(id)` / `unlikeObject(id)` - Favorites
- `shareObject(id)` / `unshareObject(id)` - Boosts
- `followActor(id)` / `unfollowActor(id)` - Following
- `followHashtag(hashtag)` / `unfollowHashtag(hashtag)` - Hashtag following

#### Subscriptions (Real-time Updates)
- `activityStream(types)` - Activity feeds
- `timelineUpdates(type)` - Timeline changes
- `quoteActivity(noteId)` - Quote post updates

### What's MISSING from GraphQL

Critical Mastodon features NOT available in GraphQL:

1. **Conversations/Direct Messages** ❌
   - No queries for conversations
   - No mutations to send/read DMs
   - No conversation management

2. **Account Management** ❌
   - No account creation/update
   - No profile editing
   - No preferences management
   - No OAuth app management

3. **Media Operations** ❌
   - No media upload mutations
   - No media metadata updates
   - No thumbnail generation control

4. **Lists Management** ❌
   - No list creation/deletion
   - No list membership management
   - No list timeline queries (beyond basic)

5. **Moderation Features** ❌
   - No mute/block operations
   - No report creation
   - No filter management

6. **Admin Operations** ❌
   - No admin queries/mutations
   - No instance management
   - No user suspension/moderation

7. **Federation Controls** ❌
   - No domain blocks
   - No relay management
   - No instance information updates

8. **Import/Export** ❌
   - No data export initiation
   - No import operations
   - No backup management

## WebSocket Coverage

### What's Available via WebSocket

#### Stream Types Supported
```javascript
// Available WebSocket streams
"public"              // Public timeline
"public:local"        // Local public timeline  
"public:remote"       // Remote public timeline
"user"                // User's home timeline
"user:notification"   // User's notifications
"direct"              // Direct messages stream
"list"                // List timeline
"hashtag"             // Hashtag timeline
```

#### Event Types
- `update` - New posts/edits
- `delete` - Post deletions
- `notification` - New notifications
- `status.update` - Status changes
- `announcement` - Instance announcements
- `filters_changed` - Filter updates

### What's MISSING from WebSocket

1. **Account Events** ❌
   - Profile updates
   - Relationship changes
   - Settings changes

2. **Conversation Management** ❌
   - Conversation creation/deletion events
   - Read status updates
   - Typing indicators

3. **Admin Events** ❌
   - Moderation actions
   - User reports
   - Instance-wide announcements editing

4. **Media Processing Events** ❌
   - Upload progress
   - Processing completion
   - Transcoding status

## Architectural Reasons

### Why GraphQL Doesn't Cover Everything

1. **REST API Priority**: Lesser was built with Mastodon REST API compatibility as the primary goal
2. **Complexity**: Some operations (like media upload) don't map well to GraphQL patterns
3. **Authentication**: OAuth flows and app management require REST endpoints
4. **Federation**: ActivityPub requires specific REST endpoints for compatibility
5. **Binary Data**: Media uploads and downloads work better with REST

### Command + Event Architecture for WebSocket

WebSocket in Lesser is a first-class interface supporting nearly all operations through a command + event pattern:

1. **Synchronous Commands** (< 29s): Direct request/response for most operations
2. **Asynchronous Commands**: Long operations use DynamoDB Streams + Lambda (up to 15 min) with progress events
3. **Real-time Events**: All state changes published to relevant streams
4. **Binary Operations**: Only media uploads require REST due to WebSocket frame limitations

## Comparison Table (Current State → Target State)

| Feature Category | REST API | GraphQL | WebSocket |
|-----------------|----------|---------|-----------|
| **Posts/Statuses** | ✅ Full | ⚠️ Partial → ✅ Full | ⚠️ Updates only → ✅ Commands + Streaming |
| **Timelines** | ✅ Full | ⚠️ Basic → ✅ Full | ✅ Streaming |
| **Conversations** | ✅ Full | ❌ None → ✅ Full | ⚠️ Stream only → ✅ Commands + Streaming |
| **Accounts** | ✅ Full | ⚠️ Read only → ✅ Full | ❌ None → ✅ Commands + Streaming |
| **Media** | ✅ Full | ❌ None → ✅ Metadata ops | ❌ None → ⚠️ Commands (no upload) |
| **Lists** | ✅ Full | ⚠️ Basic → ✅ Full | ⚠️ Stream only → ✅ Commands + Streaming |
| **Notifications** | ✅ Full | ⚠️ Basic → ✅ Full | ✅ Streaming → ✅ Commands + Streaming |
| **Search** | ✅ Full | ⚠️ Basic → ✅ Full | ❌ None → ✅ Commands |
| **Admin** | ✅ Full | ❌ None → ✅ Full | ❌ None → ✅ Commands |
| **Federation** | ✅ Full | ❌ None → ⚠️ Read only | ❌ None → ⚠️ Events only |
| **OAuth** | ✅ Full | ❌ None (REST required) | ❌ None (REST required) |
| **Filters** | ✅ Full | ❌ None → ✅ Full | ⚠️ Updates only → ✅ Commands + Streaming |
| **Reports** | ✅ Full | ❌ None → ✅ Full | ❌ None → ✅ Commands + Events |
| **Import/Export** | ✅ Full | ❌ None → ✅ Job mgmt | ❌ None → ✅ Commands + Progress |

## Implementation Pattern Examples

### WebSocket Command Examples

```javascript
// Create a post
ws.send(JSON.stringify({
  type: "command",
  action: "status.create",
  data: {
    content: "Hello world!",
    visibility: "public",
    mediaIds: []
  }
}));
// Response: { type: "status.created", data: { id: "...", content: "...", ... } }

// Start an import (long-running)
ws.send(JSON.stringify({
  type: "command", 
  action: "import.start",
  data: {
    type: "mastodon",
    dataUrl: "s3://bucket/import.tar.gz"
  }
}));
// Response: { type: "import.started", data: { jobId: "...", status: "pending" } }
// Later: { type: "import.progress", data: { jobId: "...", progress: 0.5 } }
// Finally: { type: "import.completed", data: { jobId: "...", itemsImported: 1000 } }
```

### GraphQL Mutation Examples

```graphql
mutation CreateStatus($input: CreateStatusInput!) {
  createStatus(input: $input) {
    id
    content
    visibility
    createdAt
  }
}

mutation StartImport($input: StartImportInput!) {
  startImport(input: $input) {
    jobId
    status
    createdAt
  }
}
```

## Client Integration Recommendations

### For Mastodon Clients
- **Primary**: Use REST API for full functionality
- **Enhancement**: Add WebSocket for real-time updates
- **Optional**: Use GraphQL for complex queries if needed

### For Custom Clients
- **GraphQL**: Good for read-heavy operations and complex queries
- **WebSocket**: Essential for real-time features
- **REST**: Required for complete functionality

### Example Integration Pattern
```javascript
// Use REST for actions
POST /api/v1/statuses           // Create post
POST /api/v2/media              // Upload media
POST /api/v1/conversations/read // Mark DM read

// Use WebSocket for real-time
ws.subscribe('user')            // Home timeline updates
ws.subscribe('user:notification') // Real-time notifications

// Use GraphQL for complex reads
query {
  timeline(type: HOME) {
    edges {
      node {
        content
        author { username }
        quotedPost { content }
      }
    }
  }
}
```

## Conclusion

Lesser's GraphQL and WebSocket implementations are **supplementary** to the REST API, not replacements:

- **REST API**: 100% Mastodon compatibility ✅
- **GraphQL**: ~30% coverage, focused on read operations ⚠️
- **WebSocket**: ~40% coverage, focused on real-time updates ⚠️

To achieve full Mastodon functionality, clients MUST use the REST API. GraphQL and WebSocket provide enhanced capabilities but cannot replace the REST API for complete feature access.