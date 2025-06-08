# Lesser API Test Suite

## Overview

This directory contains comprehensive API tests for Lesser that exercise all major endpoints.

## Test Coverage

The `comprehensive_api_test.py` script tests:

### Core Features
- Instance information endpoints
- OAuth app registration & authentication
- Account CRUD operations
- Status creation, editing, deletion
- Media uploads (v1 and v2)
- Polls

### Social Features
- Timelines (home, public, hashtag, list)
- Search & discovery
- Trends
- Lists management
- Filters
- Notifications
- Conversations
- Follow requests

### User Settings
- Preferences
- Bookmarks & Favourites
- Featured tags
- Markers

### Instance Features
- Custom emojis
- Announcements
- Profile directory
- Rules, privacy policy, terms

### Advanced Features
- Scheduled statuses
- Push subscriptions
- Reports
- Blocks & mutes
- Domain blocks

### Lesser-Specific Features
- Moderation system
- AI integration
- Reputation & vouches
- Community notes
- Cost tracking & analytics
- Debug endpoints

### Admin Features
- Account management
- Report handling
- Moderation overview

## Running the Tests

### Quick Start

```bash
# From the tests directory
./run_api_tests.sh https://your-instance.com

# Or with environment variables
export LESSER_URL=https://your-instance.com
export LESSER_TEST_USER=testuser
export LESSER_TEST_PASS=testpass123
./run_api_tests.sh
```

### Manual Run

```bash
# Setup environment (first time only)
./setup_test_env.sh

# Activate virtual environment
source ../test_venv/bin/activate

# Run tests
python api/comprehensive_api_test.py https://your-instance.com
```

## Test Results

Results are saved in timestamped directories:
- `test-reports-YYYYMMDD-HHMMSS/`
  - `api-comprehensive.log` - Full test output
  - `api-test-results-*.json` - Structured results

## Configuration

Environment variables:
- `LESSER_URL` - Instance URL (default: https://lesser.example.com)
- `LESSER_TOKEN` - Existing access token (if available, skips OAuth flow)
- `LESSER_TEST_USER` - Test username (default: testuser) - only used if no token
- `LESSER_TEST_PASS` - Test password (default: testpass123) - only used if no token

## Test Categories

Tests are organized into logical categories:
1. **Setup** - Instance info, app registration, authentication
2. **Accounts** - User account management
3. **Content** - Statuses, media, polls
4. **Discovery** - Timelines, search, trends
5. **Social** - Lists, filters, notifications
6. **Settings** - User preferences and features
7. **Instance** - Server-wide features
8. **Advanced** - Scheduled posts, reports
9. **Moderation** - Blocks, mutes, moderation
10. **Lesser-specific** - Custom features
11. **Admin** - Administrative functions

## Adding New Tests

To add new endpoint tests:

1. Add a new test method to `ComprehensiveAPITest` class
2. Follow the naming convention: `test_feature_name()`
3. Use `self.log_test()` to record results
4. Add the method to the `test_categories` list in `run_all_tests()`

Example:
```python
def test_new_feature(self):
    """Test new feature endpoints"""
    logger.info("\n🆕 Testing New Feature...")
    
    response = self.api_request('GET', '/new/endpoint')
    self.log_test("GET /api/v1/new/endpoint", 
                 "PASS" if response.status_code == 200 else "FAIL",
                 f"Status: {response.status_code}")
```

## Success Criteria

- All core endpoints should return appropriate status codes
- Authentication should work properly
- CRUD operations should complete successfully
- Error responses should be properly formatted
- Cost tracking headers should be present

## Debugging Failed Tests

1. Check the detailed logs in `test-reports-*/api-comprehensive.log`
2. Review the JSON results for specific failure details
3. Verify test credentials are correct
4. Ensure the instance is running and accessible
5. Check for any recent API changes 