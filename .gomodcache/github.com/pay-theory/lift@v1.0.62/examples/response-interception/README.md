# Response Interception Example

This example demonstrates how to use Lift's response interception feature to capture and access response data after handler execution. This is useful for implementing middleware patterns like:

- Response caching
- Idempotency
- Response logging and auditing  
- Response transformation
- Metrics collection

## How Response Interception Works

1. **Enable Response Buffering**: Call `ctx.EnableResponseBuffering()` in your middleware before the handler executes
2. **Execute Handler**: The handler runs normally, writing responses using `ctx.JSON()`, `ctx.Text()`, etc.
3. **Access Response Data**: After handler execution, use `ctx.GetResponseBuffer()` to access the captured response

## Example Middleware Patterns

### Response Logging Middleware

```go
func ResponseLoggingMiddleware() lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            // Enable response buffering
            ctx.EnableResponseBuffering()
            
            // Execute handler
            err := next.Handle(ctx)
            
            // Access response data
            if buffer := ctx.GetResponseBuffer(); buffer != nil {
                body, statusCode, headers, capturedData := buffer.Get()
                log.Printf("Response: Status=%d, Body=%v", statusCode, capturedData)
            }
            
            return err
        })
    }
}
```

### Response Caching Middleware

```go
func ResponseCachingMiddleware(cache map[string]interface{}) lift.Middleware {
    return func(next lift.Handler) lift.Handler {
        return lift.HandlerFunc(func(ctx *lift.Context) error {
            cacheKey := fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path)
            
            // Check cache first
            if cached, exists := cache[cacheKey]; exists {
                return ctx.JSON(cached)
            }
            
            // Enable buffering to capture response
            ctx.EnableResponseBuffering()
            
            // Execute handler
            err := next.Handle(ctx)
            if err != nil {
                return err
            }
            
            // Cache successful responses
            if buffer := ctx.GetResponseBuffer(); buffer != nil {
                _, statusCode, _, capturedData := buffer.Get()
                if statusCode >= 200 && statusCode < 300 {
                    cache[cacheKey] = capturedData
                }
            }
            
            return nil
        })
    }
}
```

## Idempotency Middleware

The Lift framework's idempotency middleware uses response buffering internally to capture responses for replay:

```go
app.Use(middleware.Idempotency(middleware.IdempotencyOptions{
    Store:      store,
    HeaderName: "Idempotency-Key",
    TTL:        24 * time.Hour,
}))
```

When a duplicate request is detected (same idempotency key), the middleware returns the cached response without executing the handler again.

## Running the Example

```bash
go run main.go
```

The example will:
1. Set up response logging and caching middleware
2. Configure idempotency support
3. Create a simple product API
4. Simulate requests to demonstrate:
   - Response caching (second GET request hits cache)
   - Idempotency (duplicate POST requests return cached response)
   - Response logging (all responses are logged with timing)

## Key Benefits

1. **Performance**: Cache expensive operations
2. **Reliability**: Implement idempotency for safe retries
3. **Observability**: Log and monitor response data
4. **Flexibility**: Transform or enhance responses after handler execution

## Important Notes

- Response buffering has minimal overhead as it only stores references to the response data
- The buffering is enabled per-request, so there's no global performance impact
- Middleware can choose whether to use buffering based on request characteristics
- The original response is still sent to the client normally