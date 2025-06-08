# Quick Start: Running Lesser Tests

This guide will help you quickly set up and run the comprehensive Lesser test suite.

## Prerequisites

1. **Install Python dependencies:**
   ```bash
   pip install -r tests/requirements.txt
   ```

2. **Install K6 for load testing (optional):**
   - macOS: `brew install k6`
   - Linux: Download from https://k6.io
   - Skip if you don't need load testing

## Step 1: Generate Authentication Token

### Option A: Use the Token Generator (Easiest)

```bash
# Interactive mode - follow prompts
python tests/utilities/generate_auth_token.py

# Or direct mode with username/password
python tests/utilities/generate_auth_token.py testuser testpassword
```

The token will be displayed. Copy it for the next step.

### Option B: Manual Token Generation

1. Create OAuth app:
```bash
curl -X POST http://localhost:8080/api/v1/apps \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "Test App",
    "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
    "scopes": "read write follow push"
  }'
```

2. Save the `client_id` and `client_secret` from response

3. Get access token:
```bash
curl -X POST http://localhost:8080/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password&client_id=YOUR_CLIENT_ID&client_secret=YOUR_CLIENT_SECRET&username=testuser&password=testpassword&scope=read write follow push"
```

4. Copy the `access_token` from response

## Step 2: Configure Test Environment

Set these environment variables:

```bash
# Required: Your access token
export LESSER_AUTH_TOKEN="your-access-token-here"

# Optional: Instance URL (defaults to http://localhost:8080)
export LESSER_URL="http://localhost:8080"

# Optional: OAuth credentials for comprehensive test
export LESSER_CLIENT_ID="your-client-id"
export LESSER_CLIENT_SECRET="your-client-secret"
```

## Step 3: Run Tests

### Run All Tests at Once
```bash
cd tests
./run_comprehensive_validation.sh
```

### Run Individual Test Suites

1. **API Tests** (requires auth token):
```bash
# First update the test to use env variables
# Edit line 32 of comprehensive_api_test.py to add client credentials from env:
# client_id = os.getenv('LESSER_CLIENT_ID')
# client_secret = os.getenv('LESSER_CLIENT_SECRET')

python integration/comprehensive_api_test.py
```

2. **Federation Tests** (no auth needed):
```bash
python integration/federation_validation_test.py
```

3. **Performance Benchmark**:
```bash
python integration/performance_benchmark.py
```

4. **Load Test** (requires K6):
```bash
k6 run load/lesser_load_test.js
```

## Expected Output

When tests run successfully, you'll see:

```
🏛️  Testing Instance Information...
✅ GET /api/v1/instance - PASSED Domain: localhost
✅ GET /api/v2/instance - PASSED Version: 0.1.0

🔐 Testing OAuth Flow...
✅ POST /api/v1/apps - PASSED App registered
✅ GET /oauth/authorize - PASSED Endpoint exists

👤 Testing Account Management...
✅ GET /api/v1/accounts/verify_credentials - PASSED User: @testuser
...
```

## Troubleshooting

### "401 Unauthorized" errors
- Check your access token is correct
- Verify token hasn't expired
- Make sure LESSER_AUTH_TOKEN is exported

### "Connection refused" errors
- Ensure Lesser is running on the expected port
- Check LESSER_URL environment variable

### Tests hang or timeout
- Some tests require actual data in the instance
- Federation tests may timeout if no remote instances are configured

### comprehensive_api_test.py needs OAuth credentials
The comprehensive_api_test.py expects client_id and client_secret as parameters. Either:
- Set them as environment variables and modify the test
- Or use the token generator which creates an app automatically

## Next Steps

After running basic tests:
1. Create test data (users, posts) for more thorough testing
2. Configure federation with test instances
3. Run load tests to check performance
4. Review test reports in the generated directories 