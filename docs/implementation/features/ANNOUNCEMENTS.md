# Announcements Implementation for Lesser

## Overview

This document describes the implementation of the announcements feature in Lesser, providing Mastodon API compatibility for server-wide announcements. Announcements allow administrators to broadcast important messages to all users, with support for dismissals and emoji reactions.

## Features

### Core Functionality
- **Server-wide Announcements**: Administrators can create announcements visible to all users
- **Time-based Visibility**: Announcements can have start and end times
- **User Dismissals**: Each user can dismiss announcements independently
- **Emoji Reactions**: Users can react to announcements with emojis
- **All-Day Events**: Support for marking announcements as all-day events

### API Endpoints

#### User Endpoints
- `GET /api/v1/announcements` - List active announcements
- `POST /api/v1/announcements/:id/dismiss` - Dismiss an announcement
- `PUT /api/v1/announcements/:id/reactions/:name` - Add a reaction
- `DELETE /api/v1/announcements/:id/reactions/:name` - Remove a reaction

#### Admin Endpoints
- `POST /api/v1/admin/announcements` - Create a new announcement (admin only)

## Storage Design

### DynamoDB Schema

#### Announcements Table
```
PK: ANNOUNCEMENT#<id>
SK: ANNOUNCEMENT
Attributes:
- ID: string
- Content: string (HTML)
- Text: string (plain text)
- PublishedAt: timestamp
- UpdatedAt: timestamp
- AllDay: boolean
- StartsAt: timestamp (optional)
- EndsAt: timestamp (optional)
- Reactions: array of Reaction objects
- Tags: array of strings
- Emojis: array of CustomEmoji objects
- Mentions: array of Mention objects
- CreatedBy: string (admin username)
```

#### Dismissals
```
PK: USER#<username>
SK: ANNOUNCEMENT_DISMISSED#<announcement_id>
Attributes:
- Username: string
- AnnouncementID: string
- DismissedAt: timestamp
```

#### Reactions
```
PK: ANNOUNCEMENT_REACTION#<announcement_id>
SK: USER#<username>#<emoji_name>
Attributes:
- Username: string
- AnnouncementID: string
- EmojiName: string
- ReactedAt: timestamp
```

## Implementation Details

### 1. Storage Layer (`pkg/storage/dynamodb/announcements.go`)

The storage implementation provides:
- CRUD operations for announcements
- User-specific dismissal tracking
- Reaction management with deduplication
- Active announcement filtering based on dates

Key methods:
```go
CreateAnnouncement(ctx, announcement) error
GetAnnouncement(ctx, id) (*Announcement, error)
GetAnnouncements(ctx, active bool) ([]*Announcement, error)
UpdateAnnouncement(ctx, announcement) error
DeleteAnnouncement(ctx, id) error
DismissAnnouncement(ctx, username, announcementID) error
IsDismissed(ctx, username, announcementID) (bool, error)
GetDismissedAnnouncements(ctx, username) ([]string, error)
AddAnnouncementReaction(ctx, username, announcementID, emojiName) error
RemoveAnnouncementReaction(ctx, username, announcementID, emojiName) error
GetAnnouncementReactions(ctx, announcementID) (map[string][]string, error)
```

### 2. API Handlers (`cmd/api/handlers/announcements.go`)

The handlers implement the Mastodon API specification:

#### GetAnnouncements
- Supports both authenticated and unauthenticated access
- Filters out dismissed announcements for authenticated users
- Merges reaction data with announcement data
- Provides default emoji reactions if none specified

#### DismissAnnouncement
- Requires authentication
- Marks announcement as dismissed for the user
- Returns empty object on success

#### Add/RemoveAnnouncementReaction
- Requires authentication
- Validates reaction is allowed
- Updates reaction counts in real-time
- Handles idempotent operations

#### CreateAnnouncement (Admin)
- Requires admin role
- Validates required fields
- Supports scheduled announcements
- Returns created announcement

### 3. Data Models (`cmd/api/models/mastodon.go`)

Added Mastodon-compatible models:
```go
type Announcement struct {
    ID          string
    Content     string
    Text        string
    PublishedAt string
    UpdatedAt   string
    AllDay      bool
    StartsAt    *string
    EndsAt      *string
    Read        bool
    Reactions   []AnnouncementReaction
    // ... other fields
}

type AnnouncementReaction struct {
    Name      string
    Count     int
    Me        bool
    URL       string
    StaticURL string
}
```

## Usage Examples

### Creating an Announcement (Admin)

```bash
curl -X POST http://localhost:8080/api/v1/admin/announcements \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "<p>Welcome to Lesser!</p>",
    "text": "Welcome to Lesser!",
    "all_day": true
  }'
```

### Getting Announcements

```bash
# Unauthenticated
curl http://localhost:8080/api/v1/announcements

# Authenticated (excludes dismissed)
curl http://localhost:8080/api/v1/announcements \
  -H "Authorization: Bearer <token>"
```

### Dismissing an Announcement

```bash
curl -X POST http://localhost:8080/api/v1/announcements/123/dismiss \
  -H "Authorization: Bearer <token>"
```

### Adding a Reaction

```bash
curl -X PUT http://localhost:8080/api/v1/announcements/123/reactions/👍 \
  -H "Authorization: Bearer <token>"
```

## Default Reactions

When no reactions are specified for an announcement, the following defaults are available:
- 👍 (thumbs up)
- 👎 (thumbs down)
- 😄 (smile)
- 🎉 (party)
- 😕 (confused)
- ❤️ (heart)
- 🚀 (rocket)
- 👀 (eyes)

## Testing

A comprehensive test suite is available in `test_announcements.py` covering:
- Unauthenticated access
- Authenticated user flows
- Admin announcement creation
- Dismissal functionality
- Reaction management
- Error cases

## Security Considerations

1. **Authentication**: Most endpoints require valid OAuth tokens
2. **Authorization**: Only admins can create announcements
3. **Rate Limiting**: Consider implementing rate limits for reactions
4. **Content Validation**: HTML content should be sanitized
5. **Reaction Validation**: Only allowed reactions should be accepted

## Performance Considerations

1. **Scan Operation**: `GetAnnouncements` uses a scan which may be inefficient at scale
   - Consider adding a GSI for efficient announcement queries
2. **Caching**: Active announcements could be cached
3. **Pagination**: Not currently implemented but may be needed for many announcements

## Conclusion

The announcements feature provides a complete implementation of the Mastodon announcements API, enabling administrators to communicate important information to users with rich interaction capabilities. The implementation follows Lesser's serverless architecture patterns and integrates seamlessly with the existing authentication and storage systems. 