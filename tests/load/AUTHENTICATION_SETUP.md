# Authentication Setup for k6 Load Tests

The load test results show a 74% error rate because most endpoints require authentication. Here's how to fix this:

## Why Authentication Failed

Your test ran without a `LESSER_TOKEN`, which means:
- ❌ Home timeline requests failed (requires auth)
- ❌ Create status requests failed (requires auth)  
- ❌ Notifications requests failed (requires auth)
- ✅ Public timeline worked (public endpoint)
- ✅ Search worked (public endpoint)

## Getting an Access Token

### Option 1: Use the Lesser Web Interface
1. Log in to your Lesser instance at https://lesser.host
2. Go to Settings → Development → Applications
3. Create a new application with these scopes:
   - `read` - for timelines and notifications
   - `write` - for creating posts
4. Copy the access token

### Option 2: Use the API
```bash
# Register an application
curl -X POST https://lesser.host/api/v1/apps \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "k6 Load Tester",
    "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
    "scopes": "read write",
    "website": "https://k6.io"
  }'

# This returns client_id and client_secret
# Then authorize and get a token (follow OAuth flow)
```

### Option 3: Use an Existing Token
If you already have a Mastodon-compatible client, you can reuse its token.

## Running Tests with Authentication

Once you have a token:

```bash
# Set the token
export LESSER_TOKEN="your-access-token-here"

# Run the full test locally
k6 run --env LESSER_URL='https://lesser.host' --env LESSER_TOKEN="$LESSER_TOKEN" tests/load/lesser_load_test.js

# Or run on Grafana Cloud
k6 cloud run --env LESSER_URL='https://lesser.host' --env LESSER_TOKEN="$LESSER_TOKEN" tests/load/lesser_load_test.js
```

## Alternative: Public Endpoints Only

If you don't have authentication set up yet, use the public-only test:

```bash
# This only tests public endpoints (no auth required)
k6 run --env LESSER_URL='https://lesser.host' tests/load/lesser_load_test_public.js
```

## Interpreting Results

### With Authentication
- Error rate should be < 10%
- All endpoint types should show success
- Cost headers will be tracked (if enabled)

### Without Authentication  
- Error rate will be ~75% (only public endpoints work)
- Home timeline, notifications, and post creation will fail
- Only public timeline and search will succeed

## Security Notes

- Never commit tokens to version control
- Use environment variables or k6 secrets
- Rotate tokens regularly
- Consider using read-only tokens for testing when possible 