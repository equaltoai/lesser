 # Lesser Test Suite

Comprehensive testing framework for validating Lesser's ActivityPub implementation and API compliance.

## Overview

The Lesser test suite provides multiple levels of testing to ensure your instance is functioning correctly:

1. **API Compliance Tests** - Validates all Mastodon-compatible endpoints
2. **Federation Tests** - Ensures ActivityPub compliance
3. **Load Tests** - Performance and scalability validation
4. **Cost Tracking Tests** - Verifies Lesser's unique cost transparency features
5. **Integration Tests** - End-to-end functionality validation

## Quick Start

Run all tests with the comprehensive validation script:

```bash
# Basic validation (no authentication required)
./tests/run_comprehensive_validation.sh

# Full validation with authentication
export LESSER_URL="https://lesser.host"
export LESSER_TOKEN="your-access-token"
./tests/run_comprehensive_validation.sh
```

## Test Components

### 1. Comprehensive API Test (`comprehensive_api_test.py`)

Tests all Mastodon API endpoints including:
- Instance information
- OAuth flow
- Account management
- Status operations (create, edit, delete)
- Timelines (home, public, hashtag)
- Notifications
- Search functionality
- Lists management
- Media upload
- Preferences
- Trends

**Usage:**
```bash
python3 tests/integration/comprehensive_api_test.py https://your-instance.com --token YOUR_TOKEN
```

### 2. Federation Validation (`federation_validation_test.py`)

Validates ActivityPub compliance:
- WebFinger discovery
- Actor endpoints
- Collections (inbox, outbox, followers, etc.)
- HTTP signature support
- Content negotiation
- NodeInfo

**Usage:**
```bash
python3 tests/integration/federation_validation_test.py https://your-instance.com
```

### 3. Load Testing (`lesser_load_test.js`)

K6-based load testing simulating realistic usage:
- Gradual ramp-up to 200 concurrent users
- Mixed workload (timelines, posts, search)
- Performance metrics (P95 < 500ms)
- Cost tracking validation

**Usage:**
```bash
k6 run --env LESSER_URL=https://your-instance.com --env LESSER_TOKEN=YOUR_TOKEN tests/load/lesser_load_test.js
```

## Test Results

All test results are saved in timestamped directories:
```
test-reports-20240120-143022/
├── summary.txt              # Overall summary
├── instance-reachable.log   # Individual test logs
├── api-comprehensive.log
├── federation-validation.log
└── ...
```

## Getting an Access Token

To run authenticated tests, you need an access token:

1. **Via OAuth Flow:**
   ```bash
   # Register an app
   curl -X POST https://your-instance.com/api/v1/apps \
     -H "Content-Type: application/json" \
     -d '{
       "client_name": "Test Suite",
       "redirect_uris": "urn:ietf:wg:oauth:2.0:oob",
       "scopes": "read write follow push admin"
     }'
   ```

2. **Via Lesser's Modern Auth:**
   - Use the WebAuthn, passkey, or wallet authentication
   - Get token from the auth response

## Requirements

### Python Requirements
```bash
pip install -r tests/requirements.txt
```

Required packages:
- requests
- websocket-client
- cryptography
- Pillow (for media tests)

### K6 Installation (for load tests)
```bash
# macOS
brew install k6

# Linux
sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6
```

### Other Tools
- curl
- jq
- bash 4+

## Performance Baselines

Lesser targets these performance metrics:

| Endpoint | Target | Acceptable |
|----------|--------|------------|
| Timeline | < 50ms | < 200ms |
| Status Create | < 100ms | < 300ms |
| Search | < 150ms | < 500ms |
| Media Upload | < 500ms | < 2000ms |

## Cost Validation

Lesser's unique cost tracking is validated by checking for headers:
- `X-Cost-Total-Micros` - Total cost in microcents
- `X-Cost-DynamoDB-Reads` - Number of DB reads
- `X-Cost-DynamoDB-Writes` - Number of DB writes
- `X-Cost-Lambda-Duration-Ms` - Lambda execution time

## Continuous Integration

Example GitHub Actions workflow:

```yaml
name: Validate Lesser Instance

on:
  schedule:
    - cron: '0 */6 * * *'  # Every 6 hours
  workflow_dispatch:

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.10'
          
      - name: Install dependencies
        run: |
          pip install -r tests/requirements.txt
          
      - name: Run validation
        env:
          LESSER_URL: ${{ secrets.LESSER_URL }}
          LESSER_TOKEN: ${{ secrets.LESSER_TOKEN }}
        run: |
          ./tests/run_comprehensive_validation.sh
          
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: test-results
          path: test-reports-*
```

## Troubleshooting

### Common Issues

1. **"Instance not reachable"**
   - Check the URL is correct
   - Ensure the instance is running
   - Verify no firewall/proxy issues

2. **"401 Unauthorized"**
   - Token may be expired
   - Ensure token has required scopes
   - Check OAuth app is not revoked

3. **"Cost headers missing"**
   - Instance may not have cost tracking enabled
   - Check Lambda environment variables

4. **Load test failures**
   - May need to adjust thresholds for your instance size
   - Check instance scaling settings

## Contributing

To add new tests:

1. Add test functions to appropriate test file
2. Update the comprehensive runner script
3. Document expected behavior
4. Submit PR with test results

## Support

For issues with the test suite:
- Check existing GitHub issues
- Review instance logs
- Contact Lesser community on Discord