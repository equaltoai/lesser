# Lesser Endpoint Rate Limiting Plan

## Endpoints Analysis

### OAuth (Authentication) - NEEDS RATE LIMITING
- `POST /oauth/token` - Token generation
  - **Risk**: Token grinding, brute force attacks
  - **Limit**: 10 requests per minute per IP
  
- `GET /oauth/authorize` - Authorization flow
  - **Risk**: Authorization flooding
  - **Limit**: 20 requests per 5 minutes per IP

### Account Management - MIXED
- `GET /api/v1/accounts/verify_credentials` - Check current user
  - **Risk**: None (just reads your own data)
  - **Limit**: NONE - do not rate limit
  
- `PATCH /api/v1/accounts/update_credentials` - Update profile
  - **Risk**: Profile update spam
  - **Limit**: 10 requests per hour per user

### Data Export/Import - NEEDS RATE LIMITING (Expensive)
- `POST /exports` - Create data export
  - **Risk**: Resource exhaustion, data scraping
  - **Limit**: 5 requests per day per user
  
- `POST /imports` - Create data import
  - **Risk**: Resource exhaustion, storage abuse
  - **Limit**: 5 requests per day per user

### Community Notes - NEEDS RATE LIMITING
- `POST /notes` - Create community note
  - **Risk**: Spam, abuse
  - **Limit**: 20 requests per hour per user
  
- `POST /notes/{id}/vote` - Vote on note
  - **Risk**: Vote manipulation
  - **Limit**: 100 requests per hour per user

### Media Upload - NEEDS RATE LIMITING
- `POST /media` - Upload media file
  - **Risk**: Storage abuse, bandwidth consumption
  - **Limit**: 20 requests per hour per user

### Search - NEEDS RATE LIMITING
- `GET /api/v1/accounts/search` - Search for accounts
  - **Risk**: Scraping user data
  - **Limit**: 30 requests per 5 minutes per user
  
- `POST /api/v1/search/statuses` - Search statuses
  - **Risk**: Content scraping
  - **Limit**: 30 requests per 5 minutes per user

### Quote Posts - NEEDS RATE LIMITING
- `POST /api/v1/statuses/{id}/quote` - Create quote post
  - **Risk**: Spam, harassment
  - **Limit**: 30 requests per hour per user

### Admin Endpoints - NO RATE LIMITING
- All `/admin/*` endpoints
  - **Risk**: Low (already requires admin auth)
  - **Limit**: NONE - admins need unrestricted access

### Read-Only Endpoints - NO RATE LIMITING
- `GET /api/v1/accounts/verify_credentials`
- `GET /accounts/relationships`
- `GET /exports/*`
- `GET /imports/*`
- `GET /notes/*`
- `GET /media/*`
- `GET /api/v2/instance`
- `GET /api/v2/trends/*`
- `GET /api/v2/notifications/*`
- `GET /users/{username}/*` (ActivityPub)
- All health checks

## Implementation

Rate limit only these specific endpoints:

```go
var EndpointLimits = map[string]EndpointLimit{
	// OAuth - prevent token grinding
	"POST:/oauth/token":         {Limit: 10, Window: time.Minute},
	"GET:/oauth/authorize":      {Limit: 20, Window: 5 * time.Minute},
	
	// Account updates - prevent spam
	"PATCH:/api/v1/accounts/update_credentials": {Limit: 10, Window: time.Hour},
	
	// Export/Import - expensive operations
	"POST:/exports": {Limit: 5, Window: 24 * time.Hour},
	"POST:/imports": {Limit: 5, Window: 24 * time.Hour},
	
	// Community notes - prevent abuse
	"POST:/notes":           {Limit: 20, Window: time.Hour},
	"POST:/notes/*/vote":    {Limit: 100, Window: time.Hour},
	
	// Media - prevent storage abuse
	"POST:/media": {Limit: 20, Window: time.Hour},
	
	// Search - prevent scraping
	"GET:/api/v1/accounts/search":   {Limit: 30, Window: 5 * time.Minute},
	"POST:/api/v1/search/statuses":  {Limit: 30, Window: 5 * time.Minute},
	
	// Quote posts - prevent spam
	"POST:/api/v1/statuses/*/quote": {Limit: 30, Window: time.Hour},
}
```

## Test Plan

1. **Verify non-rate-limited endpoint works**:
   ```bash
   curl -H 'Authorization: Bearer <JWT>' https://dev.lesser.host/api/v1/accounts/verify_credentials
   # Should return 200 + account data immediately
   ```

2. **Verify rate-limited endpoint works and limits**:
   ```bash
   # Create multiple notes rapidly
   for i in {1..25}; do
     curl -X POST -H 'Authorization: Bearer <JWT>' \
       -H 'Content-Type: application/json' \
       -d '{"object_id":"test","text":"test note"}' \
       https://dev.lesser.host/notes
   done
   # First 20 should succeed, next 5 should return 429
   ```

