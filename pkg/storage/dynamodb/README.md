# DynamoDB Storage Implementation

This package implements the `storage.Storage` interface using AWS DynamoDB as the backend.

## Features

### Implemented ✅

1. **Actor Storage** (`actor.go`)
   - `CreateActor` - Creates a new actor with encrypted private key storage
   - `GetActor` - Retrieves an actor by username
   - `GetActorPrivateKey` - Retrieves an actor's private key
   - `UpdateActor` - Updates an existing actor
   - `DeleteActor` - Deletes an actor

2. **Activity Storage** (`activity.go`)
   - `CreateActivity` - Creates a new activity (supports both outbox and inbox)
   - `GetActivity` - Retrieves an activity by ID
   - `GetOutboxActivities` - Retrieves activities created by a user with pagination
   - `GetInboxActivities` - Retrieves activities delivered to a user with pagination

3. **Core Infrastructure** (`client.go`)
   - Connection pooling for Lambda reuse
   - DynamoDB client initialization in `init()`
   - Helper functions for building DynamoDB keys
   - Interface-based design for testability

### Not Yet Implemented ❌

- Object storage operations
- Relationship operations (follows)
- Collection operations
- DynamoDB transactions for multi-item operations
- AWS KMS encryption for private keys

## DynamoDB Schema

The implementation uses a single-table design with composite keys:

### Primary Keys
- **Actors**: 
  - PK: `ACTOR#{username}`
  - SK: `PROFILE`
- **Activities**:
  - PK: `ACTOR#{username}`
  - SK: `ACTIVITY#{timestamp}#{id}`

### Global Secondary Index (GSI1)
- **Inbox Activities**:
  - GSI1PK: `INBOX#{username}`
  - GSI1SK: `{timestamp}`

## Usage

```go
import "github.com/aron23/lesser/pkg/storage/dynamodb"

// Create a new storage instance
storage, err := dynamodb.New()
if err != nil {
    log.Fatal(err)
}

// Create an actor
actor := &activitypub.Actor{
    PreferredUsername: "alice",
    // ... other fields
}
err = storage.CreateActor(ctx, actor, privateKey)

// Get activities from outbox
activities, nextCursor, err := storage.GetOutboxActivities(ctx, "alice", 20, "")
```

## Testing

### Unit Tests

Run unit tests with mocked DynamoDB client:

```bash
GO_ENV=test go test ./pkg/storage/dynamodb -v
```

### Integration Tests

Run integration tests against local DynamoDB:

```bash
# Start local DynamoDB (e.g., using Docker)
docker run -p 8000:8000 amazon/dynamodb-local

# Run integration tests
GO_ENV=test go test ./pkg/storage/dynamodb -tags=integration -v
```

## Performance Considerations

1. **Lambda Cold Starts**: The DynamoDB client is initialized in `init()` and reused across invocations
2. **Pagination**: All list operations support cursor-based pagination
3. **Efficient Queries**: Uses DynamoDB query operations instead of scans where possible
4. **Eventually Consistent Reads**: Consider using for non-critical operations

## TODO

1. Implement remaining storage operations (objects, relationships, collections)
2. Add DynamoDB transactions for atomic multi-item operations
3. Implement AWS KMS encryption for private keys
4. Add batch operations for improved performance
5. Implement caching strategy for frequently accessed data
6. Add metrics and monitoring hooks 