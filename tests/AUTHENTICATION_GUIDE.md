# Authentication Guide for Lesser Tests

This guide explains how to generate authentication tokens for running the comprehensive Lesser test suite.

## Quick Start

### Option 1: Using the Token Generator (Interactive)

1. Run the interactive token generator:
   ```bash
   python tests/utilities/generate_auth_token.py
   ```

2. Follow the prompts:
   - It will create an OAuth app automatically
   - Choose option 1 (Password Grant) for simplest testing
   - Enter username/password (create new user or use existing)
   - Copy the generated access token

### Option 2: Command Line (Direct)

```bash
# For existing user
python tests/utilities/generate_auth_token.py testuser testpassword

# For different instance
python tests/utilities/generate_auth_token.py testuser testpassword https://your-lesser-instance.com
```

### Option 3: Manual Steps

1. **Create an OAuth App:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/apps \
     -H "Content-Type: application/json" \
     -d '{
       "client_name": "Test App",
       "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
       "scopes": "read write follow push"
     }'
   ```
   Save the `client_id` and `client_secret` from the response.

2. **Register a User (if needed):**
   ```bash
   curl -X POST http://localhost:8080/api/v1/accounts \
     -H "Content-Type: application/json" \
     -d '{
       "username": "testuser",
       "email": "testuser@example.com",
       "password": "testpassword",
       "agreement": true,
       "locale": "en"
     }'
   ```

3. **Get Access Token:**
   ```bash
   curl -X POST http://localhost:8080/oauth/token \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=password&client_id=YOUR_CLIENT_ID&client_secret=YOUR_CLIENT_SECRET&username=testuser&password=testpassword&scope=read write follow push"
   ```
   Save the `access_token` from the response.

## Running the Tests

### 1. Set Environment Variables

```bash
# Set your auth token
export LESSER_AUTH_TOKEN="your-access-token-here"

# Optional: Set instance URL (defaults to http://localhost:8080)
export LESSER_URL="http://localhost:8080"
```

### 2. Run Individual Test Suites

```bash
# API tests (uses auth token from env)
python tests/integration/comprehensive_api_test.py

# Federation tests (no auth needed)
python tests/integration/federation_validation_test.py

# Performance benchmark
python tests/integration/performance_benchmark.py

# Load test (K6 required)
k6 run tests/load/lesser_load_test.js
```

### 3. Run All Tests

```bash
# Run the comprehensive validation suite
./tests/run_comprehensive_validation.sh
```

## Saving Credentials

The token generator can save credentials to a `.env.lesser` file:

```bash
# .env.lesser
LESSER_URL=http://localhost:8080
LESSER_CLIENT_ID=your-client-id
LESSER_CLIENT_SECRET=your-client-secret
LESSER_ACCESS_TOKEN=your-access-token
LESSER_REFRESH_TOKEN=your-refresh-token
```

You can then source this file:
```bash
source .env.lesser
```

## Test Requirements

1. **Python Dependencies:**
   ```bash
   pip install -r tests/requirements.txt
   ```

2. **K6 for Load Testing (optional):**
   - macOS: `brew install k6`
   - Linux: `snap install k6`
   - Or download from https://k6.io/docs/getting-started/installation/

## Troubleshooting

### Token Expired
- Use the refresh token to get a new access token
- Or generate a new token using the steps above

### 401 Unauthorized
- Verify your token is correct
- Check if the user account is active
- Ensure the OAuth app has the required scopes

### Connection Refused
- Ensure Lesser is running on the expected port
- Check LESSER_URL environment variable

### Test User Already Exists
- Use the existing user's password
- Or create a user with a different username

## Example Test Run

```bash
# 1. Generate token for testing
python tests/utilities/generate_auth_token.py testuser testpass123

# 2. Export the token
export LESSER_AUTH_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# 3. Run API tests
python tests/integration/comprehensive_api_test.py

# 4. Run all tests
./tests/run_comprehensive_validation.sh
``` 