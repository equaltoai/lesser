# Hashtags Service

The Hashtags Service provides comprehensive hashtag management and discovery functionality for the Lesser ActivityPub server.

## Overview

This service implements hashtag following, timeline queries, suggestions, and real-time activity subscriptions, enabling users to discover and engage with content based on their interests.

## Features

### Core Operations

- **Follow/Unfollow Hashtags**: Users can follow hashtags to receive updates
- **Hashtag Timelines**: Single and multi-hashtag timeline queries with union (ANY) and intersection (ALL) modes
- **Hashtag Statistics**: Track usage counts, trending scores, and historical data
- **Hashtag Discovery**: Get suggested hashtags based on trending topics and user activity
- **Notification Management**: Customize notification settings for followed hashtags
- **Hashtag Muting**: Temporarily or permanently mute hashtags

### Real-time Features

- **Activity Subscriptions**: Stream real-time posts for followed hashtags
- **Trending Updates**: Track trending hashtags and their activity

## Architecture

### Service Layer (`service.go`)

The service implements the CQRS pattern with distinct Command and Query types:

**Commands** (mutations):
- `FollowHashtagCommand`
- `UnfollowHashtagCommand`
- `UpdateHashtagNotificationsCommand`
- `MuteHashtagCommand`

**Queries** (reads):
- `GetHashtagQuery`
- `GetFollowedHashtagsQuery`
- `GetHashtagTimelineQuery`
- `GetMultiHashtagTimelineQuery`
- `GetSuggestedHashtagsQuery`

### Storage Layer

The service uses:
- `HashtagRepository`: For hashtag metadata and relationships
- `StatusRepository`: For timeline queries
- `RelationshipRepository`: For user context

### Integration

The service is registered in the `services.Registry` and accessed via:
```go
hashtagService := registry.Hashtags()
```

## GraphQL API

### Queries

```graphql
# Get hashtag information
hashtag(name: String!): Hashtag

# Get followed hashtags
followedHashtags(first: Int, after: String): HashtagConnection!

# Get single hashtag timeline
hashtagTimeline(hashtag: String!, first: Int, after: String): PostConnection!

# Get multi-hashtag timeline
multiHashtagTimeline(
  hashtags: [String!]!
  mode: HashtagMode!
  first: Int
  after: String
): PostConnection!

# Get suggested hashtags
suggestedHashtags(limit: Int): [HashtagSuggestion!]!
```

### Mutations

```graphql
# Follow a hashtag
followHashtag(hashtag: String!, notifyLevel: NotificationLevel): HashtagFollowPayload!

# Unfollow a hashtag
unfollowHashtag(hashtag: String!): UnfollowHashtagPayload!

# Update notification settings
updateHashtagNotifications(
  hashtag: String!
  settings: HashtagNotificationSettingsInput!
): UpdateHashtagNotificationsPayload!

# Mute a hashtag
muteHashtag(hashtag: String!, until: Time): MuteHashtagPayload!
```

### Subscriptions

```graphql
# Stream hashtag activity
hashtagActivity(hashtags: [String!]!): HashtagActivityUpdate!
```

## Usage Examples

### Follow a Hashtag

```go
result, err := hashtagService.FollowHashtag(ctx, &hashtags.FollowHashtagCommand{
    UserID:               "alice",
    Hashtag:              "golang",
    NotificationsEnabled: true,
})
```

### Get Hashtag Timeline

```go
result, err := hashtagService.GetHashtagTimeline(ctx, &hashtags.GetHashtagTimelineQuery{
    Hashtag:  "golang",
    First:    20,
    ViewerID: "alice",
})
```

### Get Multi-Hashtag Timeline

```go
// Union mode (ANY): Posts with any of the hashtags
result, err := hashtagService.GetMultiHashtagTimeline(ctx, &hashtags.GetMultiHashtagTimelineQuery{
    Hashtags: []string{"golang", "rust", "typescript"},
    Mode:     "ANY",
    First:    20,
    ViewerID: "alice",
})

// Intersection mode (ALL): Posts with all hashtags
result, err := hashtagService.GetMultiHashtagTimeline(ctx, &hashtags.GetMultiHashtagTimelineQuery{
    Hashtags: []string{"golang", "webdev"},
    Mode:     "ALL",
    First:    20,
    ViewerID: "alice",
})
```

### Get Suggested Hashtags

```go
suggestions, err := hashtagService.GetSuggestedHashtags(ctx, &hashtags.GetSuggestedHashtagsQuery{
    UserID: "alice",
    Limit:  10,
})
```

## Error Handling

The service defines custom error types in `errors.go`:

- `ErrGetHashtag`: Failed to retrieve hashtag information
- `ErrFollowHashtag`: Failed to follow hashtag
- `ErrUnfollowHashtag`: Failed to unfollow hashtag
- `ErrGetHashtagTimeline`: Failed to retrieve timeline
- `ErrHashtagNameRequired`: Validation error for missing hashtag name
- And more...

## Testing

Unit tests are provided in `service_test.go`. Run tests with:

```bash
go test ./pkg/services/hashtags/...
```

## Performance Considerations

- **Timeline Queries**: Use cursor-based pagination for efficient large result sets
- **Multi-Hashtag Queries**: Intersection mode (ALL) is more expensive than union mode (ANY)
- **Trending Calculations**: Cached and updated periodically
- **Subscriptions**: Buffer size of 100 events prevents blocking

## Future Enhancements

- Advanced hashtag suggestions based on user interests and social graph
- Hashtag recommendations using machine learning
- Enhanced analytics and insights
- Hashtag groups/bundles for related topics
- Hashtag moderation and quality controls

## Related Components

- **Storage Models**: `pkg/storage/models/hashtag*.go`
- **Repository**: `pkg/storage/repositories/hashtag_repository.go`
- **GraphQL Resolvers**: `graph/schema.resolvers.go` (hashtag section)
- **GraphQL Schema**: `graph/schema.graphql` (hashtag types)

## Dependencies

- `pkg/storage/repositories`: Data access layer
- `pkg/streaming`: Real-time event publishing
- `go.uber.org/zap`: Structured logging
- `pkg/common`: Validation utilities

## Contributing

When adding new features:

1. Add command/query types in `service.go`
2. Implement service methods
3. Add corresponding error types in `errors.go`
4. Add GraphQL resolvers in `graph/schema.resolvers.go`
5. Add unit tests in `service_test.go`
6. Update this README

## License

Part of the Lesser ActivityPub server project.

