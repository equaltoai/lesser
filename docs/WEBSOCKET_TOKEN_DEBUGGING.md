# WebSocket Connection Debugging - Token Extraction Issue

## Problem Summary

All WebSocket connection attempts are failing with "websocket connect missing authentication token" errors. Clients report they are sending authentication tokens in the connection URL, but the server is not receiving them.

## Changes Made

### Enhanced Logging in `cmd/graphql-ws/main.go`

1. **Added detailed event logging** in `handleConnect()`:
   - Logs all headers (including case variations)
   - Logs all query string parameters
   - Logs multi-value headers and query parameters
   - Logs connection metadata (domain name, source IP, request ID)

2. **Added token extraction logging** in `extractAuthToken()`:
   - Logs every step of token extraction
   - Logs when tokens are found and where (header vs query parameter)
   - Logs token length (for security - not the actual token)
   - Logs all query parameters being checked

## Next Steps

### 1. Deploy and Test

```bash
# Build and deploy the updated Lambda
make build-lambdas
make deploy-dev DOMAIN=dev.lesser.host
```

### 2. Monitor Logs

After deployment, attempt a WebSocket connection and immediately check the logs:

```bash
AWS_PROFILE=Lesser aws logs tail /aws/lambda/lesser-development-graphql-ws --follow --region us-east-1
```

Look for these log entries:
- `"websocket connect event received"` - Shows what API Gateway is sending
- `"extracting auth token from websocket event"` - Shows the extraction process
- `"checking query parameter"` - Shows each query parameter being checked
- `"found token in ..."` - Shows where token was found (if found)
- `"no auth token found in websocket event"` - Final fallback if nothing found

### 3. Analyze the Logs

The logs will show:
- **If query parameters are present**: Check `query_string_parameters` field
- **If headers are present**: Check `headers` field
- **The exact parameter names**: May be URL-encoded or case-sensitive
- **What API Gateway is actually forwarding**: May differ from what client sends

## Common Issues to Check

### Issue 1: Query Parameters Not Forwarded by API Gateway

API Gateway WebSocket connections may not forward query string parameters in the `$connect` event the same way HTTP requests do. 

**Solution**: Use Authorization header instead of query parameters:
```javascript
const ws = new WebSocket('wss://graphql-ws.dev.lesser.host', {
  headers: {
    'Authorization': `Bearer ${accessToken}`
  }
});
```

### Issue 2: URL Encoding Issues

If using query parameters, ensure proper URL encoding:
```javascript
// Correct
const ws = new WebSocket(`wss://graphql-ws.dev.lesser.host?access_token=${encodeURIComponent(token)}`);

// Incorrect
const ws = new WebSocket(`wss://graphql-ws.dev.lesser.host?access_token=${token}`);
```

### Issue 3: Parameter Name Mismatch

The code looks for:
- `access_token` (preferred)
- `token` (alternative)

Make sure your client uses one of these exact names.

### Issue 4: Custom Domain Configuration

If using a custom domain (`graphql-ws.dev.lesser.host`), ensure:
- API Gateway stage is configured correctly
- Domain mapping includes the stage
- Route53 records point to the correct API Gateway

## Expected Log Output

When working correctly, you should see:
```json
{
  "level": "info",
  "msg": "websocket connect event received",
  "connection_id": "...",
  "query_string_parameters": {
    "access_token": "your-token-here"
  }
}
```

Followed by:
```json
{
  "level": "info",
  "msg": "found token in query string parameters",
  "key": "access_token",
  "token_length": 123
}
```

## If Token Still Not Found

If logs show query parameters are empty but client is sending them:

1. **Check API Gateway Route Configuration**: Ensure `$connect` route is configured to forward query parameters
2. **Check API Gateway Stage Settings**: Query parameters may need to be explicitly enabled
3. **Use Authorization Header**: More reliable than query parameters for WebSocket connections
4. **Check WebSocket Client Library**: Some libraries may not forward query parameters correctly

## Additional Debugging

If needed, you can also check:
- API Gateway access logs (if enabled)
- CloudWatch X-Ray traces for the connection
- Network tab in browser dev tools (for web clients)

