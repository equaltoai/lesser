# Idempotency Middleware Guide for Mockery Team

## Overview

The Lift framework now provides built-in idempotency middleware that ensures API operations are safe to retry without causing duplicate side effects. This is critical for payment processing, order creation, and any operation where accidental duplication could cause financial or data integrity issues.

## Quick Start

### 1. Basic Setup

```go
package main

import (
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

func main() {
    app := lift.New()
    
    // Create an idempotency store (in-memory for development)
    store := middleware.NewMemoryIdempotencyStore()
    
    // Add idempotency middleware
    idempotencyMiddleware := middleware.Idempotency(middleware.IdempotencyOptions{
        Store:      store,
        HeaderName: "Idempotency-Key",  // Header clients will send
        TTL:        24 * time.Hour,      // How long to cache responses
    })
    
    // Convert and use the middleware
    app.Use(lift.Middleware(idempotencyMiddleware))
    
    // Your routes...
    app.POST("/api/payments", createPaymentHandler)
}
```

### 2. Client Usage

Clients must send an `Idempotency-Key` header with requests they want to be idempotent:

```bash
curl -X POST https://api.example.com/api/payments \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: payment-12345-attempt-1" \
  -d '{"amount": 100.00, "currency": "USD"}'
```

If the same request is sent again with the same idempotency key, the cached response is returned without executing the handler.

## Production Setup with DynamoDB

For production environments, use the DynamoDB store for distributed idempotency:

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Load AWS config
cfg, err := config.LoadDefaultConfig(context.TODO())
if err != nil {
    log.Fatal(err)
}

// Create DynamoDB client
dynamoClient := dynamodb.NewFromConfig(cfg)

// Create DynamoDB idempotency store
store := middleware.NewDynamoDBIdempotencyStore(dynamoClient, middleware.DynamoDBStoreConfig{
    TableName:      "idempotency-keys",
    PartitionKey:   "id",
    TTLAttribute:   "expires_at",
    ConsistentRead: true,
})

// Use with middleware
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store:             store,
    HeaderName:        "Idempotency-Key",
    TTL:               24 * time.Hour,
    ProcessingTimeout: 30 * time.Second,
})))
```

### DynamoDB Table Setup

Create the DynamoDB table with this configuration:

```json
{
  "TableName": "idempotency-keys",
  "KeySchema": [
    {
      "AttributeName": "id",
      "KeyType": "HASH"
    }
  ],
  "AttributeDefinitions": [
    {
      "AttributeName": "id",
      "AttributeType": "S"
    }
  ],
  "BillingMode": "PAY_PER_REQUEST",
  "TimeToLiveSpecification": {
    "AttributeName": "expires_at",
    "Enabled": true
  }
}
```

## Configuration Options

### IdempotencyOptions

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Store` | `IdempotencyStore` | Required | Backend storage for idempotency records |
| `HeaderName` | `string` | `"Idempotency-Key"` | HTTP header containing the idempotency key |
| `TTL` | `time.Duration` | `24 * time.Hour` | How long to store successful responses |
| `ProcessingTimeout` | `time.Duration` | `30 * time.Second` | Timeout for in-flight requests |
| `IncludeRequestHash` | `bool` | `false` | Whether to validate request body hasn't changed |
| `OnDuplicate` | `func(*Context, *Record)` | `nil` | Callback when duplicate detected |

## Common Use Cases

### 1. Payment Processing

```go
app.POST("/api/payments", func(ctx *lift.Context) error {
    var req PaymentRequest
    if err := ctx.ParseRequest(&req); err != nil {
        return err
    }
    
    // Process payment (this will only run once per idempotency key)
    result, err := paymentProcessor.Charge(req.Amount, req.Token)
    if err != nil {
        return lift.NewError(http.StatusBadRequest, "Payment failed", err)
    }
    
    return ctx.JSON(PaymentResponse{
        TransactionID: result.ID,
        Status:       "success",
        Amount:       req.Amount,
    })
})
```

### 2. Order Creation

```go
app.POST("/api/orders", func(ctx *lift.Context) error {
    var order Order
    if err := ctx.ParseRequest(&order); err != nil {
        return err
    }
    
    // This will only create the order once
    orderID, err := orderService.CreateOrder(order)
    if err != nil {
        return err
    }
    
    return ctx.Status(201).JSON(map[string]string{
        "order_id": orderID,
        "status":   "created",
    })
})
```

### 3. Multi-Tenant Isolation

The middleware automatically isolates idempotency keys by account/tenant:

```go
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store: store,
    // Keys are automatically namespaced by account_id if present
})))

app.POST("/api/resources", func(ctx *lift.Context) error {
    accountID := ctx.Get("account_id").(string)
    // Idempotency key "create-123" for account A is different from 
    // idempotency key "create-123" for account B
    return ctx.JSON(map[string]string{"account": accountID})
})
```

## Client Implementation Guidelines

### 1. Generating Idempotency Keys

**Good practices:**
- Use UUIDs: `Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000`
- Include context: `Idempotency-Key: payment-userid123-orderid456-attempt1`
- Use deterministic keys for retries: `Idempotency-Key: checkout-session-789-submit`

**Avoid:**
- Timestamps alone (not unique enough)
- Sequential numbers (predictable)
- User input (security risk)

### 2. Retry Logic

```javascript
async function makeIdempotentRequest(url, data, maxRetries = 3) {
    const idempotencyKey = generateIdempotencyKey(data);
    
    for (let attempt = 0; attempt < maxRetries; attempt++) {
        try {
            const response = await fetch(url, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Idempotency-Key': idempotencyKey
                },
                body: JSON.stringify(data)
            });
            
            // Check for idempotent replay
            if (response.headers.get('X-Idempotent-Replay') === 'true') {
                console.log('Request was replayed from cache');
            }
            
            return await response.json();
        } catch (error) {
            if (attempt === maxRetries - 1) throw error;
            await sleep(Math.pow(2, attempt) * 1000); // Exponential backoff
        }
    }
}
```

### 3. Handling Responses

The middleware adds an `X-Idempotent-Replay: true` header when returning cached responses:

```javascript
const response = await fetch('/api/payments', {
    headers: { 'Idempotency-Key': key }
});

if (response.headers.get('X-Idempotent-Replay') === 'true') {
    // This was a cached response
    console.log('Payment already processed');
}
```

## Error Handling

### Concurrent Request Handling

If two requests with the same idempotency key arrive while the first is still processing:

```json
{
    "code": "IDEMPOTENCY_CONFLICT",
    "message": "A request with this idempotency key is already being processed",
    "status": 409
}
```

**Client should retry after a short delay.**

### Processing Timeout

If a request takes longer than `ProcessingTimeout`, subsequent requests can proceed:

```go
IdempotencyOptions{
    ProcessingTimeout: 30 * time.Second, // Adjust based on your longest operations
}
```

## Monitoring and Debugging

### 1. Logging Duplicate Requests

```go
app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
    Store: store,
    OnDuplicate: func(ctx *lift.Context, record *middleware.IdempotencyRecord) {
        ctx.Logger.Info("Duplicate request detected", map[string]any{
            "idempotency_key": record.Key,
            "original_time":   record.CreatedAt,
            "status_code":     record.StatusCode,
        })
    },
})))
```

### 2. Metrics

Track idempotency effectiveness:

```go
var (
    idempotencyHits = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "idempotency_cache_hits_total",
        Help: "Total number of idempotent request replays",
    })
    idempotencyMisses = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "idempotency_cache_misses_total",
        Help: "Total number of new idempotent requests",
    })
)
```

## Testing

### Unit Testing with Idempotency

```go
func TestPaymentIdempotency(t *testing.T) {
    app := lift.New()
    store := middleware.NewMemoryIdempotencyStore()
    
    // Add middleware
    app.Use(lift.Middleware(middleware.Idempotency(middleware.IdempotencyOptions{
        Store: store,
    })))
    
    // Track calls
    callCount := 0
    app.POST("/payment", func(ctx *lift.Context) error {
        callCount++
        return ctx.JSON(map[string]any{"id": "txn-123"})
    })
    
    // First request
    req1 := &lift.Request{
        Method: "POST",
        Path:   "/payment",
        Headers: map[string]string{
            "Idempotency-Key": "test-key-123",
        },
    }
    ctx1 := lift.NewContext(context.Background(), req1)
    err := app.HandleTestRequest(ctx1)
    assert.NoError(t, err)
    assert.Equal(t, 1, callCount)
    
    // Duplicate request
    req2 := &lift.Request{
        Method: "POST",
        Path:   "/payment",
        Headers: map[string]string{
            "Idempotency-Key": "test-key-123",
        },
    }
    ctx2 := lift.NewContext(context.Background(), req2)
    err = app.HandleTestRequest(ctx2)
    assert.NoError(t, err)
    assert.Equal(t, 1, callCount) // Handler not called again
    assert.Equal(t, "true", ctx2.Response.Headers["X-Idempotent-Replay"])
}
```

## Best Practices

### 1. Always Use Idempotency For:
- Payment processing
- Order creation
- Resource creation (users, accounts, etc.)
- Any operation with side effects that shouldn't be duplicated

### 2. Key Selection Strategy
- **User-initiated actions**: Use session/action identifiers
- **System retries**: Use deterministic keys based on the operation
- **Webhooks**: Use the webhook event ID as the idempotency key

### 3. TTL Configuration
- **Financial operations**: 24-48 hours
- **Resource creation**: 1-7 days
- **Temporary operations**: 1-6 hours

### 4. Error Response Caching
The middleware caches both successful AND error responses. This prevents:
- Repeated charges for declined cards
- Multiple emails for validation failures
- Duplicate error logging

## Troubleshooting

### Issue: "Idempotency key already used for different request"
**Solution**: If implementing request validation, ensure clients send identical request bodies with the same idempotency key.

### Issue: High memory usage with MemoryIdempotencyStore
**Solution**: Switch to DynamoDB store for production, or reduce TTL for memory store.

### Issue: Idempotency not working across services
**Solution**: Use a shared store (DynamoDB, Redis) accessible by all service instances.

## Migration from Existing Systems

If you have an existing idempotency implementation:

1. **Gradual rollout**: Use different header names during migration
2. **Dual-write period**: Write to both old and new systems
3. **Monitor**: Track both systems' hit rates
4. **Cutover**: Switch to new system once stable

## Security Considerations

1. **Key Uniqueness**: Ensure idempotency keys include user context to prevent cross-user replay attacks
2. **Key Entropy**: Use sufficiently random keys to prevent guessing
3. **TTL Limits**: Don't set TTL too long to limit storage and potential attack surface
4. **Request Validation**: Consider enabling `IncludeRequestHash` for sensitive operations

## Additional Resources

- [Example Implementation](../examples/response-interception/main.go)
- [Idempotency Tests](idempotency_buffering_test.go)
- [DynamoDB Store Implementation](idempotency_dynamodb.go)

For questions or issues, contact the Lift framework team or create an issue in the repository.