# API Gateway v1 to v2 Migration Guide

## Overview
**This is a critical bug fix, not a migration.** The entire system was built incorrectly using API Gateway v1 message types (`APIGatewayProxyRequest/Response`) while the infrastructure uses API Gateway v2. This fundamental misconfiguration means:

- Every Lambda handler is using the wrong request/response types
- The system is broken by design and only working by accident (if at all)
- All handlers must be fixed to use the correct v2 types

The infrastructure (`infra/main.go`) clearly shows we're using:
```go
"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
```

But all Lambda handlers were incorrectly implemented with v1 types. This is a critical architectural flaw that must be corrected.

## Key Changes

### 1. Type Changes
- **Request**: `events.APIGatewayProxyRequest` → `events.APIGatewayV2HTTPRequest`
- **Response**: `events.APIGatewayProxyResponse` → `*events.APIGatewayV2HTTPResponse` (note the pointer!)

### 2. Field Changes
- `request.Path` → `request.RawPath`
- `request.HTTPMethod` → `request.RequestContext.HTTP.Method`
- `request.PathParameters` → Same location (no change)
- `request.QueryStringParameters` → Same location (no change)
- Headers may need case-sensitive handling

### 3. Response Changes
All responses must now return a pointer:
```go
// Old (WRONG)
return events.APIGatewayProxyResponse{...}, nil

// New (CORRECT)
return &events.APIGatewayV2HTTPResponse{...}, nil
```

## Migration Steps

### Step 1: Update Common Package ✅
Already completed - `pkg/common/response.go` now returns v2 response pointers.

### Step 2: Update Auth Middleware ✅
Already completed - `pkg/auth/middleware.go` now accepts v2 requests.

### Step 3: Update Lambda Handlers

#### Simple Handlers (Do These First)
- [x] `cmd/webfinger/main.go` - Updated
- [x] `cmd/objects/main.go` - Updated
- [ ] `cmd/actor/main.go`

#### Collection Handlers
- [ ] `cmd/collections/main.go`
- [ ] `cmd/inbox/main.go`
- [ ] `cmd/outbox/main.go`

#### Complex Handlers
- [ ] `cmd/auth/main.go`
- [ ] `cmd/media/main.go`
- [ ] `cmd/api/main.go` (the big one - needs modularization)

#### No Changes Needed
- `cmd/activity-processor/main.go` - Uses DynamoDB streams, not API Gateway

### Step 4: Test Each Handler
After updating each handler:
1. Build: `make build-lambda LAMBDA=<name>`
2. Deploy: `cd infra && pulumi up`
3. Test the endpoint

## Common Patterns

### Before (WRONG):
```go
func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
    if request.HTTPMethod != http.MethodGet {
        return common.BadRequest(errors.New("method not allowed")), nil
    }
    
    // Process...
    
    return events.APIGatewayProxyResponse{
        StatusCode: http.StatusOK,
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
        Body: string(body),
    }, nil
}
```

### After (CORRECT):
```go
func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
    if request.RequestContext.HTTP.Method != http.MethodGet {
        return common.BadRequest(errors.New("method not allowed")), nil
    }
    
    // Process...
    
    return &events.APIGatewayV2HTTPResponse{
        StatusCode: http.StatusOK,
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
        Body: string(body),
    }, nil
}
```

## Modularizing cmd/api/main.go

After fixing the v2 types, split the 3000+ line file:

```
cmd/api/
├── main.go              # Just the router and handler function
├── handlers/
│   ├── accounts.go      # Account registration, verify credentials
│   ├── statuses.go      # Create, update, delete, favorite, reblog
│   ├── timelines.go     # Home, public timelines
│   ├── relationships.go # Follow, unfollow, block, unblock
│   ├── search.go        # Search functionality
│   └── instance.go      # Instance information
└── models/
    └── mastodon.go      # All Mastodon API structs
```

## Testing Commands

```bash
# Build a specific lambda
make build-lambda LAMBDA=webfinger

# Build all lambdas
make build

# Deploy
cd infra && pulumi up

# Test an endpoint
curl https://your-domain/.well-known/webfinger?resource=acct:test@your-domain
``` 