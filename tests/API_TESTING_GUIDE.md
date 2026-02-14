# Lesser API Testing Guide

## Overview

This guide explains how to use the comprehensive API testing suite for Lesser. The suite tests **150+ API endpoints** across all major features.

## Quick Start

```bash
# Run all tests against your instance
cd tests
./run_api_tests.sh https://your-instance.com

# Results will be in test-reports-YYYYMMDD-HHMMSS/
```

## Test Coverage

The comprehensive test suite covers:

### ✅ Core Mastodon API (v1/v2)
- **Authentication**: OAuth app registration, token generation
- **Accounts**: CRUD operations, relationships, actions
- **Statuses**: Create, read, update, delete, interactions
- **Media**: Upload (v1/v2), update descriptions
- **Timelines**: Home, public, hashtag, list
- **Search**: Accounts, statuses, hashtags
- **Lists**: Create, manage, add/remove accounts
- **Notifications**: Fetch, clear, dismiss
- **Filters**: Create, manage keywords
- **Preferences**: Get/update user preferences

### ✅ Social Features
- **Following/Followers**: List, manage relationships
- **Favourites/Bookmarks**: Save and retrieve
- **Polls**: Create, vote, view results
- **Conversations**: Direct messages
- **Hashtags**: Follow/unfollow tags
- **Featured Tags**: Highlight tags on profile
- **Blocks/Mutes**: Account and domain blocking

### ✅ Instance Features
- **Instance Info**: Server details, rules, activity
- **Custom Emojis**: List available emojis
- **Announcements**: Server announcements
- **Directory**: Profile directory
- **Trends**: Trending tags, statuses, links

### ✅ Advanced Features
- **Scheduled Statuses**: Schedule posts
- **Push Notifications**: Web push setup
- **Reports**: Content reporting
- **Markers**: Timeline position syncing
- **Import/Export**: Data portability

### ✅ Lesser-Specific Features
- **Cost Tracking**: AWS cost headers and metrics
- **AI Integration**: Content analysis features
- **Moderation**: Advanced moderation system
- **Reputation**: Trust and vouch system
- **Community Notes**: Collaborative fact-checking
- **Federation Insights**: Instance connections

### ✅ Admin Features
- **Account Management**: Admin account actions
- **Report Handling**: Moderation queue
- **Domain Management**: Federation control

## Test Tools

### 1. Comprehensive Test Runner
```bash
./run_api_tests.sh [instance-url]
```
Runs all tests and generates detailed reports.

### 2. Single Endpoint Tester
```bash
# Test any endpoint quickly
./test_single_endpoint.py GET /instance
./test_single_endpoint.py GET /accounts/verify_credentials YOUR_TOKEN
./test_single_endpoint.py POST /statuses YOUR_TOKEN '{"status":"Test"}'
```

### 3. Test Result Analyzer
```bash
# Analyze test results
./analyze_test_results.py test-reports-*/api-test-results-*.json
```
Provides insights on:
- Success rates by category
- Failed test details
- Recommendations for improvement

### 4. Setup Script
```bash
# First-time setup
./setup_test_env.sh
```
Installs Python dependencies in a virtual environment.

## Configuration

### Environment Variables
```bash
export LESSER_URL=https://your-instance.com
export LESSER_TEST_USER=testuser
export LESSER_TEST_PASS=testpass123
export LESSER_TOKEN=your-access-token  # For single endpoint tests
```

### Test User Requirements
The test user should have:
- A valid account on the instance
- Permission to create apps
- Ability to post statuses
- (Optional) Admin privileges for admin endpoint tests

## Understanding Results

### Test Report Structure
```
test-reports-YYYYMMDD-HHMMSS/
├── api-comprehensive.log      # Full test output
└── api-test-results-*.json    # Structured results
```

### Result Categories
- **PASS** ✅ - Endpoint returned expected status code
- **FAIL** ❌ - Endpoint returned unexpected status/error
- **SKIP** ⚠️ - Test skipped (usually due to missing auth)
- **INFO** ℹ️ - Informational message

### Success Criteria
- **95%+** - Excellent, production ready
- **80-94%** - Good, minor issues to fix
- **60-79%** - Needs work, core features may be broken
- **<60%** - Major issues, not ready for production

## Common Issues

### Authentication Failures
- Verify test user credentials
- Check OAuth app registration is enabled
- Ensure password grant type is supported

### Intermittent 500s on AWS (Lambda throttling)
- Symptom: requests intermittently return `500` with a generic body like `{"message":"Internal server error"}` (often missing `x-request-id`)
- Likely cause: the API Gateway origin Lambda was throttled because the AWS account/region Lambda concurrency quota was too low
- Quick checks:
  - Lambda quota: `AWS_PROFILE=... aws lambda get-account-settings --region us-east-1 --query 'AccountLimit.ConcurrentExecutions' --output text`
  - Throttles: look for `AWS/Lambda` → `Throttles` > 0 on the relevant function(s) during the failure window
- Fix: increase the account concurrency quota (preferred) or reduce request parallelism during tests

### Endpoint Not Found (404)
- Check API version (v1 vs v2)
- Verify endpoint is implemented
- Check routing configuration

### Permission Denied (403)
- Test user may lack required permissions
- Check OAuth scopes requested
- Verify feature is enabled

### Cost Tracking Missing
- Check cost middleware is enabled
- Verify DynamoDB access
- Review CloudWatch logs

## Adding New Tests

To test new endpoints:

1. Edit `api/comprehensive_api_test.py`
2. Add a test method following the pattern:
```python
def test_new_feature(self):
    """Test new feature endpoints"""
    logger.info("\n🆕 Testing New Feature...")
    
    response = self.api_request('GET', '/new/endpoint')
    self.log_test("GET /api/v1/new/endpoint", 
                 "PASS" if response.status_code == 200 else "FAIL",
                 f"Status: {response.status_code}")
```
3. Add to `test_categories` list in `run_all_tests()`

## Best Practices

1. **Run tests regularly** - Catch regressions early
2. **Test after deployments** - Verify nothing broke
3. **Save test reports** - Track improvements over time
4. **Fix failures promptly** - Don't let them accumulate
5. **Add tests for new features** - Maintain coverage

## Debugging Tips

1. **Check detailed logs**: Look in `api-comprehensive.log`
2. **Test single endpoints**: Use `test_single_endpoint.py`
3. **Review JSON results**: Structured data in results file
4. **Check server logs**: CloudWatch/container logs
5. **Verify test data**: Ensure test user/data is valid

## Integration with CI/CD

```yaml
# Example GitHub Actions workflow
- name: Run API Tests
  env:
    LESSER_URL: ${{ secrets.LESSER_URL }}
    LESSER_TEST_USER: ${{ secrets.TEST_USER }}
    LESSER_TEST_PASS: ${{ secrets.TEST_PASS }}
  run: |
    cd tests
    ./setup_test_env.sh
    ./run_api_tests.sh
```

## Support

For issues or questions:
1. Check test logs for detailed error messages
2. Review this guide and README files
3. Check Lesser documentation
4. Report bugs with test reports attached 
