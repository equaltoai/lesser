# API Quick Reference

Lesser implements the full Mastodon API v1, making it compatible with all Mastodon clients. This guide covers the most commonly used endpoints.

## 🔐 Authentication

Lesser supports multiple authentication methods:

### OAuth2 Flow (Recommended)
```bash
# 1. Register your application
POST /api/v1/apps
{
  "client_name": "My App",
  "redirect_uris": "myapp://oauth",
  "scopes": "read write follow push"
}

# Response
{
  "client_id": "your-client-id",
  "client_secret": "your-client-secret"
}

# 2. Get user authorization
GET /oauth/authorize?client_id=...&redirect_uri=...&scope=...&response_type=code

# 3. Exchange code for token
POST /oauth/token
{
  "grant_type": "authorization_code",
  "code": "auth-code",
  "client_id": "your-client-id",
  "client_secret": "your-client-secret"
}
```

### Bearer Token Usage
```bash
# Include in all authenticated requests
Authorization: Bearer your-access-token
```

## 📝 Common Endpoints

### Account Management

#### Get Current User
```bash
GET /api/v1/accounts/verify_credentials

# Response
{
  "id": "123",
  "username": "alice",
  "acct": "alice",
  "display_name": "Alice",
  "followers_count": 100,
  "following_count": 50,
  "statuses_count": 1234,
  "avatar": "https://cdn.example.com/avatar.jpg"
}
```

#### Update Profile
```bash
PATCH /api/v1/accounts/update_credentials
{
  "display_name": "Alice Smith",
  "note": "Software developer | Coffee enthusiast",
  "avatar": "@/path/to/avatar.jpg",
  "header": "@/path/to/header.jpg"
}
```

#### Follow/Unfollow
```bash
# Follow
POST /api/v1/accounts/:id/follow

# Unfollow
POST /api/v1/accounts/:id/unfollow

# Response
{
  "id": "456",
  "following": true,
  "followed_by": false,
  "requested": false
}
```

### Timeline Operations

#### Home Timeline
```bash
GET /api/v1/timelines/home?limit=20

# Response
[
  {
    "id": "109876543210",
    "created_at": "2024-01-15T10:30:00Z",
    "account": { ... },
    "content": "<p>Hello world!</p>",
    "media_attachments": [],
    "favourites_count": 5,
    "reblogs_count": 2,
    "replies_count": 1
  }
]
```

#### Public Timeline
```bash
# Local only
GET /api/v1/timelines/public?local=true&limit=20

# Federated
GET /api/v1/timelines/public?limit=20
```

### Status (Post) Operations

#### Create Status
```bash
POST /api/v1/statuses
{
  "status": "Hello Fediverse! 🌟",
  "visibility": "public",
  "media_ids": ["123456"],
  "poll": {
    "options": ["Yes", "No"],
    "expires_in": 86400
  }
}

# Response
{
  "id": "109876543211",
  "uri": "https://example.com/users/alice/statuses/109876543211",
  "created_at": "2024-01-15T10:35:00Z",
  "content": "<p>Hello Fediverse! 🌟</p>",
  "visibility": "public",
  "account": { ... }
}
```

#### Delete Status
```bash
DELETE /api/v1/statuses/:id
```

#### Favorite/Unfavorite
```bash
# Favorite
POST /api/v1/statuses/:id/favourite

# Unfavorite
POST /api/v1/statuses/:id/unfavourite
```

#### Boost (Reblog)
```bash
# Boost
POST /api/v1/statuses/:id/reblog

# Unboost
POST /api/v1/statuses/:id/unreblog
```

### Media Upload

#### Upload Media
```bash
POST /api/v2/media
Content-Type: multipart/form-data

file=@photo.jpg
description="A beautiful sunset"

# Response
{
  "id": "123456",
  "type": "image",
  "url": "https://cdn.example.com/media/123456.jpg",
  "preview_url": "https://cdn.example.com/media/123456_preview.jpg",
  "meta": {
    "original": {
      "width": 1920,
      "height": 1080,
      "size": "1920x1080"
    }
  }
}
```

### Search

#### Search Everything
```bash
GET /api/v2/search?q=lesser&type=accounts,hashtags,statuses

# Response
{
  "accounts": [...],
  "statuses": [...],
  "hashtags": [
    {
      "name": "lesser",
      "url": "https://example.com/tags/lesser",
      "history": [...]
    }
  ]
}
```

### Notifications

#### Get Notifications
```bash
GET /api/v1/notifications?types[]=mention&types[]=follow

# Response
[
  {
    "id": "789",
    "type": "mention",
    "created_at": "2024-01-15T10:40:00Z",
    "account": { ... },
    "status": { ... }
  }
]
```

#### Clear Notifications
```bash
POST /api/v1/notifications/clear
```

### Streaming API

#### WebSocket Connection
```javascript
// Connect to streaming endpoint
const ws = new WebSocket('wss://example.com/api/v1/streaming?stream=user&access_token=...');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  if (data.event === 'update') {
    // New status in timeline
    const status = JSON.parse(data.payload);
  }
};
```

#### Available Streams
- `user` - User's home timeline + notifications
- `public` - Public timeline
- `public:local` - Local public timeline
- `hashtag` - Specific hashtag
- `list` - List timeline
- `direct` - Direct messages

## 🚀 Advanced Features

### Lists

#### Create List
```bash
POST /api/v1/lists
{
  "title": "Tech News"
}
```

#### Add/Remove Accounts
```bash
# Add accounts
POST /api/v1/lists/:id/accounts
{
  "account_ids": ["123", "456"]
}

# Remove accounts
DELETE /api/v1/lists/:id/accounts
{
  "account_ids": ["123"]
}
```

### Filters

#### Create Filter
```bash
POST /api/v2/filters
{
  "title": "Politics",
  "context": ["home", "public"],
  "filter_action": "warn",
  "keywords": [
    {
      "keyword": "election",
      "whole_word": true
    }
  ]
}
```

### Instance Information

#### Get Instance Info
```bash
GET /api/v2/instance

# Response
{
  "domain": "example.com",
  "title": "Example Lesser Instance",
  "version": "4.0.0+lesser",
  "source_url": "https://github.com/yourusername/lesser",
  "description": "A modern ActivityPub server",
  "usage": {
    "users": {
      "active_month": 150
    }
  },
  "configuration": {
    "statuses": {
      "max_characters": 500,
      "max_media_attachments": 4
    },
    "media_attachments": {
      "supported_mime_types": [
        "image/jpeg",
        "image/png",
        "image/gif",
        "video/mp4"
      ],
      "image_size_limit": 10485760,
      "video_size_limit": 41943040
    }
  }
}
```

## 📊 Rate Limits

Default rate limits per authenticated user:

| Endpoint | Limit |
|----------|-------|
| POST /api/v1/statuses | 300/hour |
| POST /api/v1/media | 30/hour |
| GET /api/v1/timelines/* | 300/5min |
| GET /api/v2/search | 30/5min |

Headers included in responses:
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 299
X-RateLimit-Reset: 1705318800
```

## 🔥 Error Handling

### Error Response Format
```json
{
  "error": "Record not found",
  "error_description": "The requested status does not exist"
}
```

### Common Status Codes
- `200` - Success
- `201` - Created
- `204` - No Content (success, no response body)
- `400` - Bad Request
- `401` - Unauthorized
- `403` - Forbidden
- `404` - Not Found
- `422` - Unprocessable Entity
- `429` - Too Many Requests
- `500` - Internal Server Error

## 🧪 Testing Your Integration

### cURL Examples
```bash
# Get home timeline
curl -H "Authorization: Bearer $TOKEN" \
  https://example.com/api/v1/timelines/home

# Post a status
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status":"Hello from cURL!"}' \
  https://example.com/api/v1/statuses

# Upload media
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@image.jpg" \
  -F "description=Test image" \
  https://example.com/api/v2/media
```

### Postman Collection
Import our [Postman collection](./lesser_api_postman_collection.json) for interactive testing.

## 🔗 Useful Links

- **[Full API Reference](./API_REFERENCE.md)** - Complete endpoint documentation
- **[GraphQL API](./GRAPHQL_API.md)** - Alternative GraphQL interface
- **[WebSocket Streaming](./STREAMING_API.md)** - Real-time updates

## 💡 Tips

1. **Pagination**: Use `max_id` and `since_id` for efficient pagination
2. **Idempotency**: Include `Idempotency-Key` header for POST requests
3. **Media**: Upload media first, then reference in status
4. **Visibility**: Options are `public`, `unlisted`, `private`, `direct`
5. **Caching**: Respect `Cache-Control` headers for better performance

---

<div align="center">

[Back to Docs](../README.md) • [Full API Reference](./API_REFERENCE.md)

</div> 