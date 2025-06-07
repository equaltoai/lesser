# Lesser Testing Overview

## Testing Philosophy

Lesser embraces comprehensive testing at all levels to ensure reliability, performance, and compatibility. Our testing infrastructure covers unit tests, integration tests, federation tests, and performance benchmarks.

## Test Categories

### 1. API Integration Tests

Complete test coverage for all Mastodon API endpoints:

- `test_api.py` - Core API functionality
- `test_api_automated.py` - Automated test suite
- `test_account_search.py` - Account search functionality  
- `test_conversations.py` - Conversation threads
- `test_favorites.py` - Favorite functionality
- `test_filters_mutes.py` - Content filtering and muting
- `test_lists.py` - List management
- `test_notifications.py` - Notification system
- `test_polls.py` - Poll functionality
- `test_priority1_endpoints.py` - Critical endpoint tests
- `test_streaming.py` - WebSocket streaming

### 2. Federation Tests

Comprehensive ActivityPub federation testing:

- `test_federation_complete.py` - Full federation test suite
- `test_federation_search.py` - Federated search functionality
- `federation_test_harness.py` - Mock federation instance simulator

### 3. Advanced Features Tests

Tests for Lesser's unique features:

- `test_cost_tracking.py` - Real-time cost tracking
- `test_enhanced_metrics.py` - Metrics and analytics
- `test_moderation_api.py` - Moderation mesh API
- `test_moderation_basic.py` - Basic moderation functions
- `test_graphql.py` - GraphQL API tests
- `test_debug_endpoints.py` - Debug endpoint tests
- `test_semantic_search.py` - AI-powered search
- `test_push_notifications.py` - Push notification system

### 4. Testing Utilities (Phase 3.3) ✅

**Test Data Generator** (`test_data_generator.py`)
- Generate realistic ActivityPub actors
- Create notes with varied content
- Build conversation threads
- Simulate follow networks
- Generate timeline data

**Federation Test Harness** (`federation_test_harness.py`)
- Mock remote ActivityPub instances
- Test activity delivery
- Verify HTTP signatures
- Measure federation performance

**Performance Benchmark** (`performance_benchmark.py`)
- Endpoint latency testing
- Concurrent load simulation
- Response time percentiles
- Throughput measurement
- Visual reporting

**Demo Script** (`test_utilities_demo.py`)
- Interactive demonstrations
- Sample data generation
- Usage examples

### 5. Media and CDN Tests

- `test_media_urls.py` - Media CDN functionality

### 6. Helper Scripts

- `add_test_data.py` - Populate instance with test data
- `extract_handlers.py` - Extract handler information

## Running Tests

### Basic Test Execution

```bash
# Run all tests
python3 test_api_automated.py

# Run specific test category
python3 test_federation_complete.py

# Run with authentication
export MASTODON_EMAIL="test@example.com"
export MASTODON_PASSWORD="password"
python3 test_api.py
```

### Generate Test Data

```bash
# Generate ActivityPub test data
python3 test_data_generator.py

# Run interactive demo
python3 test_utilities_demo.py
```

### Performance Testing

```bash
# Basic performance benchmark
python3 performance_benchmark.py --url http://localhost:3000

# With visualization
python3 performance_benchmark.py --url http://localhost:3000 --plot

# Authenticated endpoints
python3 performance_benchmark.py --url http://localhost:3000 --token YOUR_TOKEN
```

### Federation Testing

```bash
# Test federation flows
python3 federation_test_harness.py

# Run complete federation test suite
python3 test_federation_complete.py
```

## Test Requirements

Install test dependencies:

```bash
pip install -r requirements-test.txt
```

Required packages:
- `Mastodon.py>=1.8.1` - Mastodon API client
- `requests>=2.31.0` - HTTP library
- `python-dotenv==1.0.0` - Environment variables
- `pytest==7.4.3` - Test framework
- `Pillow==10.4.0` - Image processing
- `cryptography==41.0.7` - Cryptographic operations
- `faker>=20.0.0` - Test data generation
- `aiohttp>=3.9.0` - Async HTTP client
- `asyncio-throttle>=1.0.0` - Rate limiting

Optional:
- `matplotlib` - Performance visualization
- `pandas` - Data analysis

## Test Coverage Goals

1. **API Coverage**: 100% of Mastodon API endpoints
2. **Federation**: Core ActivityPub flows
3. **Performance**: <50ms response times at p99
4. **Reliability**: 99.99% uptime target
5. **Cost**: Track cost per operation

## Continuous Integration

Tests should be integrated into CI/CD pipeline:

1. **Pre-deployment**: Run unit and integration tests
2. **Post-deployment**: Run federation and performance tests
3. **Monitoring**: Continuous performance tracking
4. **Regression**: Prevent performance degradation

## Best Practices

1. **Isolation**: Tests should not depend on external services
2. **Repeatability**: Tests must produce consistent results
3. **Performance**: Tests should complete quickly
4. **Coverage**: Aim for comprehensive coverage
5. **Documentation**: Document test purposes and requirements

## Conclusion

Lesser's comprehensive testing infrastructure ensures reliability, performance, and compatibility. The combination of automated tests, testing utilities, and performance benchmarks provides confidence in the platform's quality and helps maintain Lesser's ambitious goals of <$0.01/month per user and <50ms response times. 