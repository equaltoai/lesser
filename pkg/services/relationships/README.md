# Relationships Service

The Relationships Service handles all relationship operations in the Lesser ActivityPub implementation, including follows, blocks, mutes, and relationship status management. This service is part of Phase 2.3 of the API Alignment Implementation.

## Features

### Core Operations

- **Follow/Unfollow**: Handle follow relationships with support for locked accounts and follow requests
- **Block/Unblock**: Block users with automatic unfollowing and privacy-aware events
- **Mute/Unmute**: Mute users to hide from timelines with optional duration support
- **Relationship Status**: Query comprehensive relationship data between users

### Advanced Features

- **Follow Requests**: Full support for locked accounts requiring follow approval
- **Mutual Relationship Detection**: Detect and report mutual follows
- **Privacy Controls**: Privacy-aware event emission (blocks/mutes only to actor's stream)
- **Batch Queries**: Support for querying multiple relationship statuses efficiently
- **Federation Support**: Queue ActivityPub activities for remote users

## Architecture

### Service Dependencies

```go
type Service struct {
    relationshipRepo interfaces.RelationshipRepository
    accountRepo      interfaces.AccountRepository
    publisher        streaming.Publisher
    federation       FederationService
    logger           *zap.Logger
    domainName       string
}
```

### Command Pattern

The service follows the established command pattern with structured input validation:

- `FollowCommand` - Follow a user with options for reblogs and notifications
- `UnfollowCommand` - Unfollow a user
- `BlockCommand` - Block a user with optional reason
- `UnblockCommand` - Unblock a user
- `MuteCommand` - Mute a user with optional duration and notification settings
- `UnmuteCommand` - Unmute a user

### Result Pattern

All operations return structured results with relationship data and emitted events:

- `RelationshipResult` - Single relationship with events
- `RelationshipsResult` - Multiple relationships (batch queries)
- `FollowResult` - Follow-specific data including request ID for locked accounts

## Usage Examples

### Following a User

```go
cmd := &relationships.FollowCommand{
    FollowerID:  "alice",
    FollowingID: "bob",
    ShowReblogs: true,
    Notify:      false,
}

result, err := service.Follow(ctx, cmd)
if err != nil {
    return err
}

if result.IsFollowing {
    log.Info("Now following user")
} else {
    log.Info("Follow request sent", "requestID", result.RequestID)
}
```

### Blocking a User

```go
cmd := &relationships.BlockCommand{
    BlockerID: "alice",
    BlockedID: "spam_user",
    Reason:    "spam and harassment",
}

result, err := service.Block(ctx, cmd)
if err != nil {
    return err
}

log.Info("User blocked", "blocking", result.Relationship.Blocking)
```

### Getting Relationship Status

```go
query := &relationships.GetRelationshipQuery{
    RequesterID: "alice",
    TargetID:    "bob",
}

relationship, err := service.GetRelationship(ctx, query)
if err != nil {
    return err
}

fmt.Printf("Following: %v, Followed by: %v, Blocked: %v\n",
    relationship.Following, relationship.FollowedBy, relationship.Blocking)
```

### Batch Relationship Queries

```go
query := &relationships.GetRelationshipsQuery{
    RequesterID: "alice",
    TargetIDs:   []string{"bob", "charlie", "diana"},
}

result, err := service.GetRelationships(ctx, query)
if err != nil {
    return err
}

for _, rel := range result.Relationships {
    fmt.Printf("User %s: following=%v, blocked=%v\n", 
        rel.ID, rel.Following, rel.Blocking)
}
```

## Event System

The service emits structured events for real-time streaming:

### Follow Events

- `relationship.follow_requested` - Follow request sent (locked accounts)
- `relationship.follow_accepted` - Follow request accepted
- `relationship.unfollowed` - User unfollowed

Events are sent to **both users' streams** for follow operations.

### Block Events

- `relationship.blocked` - User blocked
- `relationship.unblocked` - User unblocked

Events are sent **only to the blocker's stream** for privacy.

### Mute Events

- `relationship.muted` - User muted (with optional duration)
- `relationship.unmuted` - User unmuted

Events are sent **only to the muter's stream** for privacy.

## Federation Integration

The service automatically queues ActivityPub activities for remote users:

### Follow Activities

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "type": "Follow",
  "id": "https://example.com/activities/123",
  "actor": "https://example.com/users/alice",
  "object": "https://remote.com/users/bob"
}
```

### Block Activities

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "type": "Block",
  "id": "https://example.com/activities/456",
  "actor": "https://example.com/users/alice",
  "object": "https://remote.com/users/spammer"
}
```

### Undo Activities

```json
{
  "@context": "https://www.w3.org/ns/activitystreams",
  "type": "Undo",
  "id": "https://example.com/activities/789",
  "actor": "https://example.com/users/alice",
  "object": {
    "type": "Follow",
    "actor": "https://example.com/users/alice",
    "object": "https://remote.com/users/bob"
  }
}
```

## Relationship Data Structure

The service returns comprehensive relationship information:

```go
type RelationshipData struct {
    ID                  string    `json:"id"`                    // Target user ID
    Following           bool      `json:"following"`             // Following target
    ShowingReblogs      bool      `json:"showing_reblogs"`      // Show reblogs
    Notifying           bool      `json:"notifying"`            // Notify on posts
    Languages           []string  `json:"languages"`            // Language filter
    FollowedBy          bool      `json:"followed_by"`          // Followed by target
    Blocking            bool      `json:"blocking"`             // Blocking target
    BlockedBy           bool      `json:"blocked_by"`           // Blocked by target
    Muting              bool      `json:"muting"`               // Muting target
    MutingNotifications bool      `json:"muting_notifications"` // Muting notifications
    Requested           bool      `json:"requested"`            // Follow request sent
    RequestedBy         bool      `json:"requested_by"`         // Follow request received
    DomainBlocking      bool      `json:"domain_blocking"`      // Domain blocked
    Endorsed            bool      `json:"endorsed"`             // Endorsed user
    Note                string    `json:"note"`                 // Private note
    CreatedAt           time.Time `json:"created_at"`           // Relationship created
    UpdatedAt           time.Time `json:"updated_at"`           // Last updated
}
```

## Business Logic

### Follow Workflow

1. **Validation**: Prevent self-follows, check user existence
2. **Existing Check**: Return current status if already following
3. **Block Check**: Reject if blocked by target user
4. **Follow Request**: Create follow request in repository
5. **Approval Logic**: 
   - Public accounts: Auto-accept follow
   - Locked accounts: Leave pending for manual approval
6. **Events**: Emit appropriate events based on approval status
7. **Federation**: Queue ActivityPub Follow activity for remote users

### Block Workflow

1. **Validation**: Prevent self-blocks, check user existence
2. **Existing Check**: Return current status if already blocked
3. **Auto-Unfollow**: Remove existing follow relationships (both directions)
4. **Block Creation**: Create block record in repository
5. **Events**: Emit block event only to blocker's stream (privacy)
6. **Federation**: Queue ActivityPub Block activity for remote users

### Relationship Queries

The service builds comprehensive relationship data by querying:

- Follow status in both directions
- Block status in both directions
- Mute status
- Follow request status (pending/accepted/rejected)
- Additional metadata (reblogs, notifications, etc.)

## Error Handling

The service provides detailed error messages for various scenarios:

- **Validation Errors**: Missing required fields, invalid data
- **Business Logic Errors**: Self-actions, blocked users, etc.
- **Repository Errors**: Database failures, constraint violations
- **Federation Errors**: Remote delivery failures (logged, not propagated)

## Testing

The service includes comprehensive test coverage:

- **Unit Tests**: All public methods with mocked dependencies
- **Integration Tests**: End-to-end workflows with real dependencies
- **Edge Cases**: Error conditions, validation failures, race conditions
- **Mock Support**: Full mock implementations for testing

### Running Tests

```bash
go test ./pkg/services/relationships/...
```

## Performance Considerations

### Batch Operations

- `GetRelationships` supports up to 40 relationships per query
- Repository queries are optimized for DynamoDB single-table design
- Relationship data is cached where appropriate

### Event Publishing

- Events are published asynchronously
- Failed event delivery doesn't fail the operation
- Event publishing includes retry logic and error logging

### Federation Queuing

- Federation activities are queued asynchronously
- Failed federation delivery is logged but doesn't fail operations
- Only remote users receive federation activities (local users filtered out)

## Security & Privacy

### Privacy Controls

- **Block Events**: Only sent to blocker's stream, never to blocked user
- **Mute Events**: Only sent to muter's stream, never to muted user
- **Follow Events**: Sent to both users' streams as they're public actions

### Access Control

- Users can only modify their own relationships
- Relationship queries show privacy-appropriate data
- Blocked users cannot follow or interact

### Data Validation

- All commands are validated before processing
- User existence is verified for all operations
- Business logic prevents invalid state transitions

## Integration with Other Services

### Storage Layer

- Uses `RelationshipRepository` for all relationship data operations
- Uses `AccountRepository` for user existence checks and data

### Streaming System

- Publishes events via `streaming.Publisher` interface
- Events follow established patterns for consistency

### Federation System

- Queues activities via `FederationService` interface
- Activities follow ActivityPub specification

## Monitoring & Logging

### Structured Logging

All operations include structured logging with:

- User IDs involved in the operation
- Operation type and result
- Timing information
- Error details for debugging

### Metrics

The service tracks:

- Relationship operation counts by type
- Success/failure rates
- Event publishing success rates
- Federation queuing success rates

## Future Enhancements

### Planned Features

- **Relationship Notes**: Private notes on relationships
- **Endorsements**: Public endorsements of users
- **Domain Blocking**: Block entire domains
- **Temporary Mutes**: Time-based muting with automatic expiration
- **Relationship Analytics**: Statistics and insights

### Performance Optimizations

- **Relationship Caching**: Cache frequently accessed relationship data
- **Batch Event Publishing**: Group multiple events for efficiency
- **Federation Batching**: Batch federation activities where possible

## Compatibility

This service is designed to be fully compatible with:

- **Mastodon API**: All relationship endpoints match Mastodon behavior
- **ActivityPub**: All federation activities follow AP specification
- **Lesser Architecture**: Integrates with existing services and patterns