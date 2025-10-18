# API Reference

Lesser provides three API interfaces: REST (Mastodon-compatible), GraphQL, and WebSocket streaming.

## REST API (Mastodon v1 Compatible)

Base URL: `https://yourdomain.com/api/v1`

### Authentication

Most endpoints require authentication via Bearer token:

```bash
curl -H "Authorization: Bearer YOUR_TOKEN" \
  https://yourdomain.com/api/v1/accounts/verify_credentials
```

### Core Endpoints

#### Accounts
- `GET /api/v1/accounts/:id` - Get account
- `GET /api/v1/accounts/verify_credentials` - Verify credentials
- `PATCH /api/v1/accounts/update_credentials` - Update profile
- `GET /api/v1/accounts/:id/statuses` - Get account statuses
- `GET /api/v1/accounts/:id/followers` - Get followers
- `GET /api/v1/accounts/:id/following` - Get following

#### Statuses
- `POST /api/v1/statuses` - Create status
- `GET /api/v1/statuses/:id` - Get status
- `DELETE /api/v1/statuses/:id` - Delete status
- `POST /api/v1/statuses/:id/reblog` - Boost status
- `POST /api/v1/statuses/:id/unreblog` - Unboost status
- `POST /api/v1/statuses/:id/favourite` - Favorite status
- `POST /api/v1/statuses/:id/unfavourite` - Unfavorite status

#### Timelines
- `GET /api/v1/timelines/home` - Home timeline
- `GET /api/v1/timelines/public` - Public timeline
- `GET /api/v1/timelines/tag/:hashtag` - Hashtag timeline
- `GET /api/v1/timelines/list/:list_id` - List timeline

#### Relationships
- `POST /api/v1/accounts/:id/follow` - Follow account
- `POST /api/v1/accounts/:id/unfollow` - Unfollow account
- `POST /api/v1/accounts/:id/block` - Block account
- `POST /api/v1/accounts/:id/unblock` - Unblock account
- `POST /api/v1/accounts/:id/mute` - Mute account
- `POST /api/v1/accounts/:id/unmute` - Unmute account

#### Lists
- `GET /api/v1/lists` - Get lists
- `POST /api/v1/lists` - Create list
- `GET /api/v1/lists/:id` - Get list
- `PUT /api/v1/lists/:id` - Update list
- `DELETE /api/v1/lists/:id` - Delete list
- `GET /api/v1/lists/:id/accounts` - Get list members
- `POST /api/v1/lists/:id/accounts` - Add to list
- `DELETE /api/v1/lists/:id/accounts` - Remove from list

#### Notifications
- `GET /api/v1/notifications` - Get notifications
- `GET /api/v1/notifications/:id` - Get notification
- `POST /api/v1/notifications/clear` - Clear notifications
- `POST /api/v1/notifications/:id/dismiss` - Dismiss notification

#### Search
- `GET /api/v2/search` - Search accounts, statuses, hashtags

#### Media
- `POST /api/v1/media` - Upload media attachment
- `PUT /api/v1/media/:id` - Update media metadata

### Request/Response Format

#### Create Status Example

Request:
```json
POST /api/v1/statuses
{
  "status": "Hello Fediverse!",
  "visibility": "public",
  "sensitive": false,
  "spoiler_text": "",
  "media_ids": [],
  "poll": {
    "options": ["Yes", "No"],
    "expires_in": 86400,
    "multiple": false
  }
}
```

Response:
```json
{
  "id": "123456789",
  "created_at": "2024-01-01T12:00:00Z",
  "content": "<p>Hello Fediverse!</p>",
  "visibility": "public",
  "sensitive": false,
  "spoiler_text": "",
  "account": {
    "id": "1",
    "username": "alice",
    "acct": "alice@yourdomain.com",
    "display_name": "Alice",
    "avatar": "https://cdn.yourdomain.com/avatars/alice.jpg"
  },
  "media_attachments": [],
  "mentions": [],
  "tags": [],
  "reblogs_count": 0,
  "favourites_count": 0,
  "replies_count": 0
}
```

## GraphQL API

Endpoint: `https://yourdomain.com/graphql`

### Schema Overview

```graphql
type Query {
  # Account queries
  account(id: ID!): Account
  accounts(limit: Int, offset: Int): [Account!]!
  currentUser: Account
  
  # Status queries
  status(id: ID!): Status
  timeline(type: TimelineType!, limit: Int): [Status!]!
  
  # Search
  search(query: String!, type: SearchType): SearchResult!
  
  # Federation
  instance: Instance!
  instances: [Instance!]!
}

type Mutation {
  # Account mutations
  updateProfile(input: UpdateProfileInput!): Account!
  
  # Status mutations
  createStatus(input: CreateStatusInput!): Status!
  deleteStatus(id: ID!): Boolean!
  reblogStatus(id: ID!): Status!
  favoriteStatus(id: ID!): Status!
  
  # Relationship mutations
  followAccount(id: ID!): Relationship!
  unfollowAccount(id: ID!): Relationship!
  blockAccount(id: ID!): Relationship!
  muteAccount(id: ID!): Relationship!
  
  # List mutations
  createList(input: CreateListInput!): List!
  updateList(id: ID!, input: UpdateListInput!): List!
  deleteList(id: ID!): Boolean!
  addToList(listId: ID!, accountId: ID!): Boolean!
  removeFromList(listId: ID!, accountId: ID!): Boolean!
}

type Subscription {
  # Real-time updates
  timelineUpdates(type: TimelineType!): Status!
  notificationReceived: Notification!
  statusUpdated(id: ID!): Status!
}
```

### Example Queries

#### Get Timeline
```graphql
query GetHomeTimeline {
  timeline(type: HOME, limit: 20) {
    id
    content
    createdAt
    account {
      username
      displayName
      avatar
    }
    mediaAttachments {
      url
      type
    }
    reblogsCount
    favoritesCount
  }
}
```

#### Create Status
```graphql
mutation CreatePost {
  createStatus(input: {
    content: "Hello from GraphQL!"
    visibility: PUBLIC
    sensitive: false
  }) {
    id
    content
    createdAt
    url
  }
}
```

#### Subscribe to Timeline
```graphql
subscription TimelineUpdates {
  timelineUpdates(type: HOME) {
    id
    content
    account {
      username
    }
  }
}
```

## WebSocket Streaming API

Endpoint: `wss://yourdomain.com/streaming`

### Connection

```javascript
const ws = new WebSocket('wss://yourdomain.com/streaming?access_token=YOUR_TOKEN');

ws.onopen = () => {
  // Subscribe to streams
  ws.send(JSON.stringify({
    type: 'subscribe',
    stream: 'user'
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};
```

### Available Streams

- `user` - User's home timeline and notifications
- `public` - Public timeline
- `public:local` - Local public timeline
- `hashtag` - Specific hashtag (requires `tag` parameter)
- `list` - List timeline (requires `list` parameter)
- `direct` - Direct messages

### Event Types

```javascript
// Status update
{
  "event": "update",
  "payload": { /* Status object */ }
}

// Status deleted
{
  "event": "delete",
  "payload": "123456789"
}

// Notification
{
  "event": "notification",
  "payload": { /* Notification object */ }
}

// Filters changed
{
  "event": "filters_changed",
  "payload": null
}
```

## Federation Endpoints

### WebFinger
```
GET /.well-known/webfinger?resource=acct:alice@yourdomain.com
```

### NodeInfo
```
GET /.well-known/nodeinfo
GET /nodeinfo/2.0
```

### Actor
```
GET /users/alice
Accept: application/activity+json
```

### Inbox/Outbox
```
POST /users/alice/inbox
GET /users/alice/outbox
```

## OAuth 2.0

### Authorization Flow

1. Register application:
```bash
POST /api/v1/apps
{
  "client_name": "My App",
  "redirect_uris": "https://myapp.com/callback",
  "scopes": "read write follow push",
  "website": "https://myapp.com"
}
```

2. Authorize:
```
GET /oauth/authorize?client_id=CLIENT_ID&redirect_uri=REDIRECT_URI&response_type=code&scope=read+write
```

3. Get token:
```bash
POST /oauth/token
{
  "grant_type": "authorization_code",
  "code": "AUTH_CODE",
  "client_id": "CLIENT_ID",
  "client_secret": "CLIENT_SECRET",
  "redirect_uri": "REDIRECT_URI"
}
```

## Rate Limiting

Default limits per endpoint:

| Endpoint | Limit | Window |
|----------|-------|--------|
| POST /api/v1/statuses | 30 | 1 hour |
| POST /api/v1/media | 10 | 1 hour |
| GET /api/v1/timelines/* | 300 | 5 minutes |
| GET /api/v2/search | 60 | 1 minute |
| Default | 300 | 5 minutes |

Rate limit headers:
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 299
X-RateLimit-Reset: 1704124800
```

## Error Responses

Standard error format:
```json
{
  "error": "Record not found",
  "error_description": "The requested status does not exist",
  "error_code": "NOT_FOUND"
}
```

Common error codes:
- `400` - Bad Request
- `401` - Unauthorized
- `403` - Forbidden
- `404` - Not Found
- `422` - Unprocessable Entity
- `429` - Too Many Requests
- `500` - Internal Server Error
- `503` - Service Unavailable
