# Rate Limiting Migration to Lift-Native Implementation

## Overview

Lesser's rate limiting has been migrated from a custom implementation to use Lift's native rate limiting with the `limited` library. This provides better DynamoDB integration, automatic model registration, and follows Lift framework best practices.

## What Changed

### Before (Custom Implementation)
- Custom rate limiting middleware in `pkg/ratelimit/middleware.go`
- Used Lesser's custom `APIRateLimit` models
- Manual DynamoDB operations through repositories
- Complex model registration requirements

### After (Lift-Native Implementation)
- New `pkg/ratelimit/lift_middleware.go` using `limited` library
- Uses Lift's `DynamoRateLimiter` with `FixedWindowStrategy`
- Automatic model handling by the `limited` library
- No model registration errors

## Key Benefits

1. **No Model Registration Errors**: The `limited` library handles its own models internally
2. **Better Performance**: Uses Lift's optimized DynamoDB patterns
3. **Simpler Code**: Less custom code to maintain
4. **Standard Lift Patterns**: Follows Lift framework conventions
5. **Graceful Degradation**: On error, allows requests through rather than blocking

## Rate Limit Configuration

### Default Limits

- **API Endpoints**: 300 requests per 5 minutes
- **Write Operations** (POST/PUT/PATCH/DELETE): 30 requests per hour
- **Read Operations** (GET/HEAD): 300 requests per 5 minutes  
- **Search Endpoints**: 100 requests per 5 minutes
- **Federation Endpoints**: 1000 requests per minute

### Endpoint-Specific Limits

The middleware automatically selects appropriate limits based on:
- HTTP method (GET vs POST vs DELETE, etc.)
- Path patterns (timelines, search, statuses, etc.)
- Federation detection (inbox, outbox, webfinger, etc.)

## Implementation Details

### Rate Limit Key Generation

Keys are generated based on:
1. **User ID** (if authenticated): `user:{username}`
2. **IP Address** (if not authenticated): `ip:{ip_address}`
3. **Resource**: Normalized path (e.g., `/users/123` → `/users/*`)
4. **Operation**: HTTP method
5. **Metadata**: Tenant ID, user ID, IP

### Resource Normalization

Paths are normalized to group similar endpoints:
- Numeric IDs: `/users/123` → `/users/*`
- UUIDs: `/statuses/abc-def-123` → `/statuses/*`
- Query params removed
- Trailing slashes removed

### Response Headers

All responses include standard rate limit headers:
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 299
X-RateLimit-Reset: 1234567890
```

When rate limited (429 response):
```
Retry-After: 60
```

## Migration Steps Completed

1. ✅ Created new `lift_middleware.go` with Lift-native implementation
2. ✅ Updated `cmd/api/main.go` to use new middleware
3. ✅ Fixed import conflicts with `dynamormCore` alias
4. ✅ Updated `go.mod` to make `limited` a direct dependency
5. ✅ Resolved all linter errors

## Old Implementation

The old custom implementation is still present in `pkg/ratelimit/middleware.go` but is **no longer used**. It can be removed in a future cleanup:

- `pkg/ratelimit/middleware.go` - Old custom middleware (deprecated)
- `pkg/middleware/rate_limiter.go` - Old custom limiter (deprecated)
- `pkg/storage/models/rate_limit.go` - Old models (still used for legacy data)

## Testing

To test the new rate limiting:

```bash
# Test normal request
curl -H 'Authorization: Bearer <JWT>' https://dev.lesser.host/api/v1/accounts/verify_credentials

# Check rate limit headers
curl -i -H 'Authorization: Bearer <JWT>' https://dev.lesser.host/api/v1/accounts/verify_credentials

# Expected headers:
# X-RateLimit-Limit: 300
# X-RateLimit-Remaining: 299
# X-RateLimit-Reset: <timestamp>
```

## Troubleshooting

### Issue: Rate limiting not working

**Solution**: Check that `DisableRateLimiting` is not set to `true` in config

### Issue: 429 responses immediately

**Solution**: Check CloudWatch logs for DynamoDB errors. The `limited` library creates its own tables/items automatically.

### Issue: Rate limits too strict

**Solution**: Modify the strategy configurations in `lift_middleware.go`:
```go
apiStrategy := limited.NewFixedWindowStrategy(5*time.Minute, 300) // Adjust window or limit
```

## Performance Impact

- **Cold Start**: Minimal - no model pre-registration needed
- **Runtime**: Improved - fewer DynamoDB operations
- **Memory**: Reduced - simpler data structures

## Future Enhancements

Potential improvements:
1. Per-user custom limits (premium users, etc.)
2. Sliding window strategy instead of fixed window
3. Different limits per authentication method
4. Rate limit bypass for admin users (already supported via federation detection)
5. Redis-backed rate limiting for even better performance

## References

- Lift Rate Limiting Example: `/home/aron/ai-workspace/codebases/lift/examples/rate-limiting-limited/`
- Limited Library: https://github.com/pay-theory/limited
- Lift Documentation: `/home/aron/ai-workspace/codebases/lift/docs/`

