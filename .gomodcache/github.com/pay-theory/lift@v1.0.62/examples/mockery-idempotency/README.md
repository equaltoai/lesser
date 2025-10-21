# Mockery Payment Service: Idempotency Patterns with Lift

**This is the RECOMMENDED approach for implementing idempotency in payment processing and critical operations with Lift.**

## What is This Example?

This example demonstrates the **STANDARD patterns** for implementing idempotency in financial applications using Lift. It shows the **preferred approaches** for ensuring payment operations are exactly-once, even with network retries and client failures.

## Why Use These Idempotency Patterns?

✅ **USE these patterns when:**
- Building payment processing or financial transaction APIs
- Need to handle network retries and client failures gracefully
- Want to prevent duplicate charges from network issues
- Building APIs where duplicate requests cause serious problems
- Implementing any critical operation that must be exactly-once

❌ **DON'T USE when:**
- Building read-only operations (naturally idempotent)
- Operations where duplicates are harmless (like logging)
- Simple operations with no side effects
- Development environments where data consistency isn't critical

## Key Features Demonstrated

1. **AUTOMATIC Idempotency**: Payment handlers don't need idempotency logic - middleware handles it
2. **CRITICAL Retry Safety**: Network retries won't cause duplicate charges
3. **REQUIRED Concurrent Handling**: Prevents race conditions between simultaneous requests
4. **ESSENTIAL Error Caching**: Even failed payments are cached to prevent repeated attempts
5. **PRODUCTION-Ready Patterns**: Comprehensive logging, monitoring, and error handling

## Running the Example

```bash
cd examples/mockery-idempotency
go run main.go
```

The example will:
1. Start a payment service with idempotency enabled
2. Simulate various payment scenarios:
   - Normal payment with network retry
   - Multiple payments from same customer
   - Concurrent duplicate requests
3. Show how idempotency prevents double charges

## API Endpoints

### POST /api/v1/payments (IDEMPOTENT Pattern)
Process a payment with guaranteed exactly-once semantics

**Purpose:** Ensure payment operations are exactly-once even with retries
**When to use:** All financial transactions and critical operations

**CORRECT Request:**
```bash
curl -X POST http://localhost:8080/api/v1/payments \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: payment-unique-key-123" \  # REQUIRED: Unique per operation
  -d '{
    "amount": 99.99,
    "currency": "USD", 
    "customer_id": "cust-123",
    "description": "Monthly subscription"
  }'
```

**INCORRECT Request:**
```bash
# Missing Idempotency-Key header - allows duplicate charges
curl -X POST http://localhost:8080/api/v1/payments \
  -H "Content-Type: application/json" \
  -d '{"amount": 99.99, "currency": "USD"}'
```

**Response:**
```json
{
  "transaction_id": "txn_1234567890",
  "status": "completed",
  "amount": 99.99,
  "currency": "USD",
  "processed_at": "2024-01-02T15:04:05Z",
  "idempotency_key": "payment-unique-key-123"
}
```

### GET /api/v1/payments
List all processed payments (for demo purposes)

### GET /health
Health check endpoint

## Core Idempotency Patterns

### 1. First Request (STANDARD Behavior)
**What happens:** Original operation processing
- ✅ Processes payment normally with full business logic
- ✅ Stores response in idempotency cache for future requests
- ✅ Returns 201 Created with transaction details

### 2. Duplicate Request - Same Key (CRITICAL Behavior)
**What happens:** Cached response return
- ✅ Returns cached response immediately - NO payment processing
- ✅ Prevents duplicate charges automatically
- ✅ Returns 200 OK with `X-Idempotent-Replay: true` header
- ✅ IDENTICAL response to original request

### 3. Concurrent Requests (SAFETY Behavior)
**What happens:** Race condition prevention
- ✅ First request processes normally
- ✅ Concurrent duplicates receive 409 Conflict status
- ✅ Clients should implement exponential backoff retry
- ✅ Prevents race conditions that could cause duplicate processing

## Production Considerations

### 1. Persistent Storage (CRITICAL Pattern)

**Purpose:** Ensure idempotency works across Lambda instances and deployments
**When to use:** ALL production environments

```go
// CORRECT: Production setup with DynamoDB
cfg, _ := config.LoadDefaultConfig(context.TODO())
dynamoClient := dynamodb.NewFromConfig(cfg)

store := middleware.NewDynamoDBIdempotencyStore(dynamoClient, 
    middleware.DynamoDBStoreConfig{
        TableName: "mockery-idempotency-keys",  // REQUIRED: Persistent table
        TTL:       24 * time.Hour,             // REQUIRED: Prevent infinite growth
    })

// INCORRECT: Memory store in production
// store := middleware.NewMemoryIdempotencyStore()  // Lost on restart!
```

### 2. Idempotency Key Generation (DETERMINISTIC Pattern)

**Purpose:** Generate consistent keys for the same logical operation
**When to use:** Client-side key generation for all idempotent operations

```javascript
// CORRECT: Deterministic key generation
function generateIdempotencyKey(userId, operation, context) {
    // PREFERRED: Business context-based (same operation = same key)
    return `${userId}-${operation}-${context}`;
    
    // Examples:
    // "customer-123-payment-invoice-456"        // Same invoice = same key
    // "customer-123-subscription-monthly-202401" // Same billing period = same key
    // "tenant-abc-refund-order-789"             // Same refund = same key
}

// INCORRECT: Random keys for retries
function badKeyGeneration() {
    return `${userId}-${Date.now()}-${Math.random()}`;  // Different key each time!
    // This defeats idempotency - each retry becomes a new operation
}

// INCORRECT: Non-deterministic elements
function anotherBadPattern() {
    return `${userId}-payment-${crypto.randomUUID()}`;  // Random = not idempotent
}
```

### 3. Monitoring

Add metrics for production:
```go
OnDuplicate: func(ctx *lift.Context, record *middleware.IdempotencyRecord) {
    // Increment metrics
    idempotencyHits.Inc()
    
    // Log for analysis
    logger.Info("Duplicate payment prevented", 
        "customer_id", ctx.Get("customer_id"),
        "original_time", record.CreatedAt,
        "amount", record.Response,
    )
}
```

### 4. Error Handling

The middleware caches both successful AND failed responses:
- Prevents repeated charges for declined cards
- Avoids overwhelming payment gateways
- Provides consistent error responses

## What This Example Teaches

### ✅ Best Practices Demonstrated

1. **ALWAYS use idempotency for financial operations** - Prevents duplicate charges automatically
2. **ALWAYS use persistent storage in production** - DynamoDB instead of memory store
3. **ALWAYS use deterministic key generation** - Same operation = same key
4. **ALWAYS configure proper TTL** - Prevents infinite storage growth
5. **ALWAYS monitor duplicate patterns** - Essential for detecting client issues

### 🚫 Critical Anti-Patterns Avoided

1. **Random idempotency keys** - Defeats the purpose of idempotency
2. **Memory storage in production** - Lost on Lambda restarts
3. **No duplicate monitoring** - Can't detect client retry loops
4. **Missing error caching** - Failed operations should also be idempotent
5. **No processing timeouts** - Can cause resource exhaustion

### 💰 Financial Safety Benefits

- **100% Duplicate Charge Prevention** - Network retries are completely safe
- **Race Condition Protection** - Concurrent requests handled safely  
- **Client Bug Resilience** - Protects against client-side retry loops
- **Consistent Error Handling** - Even errors are cached idempotently

## Testing Idempotency

```go
func TestPaymentIdempotency(t *testing.T) {
    // Create app with idempotency
    app := createMockeryApp()
    
    // Process payment
    req := createPaymentRequest("test-key-123")
    resp1 := processRequest(app, req)
    
    // Retry with same key
    resp2 := processRequest(app, req)
    
    // Verify same response
    assert.Equal(t, resp1.TransactionID, resp2.TransactionID)
    assert.Equal(t, "true", resp2.Headers["X-Idempotent-Replay"])
}
```

## Migration Path

1. **Week 1**: Deploy with new header support
2. **Week 2**: Update clients to send idempotency keys
3. **Week 3**: Monitor and verify behavior
4. **Week 4**: Remove old idempotency code

## Questions?

See the [comprehensive guide](../../pkg/middleware/IDEMPOTENCY_GUIDE.md) for more details.