# Idempotency Middleware for Lift

This middleware provides automatic idempotency support for Lift applications, ensuring that duplicate requests with the same idempotency key return the same response without re-executing the handler logic.

## How It Works

The idempotency middleware intercepts requests and responses using a custom `ResponseInterceptor` that wraps the Lift context. This allows it to:

1. **Check for duplicate requests** before handler execution
2. **Capture responses** as they're being written
3. **Store responses** for future duplicate requests
4. **Return cached responses** for duplicate requests

### Key Innovation: Response Interception

The middleware uses a `ResponseInterceptor` that wraps the original context and intercepts calls to `ctx.JSON()`, `ctx.Status()`, etc. This allows it to capture the response data before it's written to the client.

```go
type ResponseInterceptor struct {
    *lift.Context
    capturedResponse any
    capturedStatus   int
    capturedError    error
    mu               sync.Mutex
}
```

## Usage

### Basic Setup

```go
import (
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

func main() {
    app := lift.New()
    
    // Use in-memory store for development
    store := middleware.NewMemoryIdempotencyStore()
    
    // Add idempotency middleware
    app.Use(middleware.Idempotency(middleware.IdempotencyOptions{
        Store: store,
        HeaderName: "Idempotency-Key",
        TTL: 24 * time.Hour,
    }))
    
    // Your routes
    app.POST("/payment", createPayment)
    
    app.Run()
}
```

### With DynamoDB Store

```go
// Setup DynamoDB client
cfg, _ := config.LoadDefaultConfig(context.Background())
dynamoClient := dynamodb.NewFromConfig(cfg)

// Create store
store := middleware.NewDynamoDBIdempotencyStore(dynamoClient, "idempotency-keys")

// Use in middleware
app.Use(middleware.Idempotency(middleware.IdempotencyOptions{
    Store: store,
}))
```

## Configuration Options

- **Store**: Backend storage implementation (required)
- **HeaderName**: Header to check for idempotency key (default: "Idempotency-Key")
- **TTL**: How long to cache successful responses (default: 24 hours)
- **ProcessingTimeout**: Timeout for in-flight requests (default: 30 seconds)
- **IncludeRequestHash**: Validate request body hasn't changed (default: false)
- **OnDuplicate**: Callback for duplicate request detection

## Features

### Automatic Response Caching
The middleware automatically captures and caches successful responses (status < 400).

### Error Response Caching
Failed requests are also cached to ensure consistent error responses.

### Concurrent Request Protection
Prevents multiple concurrent requests with the same idempotency key from executing simultaneously.

### Account Isolation
Idempotency keys are automatically scoped by account ID when present in the context.

### TTL Support
Cached responses automatically expire after the configured TTL.

## Implementation Details

### Request Flow

1. **Check for Idempotency Key**: If no key is present, the request proceeds normally
2. **Check Cache**: Look for existing response in the store
3. **Handle Cached Response**: If found, return the cached response with `X-Idempotent-Replay: true` header
4. **Lock Key**: Mark the key as "processing" to prevent concurrent duplicates
5. **Execute Handler**: Run the handler with the response interceptor
6. **Capture Response**: The interceptor captures the response data
7. **Store Response**: Save the response for future requests
8. **Return Response**: Return the response to the client

### Storage Interface

The middleware uses a pluggable storage interface:

```go
type IdempotencyStore interface {
    Get(ctx context.Context, key string) (*IdempotencyRecord, error)
    Set(ctx context.Context, key string, record *IdempotencyRecord) error
    SetProcessing(ctx context.Context, key string, expiresAt time.Time) error
    Delete(ctx context.Context, key string) error
}
```

### DynamoDB Table Schema

For the DynamoDB implementation:
- **Partition Key**: `pk` (string) - The idempotency key
- **TTL Attribute**: `ttl` (number) - Unix timestamp for automatic expiration
- **Attributes**: status, response, status_code, error, created_at, request_hash

## Testing

The middleware includes comprehensive tests demonstrating:
- Basic idempotency behavior
- Concurrent request handling
- Error response caching
- Account isolation
- TTL expiration

Run tests with:
```bash
go test ./pkg/middleware -v -run TestIdempotency
```

## Best Practices

1. **Use Unique Keys**: Generate unique idempotency keys for each distinct request
2. **Set Appropriate TTL**: Balance between cache duration and storage costs
3. **Handle 409 Conflicts**: Clients should handle 409 "already processing" errors with retries
4. **Monitor Cache Hit Rate**: Track `X-Idempotent-Replay` headers to monitor effectiveness
5. **Use Account Scoping**: Ensure idempotency keys are scoped by user/account for multi-tenant apps

## Limitations

- The middleware only captures responses made through the context methods (JSON, Text, etc.)
- Direct writes to the response writer are not captured
- Very large responses may impact memory usage with the in-memory store

## Migration from Manual Approach

If you're currently using manual idempotency updates in handlers:

### Before (Manual):
```go
func handler(ctx *lift.Context) error {
    // Process request
    resp := processPayment()
    
    // Manual update
    if err := middleware.UpdateIdempotencyResponse(ctx, resp, resp.ID); err != nil {
        ctx.Logger.Error("Failed to update idempotency", err)
    }
    
    return ctx.JSON(resp)
}
```

### After (Automatic):
```go
func handler(ctx *lift.Context) error {
    // Process request
    resp := processPayment()
    
    // Just return - middleware handles idempotency
    return ctx.JSON(resp)
}
```

The new middleware eliminates the need for manual updates in every handler.