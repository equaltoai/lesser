# Notifications Service

The Notifications Service is a comprehensive service for handling all notification operations in the Lesser project's API alignment implementation. This service follows the established patterns from other services in the project and provides complete functionality for notification management.

## Overview

This service provides the core functionality for Phase 2.7 of the API Alignment Implementation, handling notification creation, reading, clearing, listing, and real-time event emission for ActivityPub and application notifications.

## Features

### Core Operations

- **CreateNotification**: Create new notifications with comprehensive validation and event emission
- **MarkAsRead**: Mark individual notifications as read with event streaming
- **ClearNotifications**: Clear notifications with multiple strategies (all, by type, specific IDs)
- **GetNotification**: Retrieve single notifications with privacy checks  
- **ListNotifications**: Get paginated notification lists with extensive filtering

### Event Streaming

The service emits real-time events for:
- `notification.created` - When new notifications are created
- `notification.read` - When notifications are marked as read
- `notification.cleared` - When notifications are cleared

All events are published to the recipient's notification stream for real-time updates.

### Notification Types Supported

- `mention` - User mentions in posts
- `reblog` - Post reblogs/boosts
- `favourite` - Post likes/favorites  
- `follow` - New followers
- `follow_request` - Follow requests (for protected accounts)
- `poll` - Poll updates
- `status` - General status updates
- `update` - Content updates
- `admin.sign_up` - Administrative sign-up notifications
- `admin.report` - Administrative reports

### Advanced Features

#### Notification Filtering

- Filter by notification types (include/exclude)
- Filter by read/unread status
- Filter by actor (who triggered the notification)
- Filter by target type (what the notification is about)
- Support for grouped notifications
- Date range filtering with pagination

#### Notification Grouping

- Automatic grouping of similar notifications within time windows
- Group key generation for consolidation
- Group count tracking for UI display

#### Summary Statistics

- Total notification counts
- Unread notification counts  
- Counts by notification type
- Last notification timestamp

## Architecture

### Dependencies

- **NotificationRepository**: Data access for notification operations
- **AccountRepository**: User account validation and lookups
- **Publisher**: Real-time event streaming to WebSocket clients
- **Logger**: Structured logging with zap

### Command Pattern

The service uses command structs for all write operations:

- `CreateNotificationCommand` - Create new notifications
- `MarkAsReadCommand` - Mark notifications as read
- `ClearCommand` - Clear notifications with various strategies

### Query Pattern

Read operations use query structs:

- `GetNotificationQuery` - Retrieve single notification
- `ListNotificationsQuery` - List notifications with filtering and pagination

### Result Pattern

All operations return result structs containing:

- The primary data (notification, list of notifications)
- Associated events that were emitted
- Pagination information for lists
- Summary statistics where applicable

## Usage Examples

### Creating a Mention Notification

```go
service := NewService(notificationRepo, accountRepo, publisher, logger, "example.com")

cmd := &CreateNotificationCommand{
    UserID:     "alice",
    Type:       "mention", 
    ActorID:    "bob",
    ActorType:  "user",
    TargetID:   "status123",
    TargetType: "status",
    Title:      "You were mentioned",
    Body:       "Bob mentioned you in a post",
}

result, err := service.CreateNotification(ctx, cmd)
if err != nil {
    // Handle error
}

// Notification created and events emitted to alice's stream
```

### Listing Unread Notifications

```go
query := &ListNotificationsQuery{
    UserID:     "alice",
    OnlyUnread: true,
    Pagination: interfaces.PaginationOptions{
        Limit: 20,
    },
}

result, err := service.ListNotifications(ctx, query)
if err != nil {
    // Handle error  
}

// result.Notifications contains unread notifications
// result.Summary contains count statistics
```

### Clearing Notifications by Type

```go
cmd := &ClearCommand{
    UserID: "alice", 
    Types:  []string{"mention", "follow"},
}

result, err := service.ClearNotifications(ctx, cmd)
if err != nil {
    // Handle error
}

// All mention and follow notifications cleared
// Events emitted to alice's stream
```

## Testing

The service includes comprehensive tests covering:

- Successful operations for all methods
- Validation error cases  
- Authorization and privacy checks
- Repository error handling
- Event emission verification
- Mock implementations for all dependencies

Run tests with:

```bash
go test ./pkg/services/notifications/ -v
```

## Integration

The Notifications Service follows the same patterns as other services in the Lesser project:

- Uses repository interfaces for data access
- Emits events through the streaming Publisher
- Follows command/query/result patterns
- Provides comprehensive error handling
- Includes structured logging
- Supports dependency injection

This service completes Phase 2 of the API Alignment Implementation and can be integrated with the API handlers, GraphQL resolvers, and federation components.