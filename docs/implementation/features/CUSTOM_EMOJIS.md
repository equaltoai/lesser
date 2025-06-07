# Custom Emojis Implementation for Lesser

## Overview

This document describes the implementation of custom emojis in Lesser, providing Mastodon API compatibility for server-specific emojis. Custom emojis allow instances to have their own unique set of emojis that can be used in posts, profiles, and other content.

## Features

### Core Functionality
- **Custom Emoji Management**: Administrators can create and manage custom emojis
- **Category Support**: Emojis can be organized into categories
- **Visibility Control**: Control which emojis appear in the picker
- **Remote Emoji Support**: Foundation for federated custom emojis
- **Image Metadata**: Track image properties for optimization

### API Endpoints

#### User Endpoints
- `GET /api/v1/custom_emojis` - List all visible custom emojis

#### Admin Endpoints (Future)
- `POST /api/v1/admin/custom_emojis` - Create a new custom emoji
- `PUT /api/v1/admin/custom_emojis/:shortcode` - Update custom emoji
- `DELETE /api/v1/admin/custom_emojis/:shortcode` - Delete custom emoji

## Storage Design

### DynamoDB Schema

#### Custom Emojis Table
```
PK: EMOJI#<shortcode>
SK: EMOJI
Attributes:
- Shortcode: string (unique identifier, e.g., "partyparrot")
- URL: string (URL to the emoji image)
- StaticURL: string (URL to static version)
- VisibleInPicker: boolean
- Category: string (optional)
- CreatedAt: timestamp
- UpdatedAt: timestamp
- Disabled: boolean
- Domain: string (empty for local, set for remote)
- ImageRemoteURL: string (original URL if remote)
- ImageStorageVersion: int
- ImageFileSize: int64
- ImageContentType: string
- ImageWidth: int
- ImageHeight: int
- ImageUpdatedAt: timestamp
```

## Implementation Details

### 1. Storage Layer (`pkg/storage/dynamodb/custom_emojis.go`)

The storage implementation provides:
- CRUD operations for custom emojis
- Category-based filtering
- Support for disabled/enabled states
- Remote emoji tracking

Key methods:
```go
CreateCustomEmoji(ctx, emoji) error
GetCustomEmoji(ctx, shortcode) (*CustomEmoji, error)
GetCustomEmojis(ctx) ([]*CustomEmoji, error)
UpdateCustomEmoji(ctx, emoji) error
DeleteCustomEmoji(ctx, shortcode) error
GetCustomEmojisByCategory(ctx, category) ([]*CustomEmoji, error)
```

### 2. API Handler (`cmd/api/handlers/custom_emojis.go`)

The handler implements the Mastodon API specification:

#### GetCustomEmojis
- Public endpoint (no authentication required)
- Returns all visible custom emojis
- Filters out disabled local emojis
- Simple array format for Mastodon compatibility

#### Admin Endpoints (Implemented)
- CreateCustomEmoji - Add new custom emoji
- UpdateCustomEmoji - Modify emoji properties
- DeleteCustomEmoji - Remove custom emoji
- All require admin authentication

### 3. Data Models (`cmd/api/models/mastodon.go`)

Added Mastodon-compatible models:
```go
type CustomEmoji struct {
    Shortcode       string
    URL             string
    StaticURL       string
    VisibleInPicker bool
    Category        string
}
```

## Usage in Content

Custom emojis are used in content by referencing their shortcode wrapped in colons:

```
This is a message with :partyparrot: emoji!
```

When rendering content, the system should:
1. Parse for `:shortcode:` patterns
2. Look up the emoji in the database
3. Replace with appropriate HTML/image tag
4. Include emoji data in API responses

## Integration Points

### 1. Status Rendering
When returning statuses via the API, custom emojis used in the content should be included in the `emojis` array field:

```json
{
  "content": "Hello :wave:",
  "emojis": [
    {
      "shortcode": "wave",
      "url": "https://example.com/emojis/wave.gif",
      "static_url": "https://example.com/emojis/wave.png",
      "visible_in_picker": true
    }
  ]
}
```

### 2. Account Profiles
Custom emojis can be used in display names and bios. The account object should include used emojis.

### 3. Federation
When federating content with custom emojis:
- Include emoji data in the ActivityPub object
- Download and cache remote emojis
- Track the source domain

## Admin Management

### Creating a Custom Emoji

```bash
curl -X POST http://localhost:8080/api/v1/admin/custom_emojis \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "shortcode": "blobcat",
    "url": "https://example.com/emojis/blobcat.gif",
    "static_url": "https://example.com/emojis/blobcat.png",
    "category": "cats"
  }'
```

### Listing Custom Emojis

```bash
curl http://localhost:8080/api/v1/custom_emojis
```

Response:
```json
[
  {
    "shortcode": "blobcat",
    "url": "https://example.com/emojis/blobcat.gif",
    "static_url": "https://example.com/emojis/blobcat.png",
    "visible_in_picker": true,
    "category": "cats"
  }
]
```

## Implementation Status

### Completed
- ✅ Storage interface and types
- ✅ DynamoDB implementation
- ✅ API handler for listing emojis
- ✅ Admin handlers for CRUD operations
- ✅ Data models

### TODO
1. **Content Parsing**: Implement emoji parsing in status content
2. **Media Upload**: Support uploading emoji images to S3
3. **Remote Emoji**: Implement downloading and caching of remote emojis
4. **UI Integration**: Add emoji picker support
5. **Reactions**: Use custom emojis in announcement reactions
6. **Import/Export**: Bulk emoji management
7. **Animation Support**: Handle animated GIFs appropriately
8. **Size Limits**: Implement file size and dimension limits

## Best Practices

### Emoji Guidelines
1. **Naming**: Use descriptive, lowercase shortcodes without spaces
2. **Size**: Recommend 32x32 or 64x64 pixels for optimal display
3. **Format**: Support PNG, GIF, and WebP formats
4. **Categories**: Use consistent category names for organization

### Performance Considerations
1. **Caching**: Cache emoji list as it changes infrequently
2. **CDN**: Serve emoji images through a CDN
3. **Lazy Loading**: Load emojis on demand in picker
4. **Compression**: Optimize images before storage

### Security Considerations
1. **Upload Validation**: Verify image file types and sizes
2. **Sanitization**: Ensure shortcodes don't contain malicious patterns
3. **Rate Limiting**: Limit emoji creation to prevent abuse
4. **Access Control**: Only admins can manage emojis

## Future Enhancements

1. **Emoji Packs**: Import/export emoji packs for easy sharing
2. **Usage Analytics**: Track which emojis are most popular
3. **Auto-Import**: Automatically import emojis from federated content
4. **Emoji Aliases**: Support multiple shortcodes for the same emoji
5. **Animated Previews**: Show animation on hover in picker
6. **Emoji Voting**: Let users vote for new emojis
7. **Seasonal Emojis**: Time-limited special emojis

## Testing

A test suite is available in `test_custom_emojis.py` covering:
- Public emoji listing
- Admin emoji creation
- Emoji updates and deletion
- Category filtering
- Error cases

## Conclusion

The custom emojis feature provides Lesser instances with the ability to create unique, expressive communication options for their users. The implementation follows Mastodon's API specification while laying the groundwork for enhanced features like federation support and advanced categorization. The serverless architecture ensures efficient storage and retrieval of emoji data. 