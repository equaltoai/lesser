# Idempotency Middleware - Quick Reference

## Setup (5 minutes)

```go
import (
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

// 1. Create store
store := middleware.NewMemoryIdempotencyStore() // Dev/Test

// 2. Add middleware
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store:      store,
    HeaderName: "Idempotency-Key",
    TTL:        24 * time.Hour,
})))

// 3. Create handlers - they're automatically idempotent!
app.POST("/api/payment", paymentHandler)
```

## Client Usage

```bash
# First request - processes normally
curl -X POST http://localhost:8080/api/payment \
  -H "Idempotency-Key: pay-123-attempt-1" \
  -H "Content-Type: application/json" \
  -d '{"amount": 100}'

# Duplicate request - returns cached response
curl -X POST http://localhost:8080/api/payment \
  -H "Idempotency-Key: pay-123-attempt-1" \
  -H "Content-Type: application/json" \
  -d '{"amount": 100}'
# Returns same response with header: X-Idempotent-Replay: true
```

## JavaScript Client Example

```javascript
// Simple idempotent request
async function createPayment(amount) {
    const response = await fetch('/api/payment', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Idempotency-Key': `payment-${userId}-${Date.now()}`
        },
        body: JSON.stringify({ amount })
    });
    
    if (response.headers.get('X-Idempotent-Replay') === 'true') {
        console.log('Payment already processed!');
    }
    
    return response.json();
}
```

## Production Setup (DynamoDB)

```go
// AWS Setup
cfg, _ := config.LoadDefaultConfig(context.TODO())
dynamoClient := dynamodb.NewFromConfig(cfg)

// Create store
store := middleware.NewDynamoDBIdempotencyStore(
    dynamoClient, 
    middleware.DynamoDBStoreConfig{
        TableName: "idempotency-keys",
    },
)

// Use same as memory store
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store: store,
})))
```

## Common Patterns

### Payment Processing
```go
app.POST("/charge", func(ctx *lift.Context) error {
    var req ChargeRequest
    ctx.ParseRequest(&req)
    
    // This only runs ONCE per idempotency key
    result := processPayment(req.Amount, req.Token)
    
    return ctx.JSON(result)
})
```

### Resource Creation
```go
app.POST("/users", func(ctx *lift.Context) error {
    var user User
    ctx.ParseRequest(&user)
    
    // Only creates user once, even if retried
    userID := db.CreateUser(user)
    
    return ctx.Status(201).JSON(map[string]string{
        "user_id": userID,
    })
})
```

## Key Generation Best Practices

✅ **GOOD Keys:**
- `payment-user123-order456-attempt1`
- `550e8400-e29b-41d4-a716-446655440000`
- `checkout-session-789-submit`

❌ **BAD Keys:**
- `1234` (too simple)
- `timestamp-1234567890` (not unique enough)
- User-provided strings (security risk)

## Response Headers

| Header | Value | Meaning |
|--------|-------|---------|
| `X-Idempotent-Replay` | `true` | Response was served from cache |
| Status | `409` | Another request with same key is in progress |

## Error Responses

```json
// Concurrent request (409)
{
    "code": "IDEMPOTENCY_CONFLICT",
    "message": "A request with this idempotency key is already being processed"
}

// Cached error response
{
    "code": "IDEMPOTENT_ERROR_REPLAY",
    "message": "Previous request failed"
}
```

## Testing

```go
func TestIdempotency(t *testing.T) {
    // Setup
    app := lift.New()
    store := middleware.NewMemoryIdempotencyStore()
    app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
        Store: store,
    })))
    
    calls := 0
    app.POST("/test", func(ctx *lift.Context) error {
        calls++
        return ctx.JSON(map[string]int{"calls": calls})
    })
    
    // First request
    req := &lift.Request{
        Method: "POST",
        Path: "/test",
        Headers: map[string]string{"Idempotency-Key": "test-123"},
    }
    ctx := lift.NewContext(context.Background(), req)
    app.HandleTestRequest(ctx)
    
    // Duplicate request
    ctx2 := lift.NewContext(context.Background(), req)
    app.HandleTestRequest(ctx2)
    
    // Handler only called once
    assert.Equal(t, 1, calls)
    assert.Equal(t, "true", ctx2.Response.Headers["X-Idempotent-Replay"])
}
```

## Debugging

```go
// Add logging for duplicate detection
IdempotencyOptions{
    Store: store,
    OnDuplicate: func(ctx *lift.Context, record *IdempotencyRecord) {
        log.Printf("Duplicate request: key=%s, original_time=%v", 
            record.Key, record.CreatedAt)
    },
}
```

## Configuration Reference

| Option | Default | Description |
|--------|---------|-------------|
| `HeaderName` | `"Idempotency-Key"` | Header containing the key |
| `TTL` | `24h` | How long to cache responses |
| `ProcessingTimeout` | `30s` | Timeout for in-flight requests |
| `IncludeRequestHash` | `false` | Validate request body hasn't changed |

## Need Help?

- Full Guide: [IDEMPOTENCY_GUIDE.md](./IDEMPOTENCY_GUIDE.md)
- Examples: [examples/response-interception/](../../examples/response-interception/)
- Tests: [idempotency_buffering_test.go](./idempotency_buffering_test.go)