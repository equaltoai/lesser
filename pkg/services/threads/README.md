# Threads Service

## Overview

The Threads Service provides comprehensive thread synchronization and traversal capabilities for ActivityPub conversations. It enables Lesser to fetch, build, and maintain complete conversation threads across federated instances.

## Features

- **Thread Traversal**: Walk up and down reply chains to find thread roots and build complete conversation trees
- **Remote Synchronization**: Fetch missing notes and replies from remote instances
- **Missing Reply Detection**: Automatically detect and track gaps in conversation threads
- **Circular Reference Detection**: Prevent infinite loops when traversing malformed thread structures
- **Depth Limiting**: Configurable depth limits to prevent excessive traversal
- **Error Handling**: Classify and retry federation errors with exponential backoff

## Architecture

### Components

1. **Service Layer** (`service.go`)
   - Thread traversal logic
   - Remote note fetching
   - Missing reply synchronization
   - Federation integration

2. **Storage Models** (`pkg/storage/models/`)
   - `ThreadSync`: Tracks synchronization status
   - `ThreadNode`: Represents individual nodes in thread trees
   - `MissingReply`: Tracks missing replies with retry logic

3. **Repository** (`pkg/storage/repositories/thread_repository.go`)
   - Thread data persistence
   - Query operations
   - Batch operations

## Usage

### Service Initialization

```go
import (
    "github.com/equaltoai/lesser/pkg/services/threads"
    "github.com/equaltoai/lesser/pkg/storage/repositories"
)

service := threads.NewService(
    threadRepo,
    statusRepo,
    objectRepo,
    actorRepo,
    federationClient,
    publisher,
    logger,
    "example.com",
)
```

### Get Thread Context

Retrieve the complete context for a note, including ancestors and descendants:

```go
query := threads.ThreadContextQuery{
    NoteID:      "https://example.com/note/123",
    ViewerID:    "user123",
    IncludeTree: true,
}

context, err := service.GetThreadContext(ctx, query)
if err != nil {
    // Handle error
}

fmt.Printf("Root: %s\n", context.RootNote.ID)
fmt.Printf("Ancestors: %d\n", len(context.Ancestors))
fmt.Printf("Participants: %d\n", context.ParticipantCount)
fmt.Printf("Missing: %d\n", context.MissingCount)
```

### Sync Remote Thread

Fetch and synchronize a complete thread from a remote instance:

```go
cmd := threads.SyncRemoteThreadCommand{
    NoteURL:      "https://remote.social/users/alice/status/123",
    Depth:        3, // Fetch up to 3 levels deep
    ViewerID:     "user123",
    ForceRefresh: false,
}

result, err := service.SyncRemoteThread(ctx, cmd)
if err != nil {
    // Handle error
}

fmt.Printf("Synced %d posts\n", result.SyncedPosts)
fmt.Printf("Errors: %d\n", result.ErrorCount)
fmt.Printf("Status: %s\n", result.SyncStatus)
```

### Sync Missing Replies

Attempt to fetch replies that were previously marked as missing:

```go
cmd := threads.SyncMissingRepliesCommand{
    NoteID:   "https://example.com/note/123",
    ViewerID: "user123",
}

result, err := service.SyncMissingReplies(ctx, cmd)
if err != nil {
    // Handle error
}

fmt.printf("Synced %d replies\n", result.SyncedReplies)
```

## Storage Models

### ThreadSync

Tracks the synchronization status of a thread:

- `StatusID`: The status being synced
- `LastSyncAt`: When the last sync occurred
- `SyncStatus`: pending, syncing, completed, failed
- `MissingReplies`: List of missing reply IDs
- `RemoteFetched`: Whether remote fetch was attempted
- `ThreadDepth`: Current known depth

### ThreadNode

Represents a single node in the thread tree:

- `RootStatusID`: The thread root
- `StatusID`: This node's status ID
- `ParentID`: Direct parent ID
- `Depth`: Depth in the tree (0 for root)
- `Path`: Full path from root
- `ChildIDs`: Direct children
- `ReplyCount`: Number of direct replies
- `DescendantCount`: Total descendants

### MissingReply

Tracks replies that couldn't be fetched:

- `ReplyID`: The missing reply ID
- `ParentStatusID`: Parent that referenced it
- `AttemptCount`: Number of fetch attempts
- `Status`: pending, fetching, failed, resolved
- `FailureReason`: deleted, 404, 403, timeout, unreachable, invalid
- `NextRetryAt`: When to retry (exponential backoff)

## Error Handling

The service defines custom error types for different failure scenarios:

```go
var (
    ErrThreadNotFound        error
    ErrThreadRootNotFound    error
    ErrCircularReference     error
    ErrMaxDepthExceeded      error
    ErrFetchRemoteNote       error
    ErrSyncInProgress        error
    // ... and more
)
```

Fetch errors are automatically classified:

- **404/not found**: `FailureReasonNotFound`
- **410/gone**: `FailureReasonDeleted`
- **403/forbidden**: `FailureReasonForbidden`
- **Timeout**: `FailureReasonTimeout`
- **Connection errors**: `FailureReasonUnreachable`

## Configuration

### Constants

```go
const (
    MaxThreadDepth = 10 // Maximum depth for traversal
    DefaultDepth   = 3  // Default sync depth
)
```

## Federation Integration

The service integrates with the federation layer to fetch remote content using HTTP Signatures (authorized fetch):

```go
type FederationClient interface {
    FetchObject(ctx, objectURL, signingActor) (any, error)
    FetchActor(ctx, actorURL, signingActor) (*Actor, error)
}
```

## Testing

Run the test suite:

```bash
go test ./pkg/services/threads/...
```

Run with coverage:

```bash
go test -cover ./pkg/services/threads/...
```

## Best Practices

1. **Depth Limiting**: Always set reasonable depth limits to prevent excessive federation requests
2. **Error Classification**: Use the built-in error classification for retry logic
3. **Missing Reply Tracking**: Leverage the automatic missing reply detection and retry system
4. **Circular Reference Detection**: The service automatically detects and prevents circular references
5. **Local First**: Always check local storage before fetching from remote instances

## Performance Considerations

- Thread traversal is bounded by `MaxThreadDepth` to prevent excessive recursion
- Missing replies use exponential backoff (5min, 15min, 1hr, 6hr, 24hr)
- Permanent failures (deleted, forbidden) are not retried
- Batch operations are used where possible

## Future Enhancements

- Parallel fetching of thread branches
- Caching of thread contexts
- Background sync jobs for popular threads
- Thread prefetching based on user activity
- Real-time thread updates via WebSocket

## Dependencies

- `pkg/activitypub`: ActivityPub types and utilities
- `pkg/federation`: Federation client for remote fetching
- `pkg/storage/models`: Data models
- `pkg/storage/repositories`: Data persistence
- `pkg/streaming`: Event publishing

## See Also

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [Phase 1.2 Implementation Plan](../../../docs/PHASE_1_2_BREAKDOWN.md)
- [System Design](../../../docs/architecture/SYSTEM_DESIGN.md)

