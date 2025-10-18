# Lists Service

The Lists Service provides comprehensive list management functionality for the Lesser ActivityPub implementation. This service handles Mastodon-style user-created lists for organizing followed accounts and generating custom timelines.

## Overview

Lists in Lesser allow users to:
- Create custom lists with titles and replies policies
- Add and remove accounts from their lists
- View timelines containing only posts from list members
- Manage list privacy (lists are private to their owners)

## Core Operations

### List Management

- **CreateList**: Create a new list with title and replies policy
- **UpdateList**: Modify list title and/or replies policy  
- **DeleteList**: Remove a list and all its memberships
- **GetList**: Retrieve a specific list (owner only)
- **ListUserLists**: Get all lists for a user with pagination

### Membership Management

- **AddToList**: Add an account to a list (owner only)
- **RemoveFromList**: Remove an account from a list (owner only)

### Timeline Generation

- **GetListTimeline**: Retrieve posts from list members with pagination

## Key Features

### Privacy and Security
- **Owner-only Access**: Only list owners can view, modify, or delete their lists
- **Private Lists**: Lists are not discoverable by other users
- **Authorization Checks**: All operations verify user permissions

### Event Streaming
All list operations emit real-time events for streaming:
- `list.created` - When a new list is created
- `list.updated` - When list metadata is modified
- `list.deleted` - When a list is deleted
- `list.member_added` - When an account is added to a list
- `list.member_removed` - When an account is removed from a list

### Replies Policies
Lists support three replies policies:
- `followed` - Show replies only from followed accounts
- `list` - Show replies only from accounts in the list
- `none` - Don't show any replies in the timeline

## Command Patterns

### CreateListCommand
```go
type CreateListCommand struct {
    Username      string `json:"username" validate:"required"`
    Title         string `json:"title" validate:"required,min=1,max=100"`
    RepliesPolicy string `json:"replies_policy" validate:"oneof=followed list none"`
    CreatorID     string `json:"creator_id" validate:"required"`
}
```

### UpdateListCommand
```go
type UpdateListCommand struct {
    ListID        string `json:"list_id" validate:"required"`
    Title         string `json:"title,omitempty" validate:"omitempty,min=1,max=100"`
    RepliesPolicy string `json:"replies_policy,omitempty" validate:"omitempty,oneof=followed list none"`
    UpdaterID     string `json:"updater_id" validate:"required"`
}
```

### AddToListCommand
```go
type AddToListCommand struct {
    ListID        string `json:"list_id" validate:"required"`
    MemberUsername string `json:"member_username" validate:"required"`
    AdderID       string `json:"adder_id" validate:"required"`
}
```

## Result Patterns

### ListResult
```go
type ListResult struct {
    List   *models.List        `json:"list"`
    Events []*streaming.Event `json:"events"`
}
```

### TimelineResult
```go
type TimelineResult struct {
    Statuses   []*models.Status                                 `json:"statuses"`
    Pagination *interfaces.PaginatedResult[*models.Status] `json:"pagination"`
    Events     []*streaming.Event                              `json:"events"`
}
```

## Dependencies

- **ListRepository**: Database operations for list storage and queries
- **NoteRepository**: Status/note operations for timeline generation
- **Publisher**: Event streaming for real-time updates
- **Logger**: Structured logging for operations and errors

## Error Handling

The service provides comprehensive error handling:
- **Validation Errors**: Invalid input parameters
- **Authorization Errors**: Unauthorized access attempts
- **Repository Errors**: Database operation failures
- **Business Logic Errors**: Constraint violations

## Usage Example

```go
// Create service
listService := NewService(listRepo, noteRepo, publisher, logger)

// Create a list
cmd := &CreateListCommand{
    Username:      "alice",
    Title:         "Close Friends",
    RepliesPolicy: "list",
    CreatorID:     "alice",
}

result, err := listService.CreateList(ctx, cmd)
if err != nil {
    log.Fatal(err)
}

// Add member to list
addCmd := &AddToListCommand{
    ListID:        result.List.ID,
    MemberUsername: "bob",
    AdderID:       "alice",
}

memberResult, err := listService.AddToList(ctx, addCmd)
if err != nil {
    log.Fatal(err)
}

// Get list timeline
timelineQuery := &GetListTimelineQuery{
    ListID:   result.List.ID,
    ViewerID: "alice",
    Pagination: interfaces.PaginationOptions{
        Limit: 20,
    },
}

timeline, err := listService.GetListTimeline(ctx, timelineQuery)
if err != nil {
    log.Fatal(err)
}
```

## Testing

The service includes comprehensive tests covering:
- ✅ All core operations (create, update, delete, membership)
- ✅ Authorization and privacy checks
- ✅ Validation error handling
- ✅ Event emission verification
- ✅ Edge cases and error scenarios
- ✅ Mock repository patterns

Run tests with:
```bash
go test ./pkg/services/lists/... -v
```

## Integration

This service integrates with:
- **API Layer**: Mastodon-compatible list endpoints
- **Storage Layer**: DynamoDB via ListRepository and NoteRepository
- **Streaming Layer**: Real-time event publishing
- **Federation**: ActivityPub list activity distribution (future)

## Performance Considerations

- **Lazy Loading**: List memberships loaded on-demand
- **Pagination**: All list queries support cursor-based pagination  
- **Event Batching**: Multiple operations can be batched for efficiency
- **Timeline Caching**: List timelines can be cached for performance