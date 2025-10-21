# Rate Limit Fix Deployment

## What Was Fixed

1. **Used Lift's Built-in Pattern**: Replaced custom rate limiting with Lift's standard `middleware.LimitedRateLimit`
2. **Matches Autheory**: Now uses the same pattern as Autheory (proven to work in production)
3. **Simplified Implementation**: Lift's middleware creates its own DynamoDB client per request
4. **Environment Variables**: Uses `LIMITED_TABLE_NAME` and `AWS_REGION` like Lift expects

## Files Changed

### Application Code
- `pkg/ratelimit/lift_middleware.go` - Fixed client IP extraction logic
- `cmd/api/main.go` - Using Lift-native rate limiting

### Infrastructure (CDK)
- `infra/cdk/stacks/lesser_api_stack.go` - Added `RateLimitTable`
- `infra/cdk/constructs/lambda_functions.go` - Added `RATE_LIMIT_TABLE_NAME` env var
- `infra/cdk/constructs/security.go` - IAM permissions for rate limit table

## Deployment Steps

### 1. Deploy CDK Stack
```bash
cd infra/cdk
cdk deploy LesserApiStack-development --require-approval never
```

This will create the new `lesser-rate-limits-development` table.

### 2. Verify Table Creation
The table should have:
- Name: `lesser-rate-limits-development`
- PK: `PK` (String)
- SK: `SK` (String)
- TTL: `ExpiresAt`

### 3. Lambda Will Auto-Deploy
The Lambda will pick up:
- `RATE_LIMIT_TABLE_NAME=lesser-rate-limits-development`
- `ENVIRONMENT=development`

## Testing

```bash
# Should now return 200 with account data (not hang)
curl -H 'Authorization: Bearer <JWT>' https://dev.lesser.host/api/v1/accounts/verify_credentials

# Should include rate limit headers:
# X-RateLimit-Limit: 300
# X-RateLimit-Remaining: 299
# X-RateLimit-Reset: <timestamp>
```

## Rollback Plan

If issues occur, disable rate limiting:
```bash
# Set in CDK config
enableRateLimiting: false
```

Or comment out in `cmd/api/main.go`:
```go
// app.Use(ratelimit.LiftRateLimitMiddleware(rateLimitDB, logger))
```
