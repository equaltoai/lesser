# Testing Lesser API with Mastodon.py

This directory contains test scripts to verify that the lesser API is compatible with Mastodon clients.

## Setup

1. Install the test dependencies:
```bash
pip install -r requirements-test.txt
```

## Test Scripts

### 1. `test_api_automated.py` - Automated Testing (No Manual Steps)

This script tests all public endpoints and doesn't require any manual OAuth authorization.

```bash
python test_api_automated.py
```

Tests include:
- Public endpoints (instance info, timelines, emojis)
- WebFinger lookups
- NodeInfo endpoints
- OAuth app registration
- Response headers and CORS
- Missing endpoints detection

### 2. `test_api.py` - Full API Testing (Manual OAuth)

This script tests authenticated endpoints but requires manual OAuth authorization.

```bash
python test_api.py
```

When prompted:
1. Visit the authorization URL in your browser
2. Log in with your test account
3. Authorize the app
4. Copy the authorization code and paste it in the terminal

Tests include:
- OAuth flow
- Posting and deleting statuses
- Timeline access
- Notifications
- Lists and preferences
- Search functionality
- Trends

## What These Tests Verify

The tests help ensure that lesser implements the Mastodon API correctly:

✅ **Working Features**:
- OAuth 2.0 authentication
- App registration
- Basic API endpoints
- CORS headers

❌ **Known Issues**:
- Posts not appearing in timelines (DynamoDB storage format issue)
- Some endpoints return empty data
- Missing some optional endpoints

## Interpreting Results

- ✓ = Endpoint works correctly
- ✗ = Endpoint failed or is not implemented
- Status codes and response data are shown for debugging

## Next Steps

After running these tests, you'll know which API endpoints need implementation or fixes. The Mastodon.py library expects standard Mastodon API responses, so any failures indicate areas where lesser needs to be more compatible. 