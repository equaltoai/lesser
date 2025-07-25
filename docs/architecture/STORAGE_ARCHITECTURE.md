# Lesser Storage Architecture: DynamoDB for Serverless

## Overview

Lesser is a serverless ActivityPub implementation running on AWS Lambda. Understanding how storage works in this architecture is crucial for any developer working on Lesser.

## Why DynamoDB?

### The Serverless Challenge

Lambda functions have several characteristics that make traditional storage approaches impossible:

1. **Stateless Execution**: Each Lambda invocation is independent
2. **Container Lifecycle**: Lambda containers are created and destroyed dynamically
3. **No Shared Memory**: Different Lambda instances cannot share in-memory data
4. **Cold Starts**: New containers start with no previous state

### Why In-Memory Storage Doesn't Work

```go
// THIS DOESN'T WORK IN LAMBDA!
type Service struct {
    cache map[string]any // Lost on container recycle
    mutex sync.RWMutex           // Only protects within single instance
}
```

Problems with in-memory storage in Lambda:
- Data is lost when the container is recycled (~15 minutes of inactivity)
- Each Lambda instance has its own memory space
- Concurrent requests might hit different instances
- Cold starts always have empty memory

### DynamoDB: The Serverless Database

DynamoDB is AWS's serverless NoSQL database that solves all these problems:
- **Persistent**: Data survives across all Lambda invocations
- **Shared**: All Lambda instances access the same data
- **Scalable**: Automatically scales with your application
- **Fast**: Single-digit millisecond performance
- **Cost-Effective**: Pay only for what you use

## Lesser's Storage Pattern

### Storage Interface

All storage operations go through a common interface (`pkg/storage/interface.go`):

```go
type Storage interface {
    // Actor operations
    CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
    GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
    
    // Session operations
    CreateSession(ctx context.Context, session *Session) error
    GetSession(ctx context.Context, sessionID string) (*Session, error)
    
    // Wallet operations (newly added)
    StoreWalletChallenge(ctx context.Context, challenge *WalletChallenge) error
    GetWalletChallenge(ctx context.Context, challengeID string) (*WalletChallenge, error)
    
    // ... hundreds more operations
}
```

### DynamoDB Implementation

The DynamoDB implementation (`pkg/storage/dynamodb/`) uses consistent patterns:

```go
// Key patterns
PK: USER#{username}              // Partition key for user data
SK: SESSION#{sessionId}          // Sort key for sessions
SK: WALLET#{address}             // Sort key for wallets
SK: CREDENTIAL#{credentialId}    // Sort key for WebAuthn

// TTL for automatic cleanup
TTL: {expiresAt.Unix()}          // DynamoDB deletes expired items
```

### Common Patterns

#### 1. User Data Storage
```go
// Store user-related data
item["PK"] = &types.AttributeValueMemberS{Value: s.userPK(username)}
item["SK"] = &types.AttributeValueMemberS{Value: "WALLET#" + address}
```

#### 2. Reverse Indexes
```go
// Allow lookups in both directions
// User -> Wallets
PK: USER#{username}, SK: WALLET#{address}

// Wallet -> User
PK: WALLET#{type}#{address}, SK: USER#{username}
```

#### 3. TTL for Temporary Data
```go
// Challenges expire automatically
item["TTL"] = &types.AttributeValueMemberN{
    Value: fmt.Sprintf("%d", expiresAt.Unix())
}
```

## Best Practices

### 1. Always Use the Storage Interface
```go
// Good
func (s *Service) GetData(ctx context.Context) error {
    return s.store.GetItem(ctx, key)
}

// Bad - Won't work in Lambda!
func (s *Service) GetData(ctx context.Context) error {
    return s.cache[key] // In-memory cache
}
```

### 2. Design for Concurrent Access
```go
// DynamoDB handles concurrency
// No need for mutexes or locks
```

### 3. Use TTL for Cleanup
```go
// Let DynamoDB clean up expired data
item["TTL"] = &types.AttributeValueMemberN{
    Value: fmt.Sprintf("%d", time.Now().Add(5*time.Minute).Unix())
}
```

### 4. Batch Operations When Possible
```go
// Use batch operations for efficiency
writeRequests := make([]types.WriteRequest, 0, len(items))
```

## Cost Considerations

DynamoDB pricing for Lesser:
- **Storage**: $0.25 per GB per month
- **Read Units**: $0.25 per million reads
- **Write Units**: $1.25 per million writes
- **TTL Deletes**: Free!

Typical costs per user:
- Storage: <$0.001/month
- Operations: <$0.001/month
- Total: <$0.002/month per active user

## Common Pitfalls

### 1. Forgetting Lambda is Stateless
```go
// This breaks in production!
var globalCache = make(map[string]string)
```

### 2. Not Using TTL
```go
// Without TTL, temporary data accumulates forever
// Always set TTL for temporary data like challenges
```

### 3. Inefficient Key Design
```go
// Bad: Scanning entire table
// Good: Query with proper partition/sort keys
```

## Conclusion

DynamoDB is not just a choice for Lesser—it's a requirement. The serverless architecture demands a storage solution that:
- Persists across Lambda invocations
- Scales automatically
- Provides consistent performance
- Handles concurrent access

Every feature in Lesser, from basic authentication to wallet signatures, relies on DynamoDB for reliable, scalable storage. Understanding this architecture is essential for contributing to Lesser. 